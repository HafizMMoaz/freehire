## MODIFIED Requirements

### Requirement: Slug-based public routing

Public job endpoints SHALL address jobs by `public_slug` rather than by the
numeric id. A request for an unknown slug SHALL return 404. A request for a slug belonging to a
private job (`is_private = true`) SHALL also return 404 unless the requester is the job's
`created_by` user — a private job is indistinguishable from an unknown one to anyone else,
including an anonymous caller.

#### Scenario: Fetch a job by slug

- **WHEN** a client requests `GET /api/v1/jobs/:slug` with an existing slug
- **THEN** the response is the matching job under `{"data": ...}`

#### Scenario: Unknown slug

- **WHEN** a client requests `GET /api/v1/jobs/:slug` with a slug that matches no
  job
- **THEN** the response status is 404

#### Scenario: Record a view by slug

- **WHEN** an authenticated client sends `POST /api/v1/jobs/:slug/view`
- **THEN** the slug is resolved to the job's internal id and the view is recorded
  in `user_jobs` for that (user, job)

#### Scenario: Mark applied by slug

- **WHEN** an authenticated client sends `POST /api/v1/jobs/:slug/apply`
- **THEN** the slug is resolved to the job's internal id and `applied_at` is set
  in `user_jobs` for that (user, job)

#### Scenario: A private job's owner can fetch it by slug

- **WHEN** the user who created a private job requests `GET /api/v1/jobs/:slug` for its slug
- **THEN** the response is that job under `{"data": ...}`

#### Scenario: A private job is unreachable by anyone else

- **WHEN** a different authenticated user, or an anonymous caller, requests
  `GET /api/v1/jobs/:slug` for a private job's slug
- **THEN** the response status is 404, identical to an unknown slug
