## ADDED Requirements

### Requirement: Every completed turn records what it spent

A turn SHALL write exactly one usage record when it ends, on every terminal path: an
answer, the step ceiling, cancellation, and failure. The record MUST carry the session it
belonged to, the preset it ran under, the model that ran it, how many model calls it took,
and the input and output tokens summed across those calls.

One record per turn, not one per model call. A turn is what a caller asks for and what a
price would attach to; the rounds inside it are an implementation detail of the loop.

#### Scenario: An answered turn is recorded

- **WHEN** a turn ends with `end_turn` after three model calls
- **THEN** one record exists for it, carrying three rounds and the summed token counts

#### Scenario: A failed turn is recorded too

- **WHEN** a turn ends in error after several rounds
- **THEN** its record exists and reports the error stop reason

#### Scenario: A cancelled turn records what was already spent

- **WHEN** the caller aborts the request mid-turn
- **THEN** a record exists carrying the rounds completed before the abort

#### Scenario: A turn stopped at the ceiling is recorded

- **WHEN** a turn reaches `MaxSteps` and answers under the forced final call
- **THEN** its record counts that final call among its rounds

### Requirement: A provider that reports no tokens still yields a record

Where the provider surfaces no token counts, the turn SHALL still be recorded, with zero
tokens and its true round count. Rounds are counted by this process and are never missing;
tokens come from the provider and sometimes are. A turn absent from the table would
otherwise be indistinguishable from a turn that never ran, and the round count is the
fallback signal for what a turn cost.

#### Scenario: No usage in the provider's response

- **WHEN** a turn completes and no model call reported token counts
- **THEN** a record exists with zero input and output tokens and the true number of rounds

### Requirement: Recording never affects the turn

A failure to record SHALL NOT fail the turn, alter any event the client receives, or delay
the terminal `result`. The caller has their answer; losing the bookkeeping for it is our
problem and belongs in a log.

#### Scenario: The store is unavailable

- **WHEN** writing the usage record fails
- **THEN** the turn's events are unchanged, the client sees no error, and the failure is logged

#### Scenario: No store configured

- **WHEN** the runner has no usage store
- **THEN** turns run exactly as they do today

### Requirement: A turn is recorded once

Recording SHALL be idempotent per turn: a retry of the write, or a second terminal path
reached by the same turn, MUST NOT produce a second row. The turn's own identity — its
session and the sequence of the assistant message it produced — is what makes it unique,
not the moment it was written.

#### Scenario: A repeated write is absorbed

- **WHEN** the same turn is recorded twice
- **THEN** the table holds one row for it

### Requirement: The caller can see their own usage

`GET /api/v1/me/usage` SHALL report the authenticated caller's assistant usage for the
current period: turns, rounds, and tokens. It MUST report only the caller's own, and MUST
answer a caller with no usage as zeroes rather than as an error or a 404.

#### Scenario: A caller with usage

- **WHEN** a caller who has run turns this period reads the endpoint
- **THEN** the response carries their turn count, round count and token totals

#### Scenario: A caller with none

- **WHEN** a caller who has never run a turn reads the endpoint
- **THEN** the response is `200` with zeroes

#### Scenario: Usage is owner-scoped

- **WHEN** two accounts have both run turns
- **THEN** each sees only its own totals

#### Scenario: The endpoint requires authentication

- **WHEN** the request carries no accepted credential
- **THEN** the response is `401`

### Requirement: The record supports asking which preset costs what

The stored shape SHALL allow totals to be grouped by preset and by model over a period
without reading the transcript. The question that prompted this change — which of chat,
tailoring, the rehearsal and the browsing panel is expensive — must be answerable from
this table alone.

#### Scenario: Grouping by preset

- **WHEN** turns have run under more than one preset
- **THEN** their totals can be summed per preset for a period
