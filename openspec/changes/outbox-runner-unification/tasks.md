## 1. Foundation: `internal/outbox`

- [x] 1.1 Define `Outcome`, `Stats`, `RunOptions`, `Claimer[C]`, `Processor[C]`,
      `BatchProcessor[C]` per the design doc's API shape
      (`docs/superpowers/specs/2026-08-09-outbox-runner-unification-design.md`).
- [x] 1.2 Implement `RunPool[C any]`: bounded-concurrency drain when
      `Concurrency > 1`; a plain sequential loop (no goroutine spawned) when
      `Concurrency <= 1`, honoring `MaxPerRun` (shrinks the next claim size,
      stops before claiming past the budget). No built-in context-cancellation
      handling — deliberately caller-owned; see the `Claimer` doc comment in
      `internal/outbox/runner.go` and the design doc's Decisions section.
- [x] 1.3 Implement `RunBatch[C any]`: one `BatchProcessor` call per wave,
      falling back to per-item `Processor` calls only on a batch-level error;
      honors `MaxPerRun` symmetrically with `RunPool`.
- [x] 1.4 Unit-test `internal/outbox` with a fake `Claimer`/`Processor`/
      `BatchProcessor`: claim-until-empty termination, `MaxPerRun` budget
      shrinking (both entry points), claim-error abort (both entry points),
      `RunPool` sequential-vs-pooled branching (incl. strict in-order
      processing at `Concurrency<=1`), `RunBatch` fallback-on-failure.
      Reviewed via requesting-code-review; two Important findings (RunBatch's
      missing `MaxPerRun`, untested claim-error path) fixed before completion.

## 2. Migrate `internal/enrich` (RunPool)

- [x] 2.1 Wrap `enrich`'s existing `Store.Claim`/per-entry processing into a
      `Claimer[enrich.Claimed]` + `Processor[enrich.Claimed]` closure, preserving
      the `maxAttempts=1` immediate dead-letter for a corrupted row.
      (`enrich.Store.Claim` already satisfies `outbox.Claimer[Claimed]`
      structurally — no adapter type was needed.)
- [x] 2.2 Replace `enrich`'s hand-rolled claim-wave loop with a call into
      `outbox.RunPool`. Required adding `outbox.RunOptions.OnWave` (not in the
      original task scope) to preserve the per-wave progress-heartbeat log line
      — see the design.md Decisions entry added during task group 1's revisit.
- [x] 2.3 Shrink `internal/enrich/runner_test.go` to package-specific logic only
      (`Sanitize`/`Validate` wiring, corrupted-row dead-letter path); loop
      mechanics now live in `internal/outbox`'s own tests.
      `TestRun_waveDrainsConcurrently` removed, subsumed by
      `TestRunPool_RunsUpToConcurrencyItemsInParallel`. Reviewed via
      requesting-code-review: ready to merge, one cosmetic naming nit fixed.

## 3. Migrate `internal/embed` (RunBatch)

- [x] 3.1 Wrap `embed`'s open/closed batch indexing into a
      `Claimer[embed.Claimed]` + `BatchProcessor[embed.Claimed]` +
      `Processor[embed.Claimed]` (per-item fallback) closure.
      Required changing `outbox.BatchProcessor`'s contract from bare `error`
      to `(Stats, error)` (see design.md) — embed's own open/closed split
      already does its own internal batch-then-fallback per group, so it
      always returns a nil error with an accurate per-wave `Stats` delta;
      `Processor[embed.Claimed]` is wired but structurally unreachable
      (`unreachableFallback`, panics if ever called — confirmed via review
      that this is safe under `worker.Main`'s panic-capture wrapper).
- [x] 3.2 Replace `embed`'s hand-rolled claim-wave loop with a call into
      `outbox.RunBatch`.
- [x] 3.3 Shrink `internal/embed/runner_test.go` to package-specific logic only
      (open/closed branching, vector-provenance stamping on complete).
      No tests removed — embed never had a pure loop-mechanics test to begin
      with; all 6 existing tests were already domain-specific. Reviewed via
      requesting-code-review together with the BatchProcessor contract
      change: ready to merge, no blocking issues.

## 4. Migrate `internal/searchdrain` (RunBatch)

- [ ] 4.1 Wrap `searchdrain`'s wave-push indexing into a
      `Claimer[searchdrain.Claimed]` + `BatchProcessor` + `Processor` (per-item
      fallback) closure, preserving the `skipOnTimeout` no-fallback-on-context-
      timeout behavior.
- [ ] 4.2 Replace `searchdrain`'s hand-rolled claim-wave loop with a call into
      `outbox.RunBatch`.
- [ ] 4.3 Shrink `internal/searchdrain/runner_test.go` to package-specific logic
      only (document building, `skipOnTimeout` branching).

## 5. Migrate `internal/applyform` (RunPool)

- [ ] 5.1 Wrap `applyform`'s per-capture fetch+save into a
      `Claimer[applyform.Claimed]` + `Processor[applyform.Claimed]` closure,
      preserving `ErrPostingGone` → `Discard` and the `MaxPerRun` backlog bound.
- [ ] 5.2 Replace `applyform`'s hand-rolled claim-wave loop (including its
      `MaxPerRun`-aware batch-size shrinking) with a call into `outbox.RunPool`.
- [ ] 5.3 Shrink `internal/applyform/runner_test.go` to package-specific logic
      only (`ErrPostingGone` → `Discard` mapping, `RunStats.Degraded()`
      heuristic); confirm `Degraded()` still derives correctly from the
      `outbox.Stats` embedded in `applyform.RunStats`.

## 6. Migrate `internal/adzunadesc` (RunPool)

- [ ] 6.1 Wrap `adzunadesc`'s per-item fetch+hydrate (including its chained
      `EnqueueSearchOutbox` on success) into a `Claimer[adzunadesc.Claimed]` +
      `Processor[adzunadesc.Claimed]` closure.
- [ ] 6.2 Replace `adzunadesc`'s hand-rolled claim-wave loop with a call into
      `outbox.RunPool`.
- [ ] 6.3 Shrink `internal/adzunadesc/runner_test.go` to package-specific logic
      only.

## 7. Migrate `internal/maillink` (RunPool, sequential)

- [ ] 7.1 Add a `Claim(ctx, batch, leaseSeconds)`-ordered adapter over
      `Store.ClaimBatch(ctx, leaseSeconds, batchSize)` (argument order differs)
      so `maillink` satisfies `Claimer[maillink.Claimed]`.
- [ ] 7.2 Wrap `maillink`'s per-email classify+link into a
      `Processor[maillink.Claimed]` closure, preserving the `appCache` per-wave
      reuse.
- [ ] 7.3 Replace `maillink`'s hand-rolled sequential loop with a call into
      `outbox.RunPool` at `Concurrency: 1`, confirming the sequential (no
      goroutine) path is what actually runs.
- [ ] 7.4 Shrink `internal/maillink/runner_test.go` to package-specific logic
      only (thread-continuity ordering, `appCache` behavior).

## 8. Verification

- [ ] 8.1 `go build ./...` and `go vet ./...` pass.
- [ ] 8.2 `go vet -tags=integration ./...` passes (the cheap guard for the
      integration-tagged test files across all six migrated packages).
- [ ] 8.3 `go test ./...` passes, including `internal/outbox` and all six
      migrated packages' shrunk test suites.
- [ ] 8.4 Spot-check one worker's log output (e.g. `go run ./cmd/enrich`
      against a local DB) to confirm progress-heartbeat and final-stats log
      lines are unchanged in content and format.
