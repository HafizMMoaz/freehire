package applyform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/outbox"
)

// Claimed is one capture leased to this run. Provider and ExternalID come straight off the
// posting's row, which is everything a fetch needs — the external id is the board and the
// platform's posting id joined by sources.NamespaceExternalID.
type Claimed struct {
	OutboxID   int64
	JobID      int64
	Provider   string
	ExternalID string
	// URL is the posting's own address. Most fetchers ignore it — the board and posting
	// id are enough — but a platform with more than one regional host cannot be reached
	// without it, and the host cannot be discovered by trying: Lever answers 404 from the
	// wrong region, which is the same code it uses for a posting that no longer exists.
	URL string
}

// Store is the persistence the runner needs. The real implementation wraps the generated
// queries and a pool; tests use a fake. The port hides the pool and the transaction
// boundaries, not the queue's semantics.
type Store interface {
	// Claim leases up to batch live, unleased captures.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Save stores the captured form and retires the queue entry, atomically — so a
	// capture cannot be both stored and left queued, nor retired without being stored.
	Save(ctx context.Context, outboxID, jobID int64, form Form) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
	// Discard retires an entry whose posting the platform no longer has, storing nothing.
	Discard(ctx context.Context, outboxID int64) error
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// BatchSize is how many captures one claim wave leases.
	BatchSize int
	// LeaseSeconds is how long a claim is held before another run may take it — the
	// crash reaper, since a worker that dies mid-capture never releases its entries.
	LeaseSeconds int
	// MaxAttempts bounds the retries before a capture is dead-lettered.
	MaxAttempts int
	// Concurrency bounds how many postings are fetched at once. It is the only thing
	// stopping a backlog drain from becoming a flood aimed at one platform.
	Concurrency int
	// MaxPerRun bounds how much of the backlog ONE run takes; 0 is unbounded.
	//
	// Concurrency limits how fast, this limits how long, and the second bound is the one
	// that matters operationally. Nothing here holds a lock — what keeps a cron worker
	// from stacking on itself is systemd refusing to start a second instance of a
	// Type=oneshot unit while the first is active. So a run that works through a 185k
	// backlog for hours does not merely take a long time: it silently swallows every
	// scheduled firing behind it while ingest keeps enqueueing. Bounded, the cron cadence
	// is what sets throughput, which is where that decision belongs.
	MaxPerRun int
	// CallTimeout bounds a single fetch; 0 leaves it to the transport's own timeout.
	CallTimeout time.Duration
}

// RunStats is what the run did. DeadLettered is counted apart from Failed because a
// failure is a retry and a dead letter is work abandoned — a queue quietly filling with
// the latter is not a healthy queue.
type RunStats struct {
	Captured int
	Failed   int
	// Gone counts postings the platform no longer has. They are retired from the queue
	// rather than retried, and counted apart from Failed because they are ordinary
	// attrition — an employer took the posting down and our sweep has not caught up yet.
	Gone         int
	DeadLettered int
}

// Degraded reports whether a run deserves an alert, and it deliberately does NOT use the
// shared worker.ExitCode rule that every other worker here applies.
//
// That rule — non-zero on any per-item failure — fits a worker whose items rarely fail. This
// one issues hundreds of thousands of requests to other companies' APIs, where a handful of
// transient failures per run is the normal, healthy shape: the queue retries them, and the
// first production run measured 10 in 2490. Marking that degraded paints the unit red on
// nearly every firing, and a status that is always red carries no information.
//
// What does deserve an alert is work ABANDONED rather than postponed: a dead letter is a
// capture nobody will look at again. And a run that captured nothing at all while failing is
// not a blip either — that is the platform being gone, and it must not hide behind the same
// forgiveness the blips get.
func (s RunStats) Degraded() bool {
	return s.DeadLettered > 0 || (s.Captured == 0 && s.Failed > 0)
}

// Run drains the capture queue and returns. It is the whole worker: claim a wave, fetch
// each posting's form through its provider's fetcher, store it, and keep going until a
// wave comes back empty.
//
// It returns an error only when the QUEUE itself is unusable — there is no per-item
// recovery from being unable to claim. Every other failure belongs to one capture and is
// recorded against it, so one platform having a bad afternoon costs its own postings and
// nothing else.
func Run(ctx context.Context, s Store, fetchers map[string]Fetcher, opts RunOptions) (RunStats, error) {
	rn := &run{store: s, fetchers: fetchers, opts: opts}
	result, err := outbox.RunPool(ctx, cancelAwareClaimer{store: s}, outbox.RunOptions{
		BatchSize:    opts.BatchSize,
		LeaseSeconds: opts.LeaseSeconds,
		Concurrency:  opts.Concurrency,
		MaxPerRun:    opts.MaxPerRun,
	}, rn.process)
	stats := RunStats{
		Captured:     result.Succeeded,
		Failed:       result.Failed,
		Gone:         result.Discarded,
		DeadLettered: result.DeadLettered,
	}
	if err != nil {
		return stats, fmt.Errorf("claim apply-form captures: %w", err)
	}
	return stats, nil
}

// cancelAwareClaimer adapts Store to outbox.Claimer, checking ctx.Err() before every
// claim. A context cancellation shows up as every capture in the wave failing, so
// stopping here — rather than letting the next claim attempt or per-item fetch surface
// the cancellation as an ordinary error — keeps a cancelled run from burning the
// remaining attempts of a large backlog on a shutdown, matching the original behavior's
// post-wave ctx.Err() check (an empty claim is outbox.RunPool's ordinary clean-stop
// path, so this reproduces it without RunPool needing to know about cancellation).
type cancelAwareClaimer struct {
	store Store
}

func (c cancelAwareClaimer) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	if ctx.Err() != nil {
		return nil, nil
	}
	return c.store.Claim(ctx, batch, leaseSeconds)
}

// run carries one Run's dependencies and options so process uses the receiver instead
// of threading them through every call.
type run struct {
	store    Store
	fetchers map[string]Fetcher
	opts     RunOptions
}

// process fetches and stores one posting's form and reports what happened;
// outbox.RunPool tallies the result.
func (rn *run) process(ctx context.Context, c Claimed) outbox.Outcome {
	err := captureOne(ctx, rn.store, rn.fetchers, rn.opts, c)
	if err == nil {
		return outbox.Succeeded
	}

	// The platform does not have this posting and never will again. Retrying to reach
	// the same answer spends requests and ends in a dead letter that reads like a
	// fault, so the entry is retired instead.
	if errors.Is(err, ErrPostingGone) {
		if discardErr := rn.store.Discard(ctx, c.OutboxID); discardErr != nil {
			log.Printf("apply-form: retire gone capture %d: %v", c.OutboxID, discardErr)
		}
		return outbox.Discarded
	}

	dead, failErr := rn.store.Fail(ctx, c.OutboxID, err.Error(), rn.opts.MaxAttempts)
	if failErr != nil {
		log.Printf("apply-form: record failure for capture %d: %v", c.OutboxID, failErr)
	} else if dead {
		log.Printf("apply-form: capture %d (job %d) dead-lettered: %v", c.OutboxID, c.JobID, err)
	}
	if failErr == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
}

// captureOne fetches and stores one posting's form.
func captureOne(ctx context.Context, s Store, fetchers map[string]Fetcher, opts RunOptions, c Claimed) error {
	// A claim for a provider with no fetcher should be impossible — the enqueue gate and
	// the fetcher registry read the same map. If one appears anyway it is failed rather
	// than skipped: skipping would leave it claimable forever, re-read by every run.
	fetcher, ok := fetchers[c.Provider]
	if !ok {
		return fmt.Errorf("no fetcher for provider %q", c.Provider)
	}
	if _, _, ok := splitBoardPosting(c.ExternalID); !ok {
		return fmt.Errorf("external id %q is not a board-namespaced posting id", c.ExternalID)
	}

	if opts.CallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.CallTimeout)
		defer cancel()
	}

	form, err := fetcher.Fetch(ctx, c)
	if err != nil {
		return err
	}
	if err := s.Save(ctx, c.OutboxID, c.JobID, form); err != nil {
		return fmt.Errorf("store form for job %d: %w", c.JobID, err)
	}
	return nil
}
