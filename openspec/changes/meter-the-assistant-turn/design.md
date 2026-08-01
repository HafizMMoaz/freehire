## Context

`Runner.Run` is a bounded loop. Each pass calls `llm.Chat`, and `emitUsage` reads the
provider's counts off the returned choice and emits a `usage` event. It is called from two
places — inside the loop for a tool-calling round (`runner.go:170`) and after it for the
answering call (`runner.go:193`) — so **one turn already emits N usage frames, one per model
call.** Nothing sums them and nothing stores them.

`internal/credits` is a working ledger: `credit_ledger` append-only with `kind`, `feature`,
`delta`, `period` and a `ref`; `credit_balances` a materialized cache read on the hot debit
path; `Debit` idempotent through `DebitExists(user, feature, ref)`. Two call sites use it
(`cv_tailor.go:109`, `match_analysis.go:263`). The assistant is not one of them.

The temptation is to jump straight to `Debit(userID, "assistant", turnRef)`. This change
deliberately does not, for a reason that is not caution but arithmetic: nobody knows what a
turn costs, so nobody can choose what it should cost. A turn ranges from one round of chat
to a thirty-round autopilot pass. Pricing that spread without the distribution is guessing,
and a guess that refuses people is expensive to be wrong about.

## Goals / Non-Goals

**Goals:**

- One durable row per turn, on every terminal path, with rounds and tokens.
- Answer "what does a turn cost, by preset, by model, over a period" from one table.
- Show a caller their own usage.
- Add no failure mode to the turn.

**Non-Goals:**

- Debiting credits, refusing a turn, or pricing one. That is the next change, and this one
  exists to inform it.
- Per-round rows. The round is not the unit anyone would price or read.
- Counting tokens ourselves when the provider reports none. A local tokenizer would
  disagree with the invoice, and a number that looks authoritative and is not is worse than
  a zero next to a truthful round count.
- Dictation minutes. Transcription is billed by audio duration on a different endpoint;
  it deserves its own row shape and is not squeezed in here.

## Decisions

### One row per turn, written once at the end

The loop accumulates into a small value — rounds, input, output — and hands it to the store
on its way out. Not a row per model call: the caller asked for one thing, a price would
attach to one thing, and the per-round detail is loop mechanics that would triple the table
for a question nobody asks.

*Alternative considered:* writing a row per round and aggregating in SQL. Truer to what the
provider bills, and it survives a crash mid-turn with partial data. Rejected because every
read then needs a GROUP BY, and the crash case is exactly the case the terminal-path rule
already covers — the loop emits a `result` on every path, so there is always a moment to write.

### Written on every terminal path, including error and cancellation

A turn that failed after twenty rounds is the most expensive kind. Recording only successes
would make the table systematically understate the bill, and in the direction that matters.

Cancellation is the subtle one. Aborting the fetch stops the loop before the next model
call, but the calls already made were billed. The record is written from whatever the loop
accumulated, so a cancelled turn tells the truth about what it spent rather than vanishing.

### The turn's identity is its idempotency key

`(session_id, seq)` of the assistant message the turn produced — a UNIQUE constraint, and
the insert is `ON CONFLICT DO NOTHING`. This mirrors `credits.Debit`, which is idempotent
through `DebitExists(user, feature, ref)` for the same reason: a retry must not double-count.

It also means the ref is already in the right shape for the debit the next change will add,
so that change does not have to invent one or backfill.

### Failure to record is a log line

The write happens after the terminal event, in the same goroutine, and its error is logged
and dropped. A turn that answered correctly must not be reported as failed because a
bookkeeping insert lost a connection. This is the same rule the follow-up endpoint follows
and for the same reason.

`Runner` takes the store as an optional dependency, nil meaning "do not record" — the
pattern `internal/speech` and `internal/headshot` already use, so tests and any embedding
that does not care are unaffected.

### The table

```sql
CREATE TABLE assistant_turn_usage (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id    uuid   NOT NULL REFERENCES assistant_sessions(id) ON DELETE CASCADE,
    seq           integer NOT NULL,          -- of the assistant message this turn produced
    preset        text   NOT NULL,
    model         text   NOT NULL,
    rounds        integer NOT NULL,
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    stop_reason   text   NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, seq)
);
CREATE INDEX ON assistant_turn_usage (user_id, created_at DESC);
CREATE INDEX ON assistant_turn_usage (created_at);
```

`preset` and `model` are **denormalised on purpose.** The preset is on the session and the
model is in configuration, but both can change under a row that is a historical fact:
`ASSISTANT_MODEL` moved from `privateclaw/mid` to `privateclaw/flagship` at some point, and a
join would silently reattribute every past turn to today's model. A cost record that changes
when configuration changes is not a cost record.

No CHECK on `preset`: `assistant_sessions.preset` already constrains the vocabulary at the
source, and a second CHECK here means adding a preset becomes two migrations that can
disagree.

`ON DELETE CASCADE` on both keys, matching how the rest of the assistant's tables treat a
deleted account and a deleted conversation. This does mean **deleting a conversation erases
its cost history**, which is the right default for a per-user product and the wrong one for
a finance report; if the numbers ever need to outlive the conversation, that is a deliberate
change to make then, not a default to set now.

### `/me/usage` reports the period, not the lifetime

Scoped to the current period using the same `periodKey` shape `internal/credits` uses, so
the two agree about what "this month" means and the next change does not have to reconcile
two calendars.

## Risks / Trade-offs

- **A provider that never reports tokens makes the table look free** → rounds are counted by
  us and are always right, so the fallback signal survives; and the spec pins the
  zero-token-with-true-rounds case as a scenario rather than leaving it to chance.
- **Denormalised `preset`/`model` drift from the session** → intended; documented above and
  in the AGENTS.md addition, because the drift IS the historical record.
- **Deleting a conversation erases its usage** → accepted for now, called out above.
- **The write is on the turn's own path** → it is a single-row insert after the client has
  its answer, and a failure is dropped; the turn cannot be slowed by more than one insert or
  broken by any of it.
- **This change measures and does not bound** → true, and stated. The bound is the next
  change; the rate limits and the recording ceiling from the voice work stay in the meantime.

## Migration Plan

One migration (next free number — `0066` was the last on `origin/main` when this was
written; re-check, migration numbers have collided in this repo before). Expand-only: a new
table nothing reads until the endpoint ships, so it can be applied ahead of the deploy with
no ordering hazard.

`release.sh` on host-2 appeared to run `cmd/migrate` on the 2026-07-31 release
(`migrate: 86 file(s) on disk, 0 baselined, 0 applied`), which contradicts the freehire-ops
README. Confirm on a release that actually applies a file; if the README is right, apply by
hand before the flip.

Rollback is dropping the table. Nothing else reads it.

## Open Questions

- Does `/me/usage` belong on a page, or is it enough that the number exists for us? The
  spec requires the endpoint; whether the SPA renders it is a product call. Building it
  first as an API costs nothing either way.
- Should a turn started by autopilot be distinguishable from one the caller typed? The
  preset gives most of it (`tailor` covers both), but the unattended run is the expensive
  shape and might deserve its own marker. Cheap to add now, awkward to backfill later.
