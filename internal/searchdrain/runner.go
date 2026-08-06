// Package searchdrain drives the incremental facet-search queue: drain search_outbox
// wave by wave, pushing each wave into the live Meilisearch `jobs` index as ONE batch
// (one Meili task per wave, not per job).
//
// It replaces the previous design where cmd/ingest pushed each crawl's changed jobs
// straight to Meilisearch (search.Client.SubmitJobs) from inside every one of the
// per-board worker processes — observed in production to cost 50-90s per push
// regardless of batch size, because Meilisearch re-merges its inverted index/facet
// structures across the whole multi-million-document live index on every push, not
// just the changed rows. Routing every write through one outbox lets a single worker
// collapse however many boards changed in a window into one Meili task.
//
// It mirrors internal/embed: the batch/fallback logic is unit-tested with fakes of
// the Store + Indexer ports and cmd/search-drain wires the real adapters. Unlike
// embed there is no open/closed split — a job that closed or became a non-canonical
// repost after being queued is simply not claimed (see ClaimSearchOutboxBatch); that
// reconciles on the next full reindex, same as before this queue existed.
package searchdrain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Claimed is one outbox entry leased to this run.
type Claimed struct {
	OutboxID int64
	JobID    int64
}

// Store is the persistence the runner needs. The real implementation wraps the
// generated queries and a pool; tests use a fake.
type Store interface {
	// Claim leases up to batch live, unleased entries.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Jobs returns the persisted rows the documents are built from. A corrupted row
	// aborts the whole load, so the runner retries such a batch per item to isolate it.
	Jobs(ctx context.Context, ids []int64) ([]db.Job, error)
	// Complete deletes the outbox entries for a wave that was successfully indexed.
	Complete(ctx context.Context, entries []Claimed) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// Indexer pushes a batch of jobs into the live facet index.
type Indexer interface {
	// IndexBatch builds and pushes the jobs' documents as one Meili task.
	IndexBatch(ctx context.Context, jobs []db.Job) error
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// BatchSize is the claim wave size and the index-push batch size — the lever that
	// collapses per-board Meili tasks into one task per wave.
	BatchSize    int
	LeaseSeconds int
	MaxAttempts  int
	// CallTimeout bounds a single batch's (or fallback item's) index operation; 0
	// means no per-call timeout.
	CallTimeout time.Duration
}

// Stats reports what a run did.
type Stats struct {
	Indexed      int
	Failed       int
	DeadLettered int
}

// Runner drains the queue wave by wave until no claimable entries remain.
type Runner struct {
	Store   Store
	Indexer Indexer
}

// Run drains the queue until no claimable entries remain. A failure on a single
// entry is recorded and never aborts the run.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	rn := &run{store: r.Store, indexer: r.Indexer, opt: opt}
	for {
		batch, err := r.Store.Claim(ctx, opt.BatchSize, opt.LeaseSeconds)
		if err != nil {
			return rn.stats, fmt.Errorf("claim: %w", err)
		}
		if len(batch) == 0 {
			return rn.stats, nil
		}
		rn.processBatch(ctx, batch)
		log.Printf("search-drain: progress indexed=%d failed=%d dead=%d",
			rn.stats.Indexed, rn.stats.Failed, rn.stats.DeadLettered)
	}
}

// run accumulates one Run's options and tallies. Waves are processed sequentially,
// so the tallies need no lock.
type run struct {
	store   Store
	indexer Indexer
	opt     RunOptions
	stats   Stats
}

// processBatch indexes a whole wave in one batch and completes it in one call. Any
// batch-level failure (a corrupted-row load, a batch index error, a partial load)
// falls back to per-item processing so one bad entry can't sink the wave.
func (rn *run) processBatch(ctx context.Context, entries []Claimed) {
	if len(entries) == 0 {
		return
	}
	start := time.Now()
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	jobs, err := rn.store.Jobs(callCtx, jobIDs(entries))
	if err != nil {
		if rn.skipOnTimeout(callCtx, entries, "load jobs") {
			return
		}
		rn.fallback(ctx, entries)
		return
	}
	if len(jobs) != len(entries) {
		rn.fallback(ctx, entries)
		return
	}
	if err := rn.indexer.IndexBatch(callCtx, jobs); err != nil {
		if rn.skipOnTimeout(callCtx, entries, "index") {
			return
		}
		rn.fallback(ctx, entries)
		return
	}
	if err := rn.store.Complete(callCtx, entries); err != nil {
		if rn.skipOnTimeout(callCtx, entries, "complete") {
			return
		}
		rn.fallback(ctx, entries)
		return
	}
	rn.stats.Indexed += len(entries)
	log.Printf("search-drain: indexed batch of %d in %s", len(entries), since(start))
}

// skipOnTimeout reports whether callCtx expired — a normal-but-slow operation simply
// outran CallTimeout, not a per-document defect Meilisearch reported — and, if so,
// logs and leaves the wave claimed for its lease to expire, so a later run retries the
// WHOLE batch fresh. Falling back to per-item on a mere timeout would be actively
// harmful here: this Meili index's cost is dominated by a fixed whole-index re-merge,
// so a single document costs about as much to push as the whole batch (see the package
// doc), and per-item fallback would turn one slow-but-fine batch into up to
// len(entries) equally slow calls — exactly what produced the 2026-08-05 outage this
// guards against.
func (rn *run) skipOnTimeout(callCtx context.Context, entries []Claimed, stage string) bool {
	if !errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return false
	}
	log.Printf("search-drain: batch of %d timed out during %s after %s — leaving claimed for "+
		"lease-expiry retry, not falling back to per-item", len(entries), stage, rn.opt.CallTimeout)
	return true
}

func (rn *run) fallback(ctx context.Context, entries []Claimed) {
	for _, e := range entries {
		rn.processOne(ctx, e)
	}
}

func (rn *run) processOne(ctx context.Context, entry Claimed) {
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	jobs, err := rn.store.Jobs(callCtx, []int64{entry.JobID})
	if err != nil {
		// A corrupted row (XX001) can never load — dead-letter it immediately rather
		// than burning the attempt budget across cron runs (mirrors embed/enrich).
		if pgerr.IsDataCorrupted(err) {
			rn.failN(entry, fmt.Errorf("load job: %w", err), 1)
			return
		}
		rn.fail(entry, fmt.Errorf("load job: %w", err))
		return
	}
	if len(jobs) == 0 {
		// A missing row (hard-deleted, or excluded from the claim because it closed
		// or became a repost) is just as deterministic as a corrupted one — no future
		// run can load it either. Dead-letter immediately rather than burning the
		// attempt budget across cron runs.
		rn.failN(entry, fmt.Errorf("job %d not found", entry.JobID), 1)
		return
	}
	if err := rn.indexer.IndexBatch(callCtx, jobs); err != nil {
		rn.fail(entry, fmt.Errorf("index: %w", err))
		return
	}
	if err := rn.store.Complete(callCtx, []Claimed{entry}); err != nil {
		rn.fail(entry, fmt.Errorf("complete: %w", err))
		return
	}
	rn.stats.Indexed++
}

// callContext derives the per-batch timeout context (no-op when CallTimeout is 0).
func (rn *run) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if rn.opt.CallTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, rn.opt.CallTimeout)
}

func (rn *run) fail(entry Claimed, cause error) {
	rn.failN(entry, cause, rn.opt.MaxAttempts)
}

// failN records a failure with an explicit attempt ceiling. fail uses the run's
// MaxAttempts; the corrupted/missing-row paths pass 1 to force an immediate
// dead-letter.
func (rn *run) failN(entry Claimed, cause error, maxAttempts int) {
	// Fail bookkeeping runs on the run's background context, not the per-call one: a
	// timed-out/cancelled call must still record its own failure.
	dead, err := rn.store.Fail(context.Background(), entry.OutboxID, cause.Error(), maxAttempts)
	if err != nil {
		log.Printf("search-drain: outbox=%d fail-bookkeeping error: %v", entry.OutboxID, err)
	}
	if err == nil && dead {
		rn.stats.DeadLettered++
		return
	}
	rn.stats.Failed++
}

func jobIDs(entries []Claimed) []int64 {
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.JobID
	}
	return ids
}

func since(t time.Time) time.Duration { return time.Since(t).Round(time.Millisecond) }
