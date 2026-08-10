package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClaimer replays a fixed sequence of waves, then returns empty forever —
// the shape every real Store.Claim has (a lease-bounded batch, empty when drained).
type fakeClaimer struct {
	mu             sync.Mutex
	waves          [][]int
	calls          int
	requestedBatch []int
}

func (f *fakeClaimer) Claim(ctx context.Context, batch, leaseSeconds int) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestedBatch = append(f.requestedBatch, batch)
	if f.calls >= len(f.waves) {
		return nil, nil
	}
	w := f.waves[f.calls]
	f.calls++
	return w, nil
}

func TestRunPool_ProcessesAllItemsUntilClaimEmpty(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}, {4, 5}}}

	var mu sync.Mutex
	processed := map[int]bool{}
	process := func(ctx context.Context, item int) Outcome {
		mu.Lock()
		processed[item] = true
		mu.Unlock()
		return Succeeded
	}

	stats, err := RunPool(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60, Concurrency: 2}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 5 {
		t.Errorf("Succeeded = %d, want 5", stats.Succeeded)
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if !processed[id] {
			t.Errorf("item %d was never processed", id)
		}
	}
}

func TestRunPool_RunsUpToConcurrencyItemsInParallel(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}

	arrived := make(chan int, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	doRelease := func() { releaseOnce.Do(func() { close(release) }) }
	defer doRelease()

	process := func(ctx context.Context, item int) Outcome {
		arrived <- item
		<-release
		return Succeeded
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = RunPool(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60, Concurrency: 3}, process)
	}()

	// All 3 items must arrive concurrently — proves they are not serialized one
	// at a time behind each other's <-release wait.
	for i := 0; i < 3; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for concurrent arrivals; got %d/3", i)
		}
	}
	doRelease()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunPool did not return after release")
	}
}

func TestRunPool_MaxPerRunStopsAndShrinksNextClaim(t *testing.T) {
	// Wave two carries only 1 item — a real Store.Claim would truncate to the
	// shrunk batch size; this fake just needs the data shaped for it.
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}, {4}}}
	process := func(ctx context.Context, item int) Outcome { return Succeeded }

	stats, err := RunPool(context.Background(), claimer, RunOptions{
		BatchSize:    3,
		LeaseSeconds: 60,
		Concurrency:  1,
		MaxPerRun:    4,
	}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 4 {
		t.Errorf("Succeeded = %d, want 4", stats.Succeeded)
	}
	if claimer.calls != 2 {
		t.Errorf("claim calls = %d, want 2 (must stop before a 3rd claim once budget is spent)", claimer.calls)
	}
	wantRequested := []int{3, 1}
	if len(claimer.requestedBatch) != len(wantRequested) {
		t.Fatalf("requestedBatch = %v, want %v", claimer.requestedBatch, wantRequested)
	}
	for i, want := range wantRequested {
		if claimer.requestedBatch[i] != want {
			t.Errorf("requestedBatch[%d] = %d, want %d (should shrink to the remaining budget)", i, claimer.requestedBatch[i], want)
		}
	}
}

func TestRunPool_ConcurrencyOneProcessesInStrictSubmissionOrder(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}

	// order is deliberately unsynchronized: any concurrent access here is a data
	// race (catch with `go test -race`), which a true sequential loop can't trigger.
	var order []int
	process := func(ctx context.Context, item int) Outcome {
		if item == 1 {
			// If items ran concurrently, 2 and 3 would very likely finish first.
			time.Sleep(20 * time.Millisecond)
		}
		order = append(order, item)
		return Succeeded
	}

	_, err := RunPool(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60, Concurrency: 1}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 2, 3}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i, id := range want {
		if order[i] != id {
			t.Errorf("order[%d] = %d, want %d", i, order[i], id)
		}
	}
}

// Runner has no built-in context-cancellation handling (see the Claimer doc comment
// for why); a Claimer that wants a clean stop on cancellation returns an empty batch,
// which this test's claim-until-empty contract already covers generically —
// TestRunPool_ProcessesAllItemsUntilClaimEmpty exercises the exact same path a
// cancellation-aware Claimer would take.

func TestRunBatch_SucceedsWholeWaveViaOneBatchCall(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}
	var batchCalls, itemCalls int

	processBatch := func(ctx context.Context, items []int) error {
		batchCalls++
		return nil
	}
	processOne := func(ctx context.Context, item int) Outcome {
		itemCalls++
		return Succeeded
	}

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if batchCalls != 1 {
		t.Errorf("batchCalls = %d, want 1", batchCalls)
	}
	if itemCalls != 0 {
		t.Errorf("itemCalls = %d, want 0 (fallback must not run when the batch call succeeds)", itemCalls)
	}
	if stats.Succeeded != 3 {
		t.Errorf("Succeeded = %d, want 3", stats.Succeeded)
	}
}

func TestRunBatch_FallsBackToPerItemOnBatchFailure(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}

	processBatch := func(ctx context.Context, items []int) error {
		return errors.New("meili down")
	}
	var mu sync.Mutex
	seen := map[int]bool{}
	processOne := func(ctx context.Context, item int) Outcome {
		mu.Lock()
		seen[item] = true
		mu.Unlock()
		if item == 2 {
			return Failed
		}
		return Succeeded
	}

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seen) != 3 {
		t.Errorf("per-item fallback saw %d items, want 3", len(seen))
	}
	if stats.Succeeded != 2 || stats.Failed != 1 {
		t.Errorf("stats = %+v, want Succeeded=2 Failed=1", stats)
	}
}
