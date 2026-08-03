## MODIFIED Requirements

### Requirement: On-demand LLM fit analysis

The system SHALL provide an authenticated `POST /api/v1/jobs/:slug/fit` endpoint that runs a fixed
three-stage LLM prompt-chain comparing the job (title + description), the company context, and the
caller's stored CV text, and returns a structured fit analysis. The chain MUST be a deterministic,
server-orchestrated sequence of typed calls — not an autonomous, self-directing agent. The analysis
MUST be scoped to the calling user and the job addressed by `:slug`. A private job (`is_private =
true`) not created by the calling user SHALL be treated identically to an unknown slug by this
endpoint and by its cached-read (`GET`) and streaming (`GET .../fit/stream`) counterparts.

#### Scenario: Signed-in user with a CV requests analysis

- **WHEN** a signed-in user with a stored CV and a saved profile POSTs to `/api/v1/jobs/:slug/fit` for an open job
- **THEN** the system runs the three-stage chain and responds `200` with `{ "data": { "has_cv": true, "analysis": <verdict> } }`

#### Scenario: Unknown job slug

- **WHEN** the caller POSTs to `/api/v1/jobs/:slug/fit` for a slug that does not exist
- **THEN** the system responds `404`

#### Scenario: Unauthenticated caller

- **WHEN** an unauthenticated request hits the fit endpoint
- **THEN** the system responds `401` and never invokes the LLM

#### Scenario: A private job's owner can run fit analysis on it

- **WHEN** the user who created a private job POSTs to `/api/v1/jobs/:slug/fit` for its slug
- **THEN** the system runs the three-stage chain as it would for any other job

#### Scenario: A private job is not found for anyone else

- **WHEN** a user who did not create a private job requests any of `GET /api/v1/jobs/:slug/fit`,
  `POST /api/v1/jobs/:slug/fit`, or `GET /api/v1/jobs/:slug/fit/stream` for its slug
- **THEN** the system responds `404` and never invokes the LLM
