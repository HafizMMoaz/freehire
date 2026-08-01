## 1. The record

- [ ] 1.1 Add the migration creating `assistant_turn_usage` (schema in design.md). Take the next free number after re-checking `origin/main` — numbers have collided here before — and mirror it into the sqlc source of truth.
- [ ] 1.2 Add `internal/db/queries/assistant_usage.sql`: the `ON CONFLICT (session_id, seq) DO NOTHING` insert, the per-caller period rollup, and the per-preset rollup. Run `make sqlc` and commit the generated diff.
- [ ] 1.3 Add `internal/assistant/usage.go` with the accumulator (`rounds`, `input`, `output`, `add(llm.Usage)`) and the store's `Record` method; unit-test the accumulator over several rounds, over rounds that reported nothing, and over zero rounds.

## 2. Wiring the loop

- [ ] 2.1 Make `Runner` take an optional usage store (nil = do not record), following the `internal/speech` nil-means-absent pattern.
- [ ] 2.2 Accumulate usage where `emitUsage` already reads it — both call sites, `runner.go:170` and `:193` — without changing what is emitted.
- [ ] 2.3 Record on every terminal path: answer, step ceiling, cancellation, failure. Prove it with a test per path against a scripted model, asserting one row each and the right round count.
- [ ] 2.4 Test that a failing store leaves the turn's events untouched and the terminal `result` unchanged, and that a nil store changes nothing at all.
- [ ] 2.5 Test that recording the same turn twice leaves one row.

## 3. The endpoint

- [ ] 3.1 Add `GET /api/v1/me/usage` returning the caller's turns, rounds and tokens for the current period, using the same `periodKey` shape `internal/credits` uses.
- [ ] 3.2 Handler tests: a caller with usage, a caller with none (200 and zeroes, not 404), owner scoping across two accounts, and 401 without a credential.
- [ ] 3.3 Integration test against a real turn: run one through the scripted model, then read the endpoint and see it.

## 4. Close out

- [ ] 4.1 Update `internal/assistant/AGENTS.md`: replace the "No metering" limitation with what is now true — the turn is recorded, still not bounded — and write down why `preset` and `model` are denormalised, because that reads as a mistake until someone explains that the drift is the point.
- [ ] 4.2 `go build ./... && go vet ./... && gofmt -l .`, `go test ./...`, `go test -tags=integration ./internal/handler/`.
- [ ] 4.3 Verify against the spec's scenarios, deploy, then let it run a week before touching pricing — the whole point of this change is the distribution it produces.

## 5. Deliberately not here

The debit, the refusal, the price, and growing `credit_ledger`'s `feature IN ('match','tailor')`
CHECK to admit a third value. Open that change once this table has a week of data in it.
