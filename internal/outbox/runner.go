// Package outbox is the shared claim-wave drain loop behind every Postgres outbox
// queue worker in this repo. It owns the loop, not the queue: claim a wave, process
// it, tally the outcome, repeat until a claim comes back empty. Each caller keeps its
// own Store/Indexer/Provider port and its own Complete/Fail/Discard logic; Runner
// never touches persistence itself.
package outbox

import (
	"context"
	"sync"
)

// Outcome is what a Processor did with one claimed item. The caller has already made
// whatever Store call follows from it (Complete, Fail, Discard); Runner only tallies.
type Outcome int

const (
	Succeeded Outcome = iota
	Failed
	DeadLettered
	Discarded
)

// Stats is what one Run did.
type Stats struct {
	Succeeded    int
	Failed       int
	DeadLettered int
	Discarded    int
}

// Claimer leases up to batch live, unleased entries — the shape every package's
// existing Store.Claim method already has.
//
// Deliberately absent: Runner has no built-in context-cancellation early-exit. Two of
// its four RunPool callers (applyform, adzunadesc) return (stats, nil) as soon as a
// wave finishes if ctx is cancelled, so a shutdown doesn't burn the rest of a large
// backlog's retry attempts; the other two (enrich, maillink) let a cancelled ctx
// surface as an ordinary error from the next Claim call. Baking either behavior into
// Runner would silently change the other callers' exit code on SIGTERM. A caller that
// wants the early-exit implements it in its own Claim wrapper: check ctx.Err() first
// and return (nil, nil) instead of querying — Runner's normal empty-batch stop then
// reproduces the exact same outcome.
type Claimer[C any] interface {
	Claim(ctx context.Context, batch, leaseSeconds int) ([]C, error)
}

// RunOptions are the per-run knobs. MaxAttempts and any per-call timeout are
// deliberately absent here — they govern a single Processor call's own retry/timeout
// behavior, which stays inside each caller's closure alongside its Store calls, not in
// the shared loop.
type RunOptions struct {
	BatchSize    int
	LeaseSeconds int
	// Concurrency is RunPool-only; RunBatch ignores it (its concurrency lives inside
	// whatever the caller's BatchProcessor does in one call).
	Concurrency int
	// MaxPerRun bounds how much of the backlog one Run takes, shared by RunPool and
	// RunBatch; 0 is unbounded.
	MaxPerRun int
}

// Processor handles one claimed item end to end, including whatever Store.Complete /
// Store.Fail / Store.Discard call follows, and reports what happened.
type Processor[C any] func(ctx context.Context, item C) Outcome

// RunPool drains via a bounded pool when opt.Concurrency > 1. At Concurrency <= 1 it
// processes the wave as a plain sequential loop — no goroutine is spawned at all — so
// a caller that needs in-order processing (internal/maillink, where a later message in
// a thread must see the same wave's earlier link) gets a real ordering guarantee: Go's
// channel semantics between blocked senders are not formally FIFO, so a size-1
// semaphore-gated pool would not have been a safe substitute for a literal loop.
func RunPool[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, process Processor[C]) (Stats, error) {
	var stats Stats
	for {
		batchSize, budgetSpent := nextBatchSize(opt, stats)
		if budgetSpent {
			return stats, nil
		}
		batch, err := claimer.Claim(ctx, batchSize, opt.LeaseSeconds)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		if opt.Concurrency <= 1 {
			for _, item := range batch {
				tally(&stats, process(ctx, item))
			}
		} else {
			runPoolWave(ctx, batch, opt.Concurrency, process, &stats)
		}
	}
}

// BatchProcessor handles a whole wave in one call (e.g. one Meilisearch bulk push).
// A nil error means every item in the wave succeeded and was already completed by
// the call; a non-nil error means the whole wave failed and RunBatch falls back to
// processing it item by item through Processor.
type BatchProcessor[C any] func(ctx context.Context, items []C) error

// RunBatch drains via one BatchProcessor call per wave, falling back to per-item
// Processor calls only when the batch call itself fails — so one poison/corrupted
// item can't sink an otherwise-healthy wave.
func RunBatch[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, processBatch BatchProcessor[C], processOne Processor[C]) (Stats, error) {
	var stats Stats
	for {
		batchSize, budgetSpent := nextBatchSize(opt, stats)
		if budgetSpent {
			return stats, nil
		}
		batch, err := claimer.Claim(ctx, batchSize, opt.LeaseSeconds)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		if err := processBatch(ctx, batch); err != nil {
			for _, item := range batch {
				tally(&stats, processOne(ctx, item))
			}
			continue
		}
		stats.Succeeded += len(batch)
	}
}

func runPoolWave[C any](ctx context.Context, batch []C, concurrency int, process Processor[C], stats *Stats) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, concurrency)

	for _, item := range batch {
		wg.Add(1)
		go func(it C) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			outcome := process(ctx, it)
			mu.Lock()
			tally(stats, outcome)
			mu.Unlock()
		}(item)
	}
	wg.Wait()
}

// nextBatchSize applies opt.MaxPerRun (shared by RunPool and RunBatch) to
// opt.BatchSize, reporting whether the budget is already spent — in which case the
// caller must stop without claiming again.
func nextBatchSize(opt RunOptions, stats Stats) (size int, budgetSpent bool) {
	if opt.MaxPerRun <= 0 {
		return opt.BatchSize, false
	}
	processed := stats.Succeeded + stats.Failed + stats.DeadLettered + stats.Discarded
	remaining := opt.MaxPerRun - processed
	if remaining <= 0 {
		return 0, true
	}
	return min(opt.BatchSize, remaining), false
}

func tally(stats *Stats, o Outcome) {
	switch o {
	case Succeeded:
		stats.Succeeded++
	case Failed:
		stats.Failed++
	case DeadLettered:
		stats.DeadLettered++
	case Discarded:
		stats.Discarded++
	}
}
