## Why

Nobody knows what the assistant costs.

`internal/assistant/runner.go` already reads token counts off every model call and
emits them as a `usage` SSE frame. The browser receives them and throws them away.
Nothing is written down, so there is no answer to "what did last week cost", "which
preset is expensive", or "is one account responsible for most of it".

`internal/credits` — a per-user points ledger with grants, debits, periods and an
idempotency key — has existed since the résumé match and CV tailoring shipped. The
assistant was never wired to it. Its own AGENTS.md says so outright: *"A turn is free
to the caller and billed to us... nothing bounds the spend now. `internal/credits` is
the seam for a per-turn debit."*

Two things since then made that gap wider rather than narrower. Dictation added a path
billed per minute of audio against a real paid key. Follow-ups added a model call per
settled turn. Both are bounded only by rate limits, which bound *frequency*, not *cost* —
a rate limit cannot tell a one-round question from a thirty-round autopilot pass.

This change does the half that blocks nothing: **write down what a turn cost.** It
changes no behaviour, refuses nobody, and produces the numbers any pricing decision needs
first. Charging for turns is deliberately a separate change, because a credit price
chosen before seeing the distribution is a guess.

## What Changes

- Every completed turn records what it spent: the session, the preset, the model, the
  rounds it took, and the input/output tokens summed across those rounds.
- The record is written whether the turn ended with an answer, at the step ceiling, or in
  error. A turn that failed after twenty rounds is the most expensive kind and the one
  worst served by recording nothing.
- Cancelled turns record what was spent before the caller walked away, because the
  provider billed for it.
- `GET /api/v1/me/usage` reports the caller's own totals for the current period.
- A `meta` block on the assistant's own admin/insights read (or a small worker query —
  see design) can answer the per-preset question without a per-user scan.
- **No refusal, no gate, no credit debit.** The endpoint that runs a turn behaves exactly
  as it does today.

## Capabilities

### New Capabilities

- `assistant-turn-accounting`: recording what one turn spent, and reporting it back to the
  caller.

### Modified Capabilities

None. `assistant-agent-runtime` describes the turn loop's contract with its client — the
events, the bounds, the terminal `result`. Recording a row after the loop finishes adds no
requirement to it and removes none. If the follow-up change adds a *refusal*, that is when
that spec changes.

## Impact

**New code.** `internal/assistant/usage.go` (the aggregate and its store), a migration
adding `assistant_turn_usage`, `internal/db/queries/assistant_usage.sql` plus `make sqlc`,
and a handler for `/me/usage`.

**Modified code.** `internal/assistant/runner.go` — the loop must accumulate per-round
usage and hand the total to the store on every terminal path. `internal/handler/assistant.go`
wires the store.

**Schema.** One new table. Expand-only, no backfill, nothing reads it until the endpoint
ships. Note for the deploy: `release.sh` on host-2 **does** run `cmd/migrate` (observed
2026-07-31: `migrate: 86 file(s) on disk, 0 baselined, 0 applied`), which contradicts the
freehire-ops README claiming bare metal has no migration runner. Confirm on a run that
actually applies something before relying on it.

**Not in scope, deliberately:** debiting `credit_ledger`, refusing a turn on an empty
balance, a price per turn, and the `feature IN ('match','tailor')` CHECK that would have to
grow a third value. All of that belongs to the change this one makes possible.
