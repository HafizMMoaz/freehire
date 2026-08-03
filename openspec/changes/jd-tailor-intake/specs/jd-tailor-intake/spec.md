## ADDED Requirements

### Requirement: JD resolution endpoint

The system SHALL expose an authenticated `POST /api/v1/me/jd/resolve` endpoint accepting exactly
one of `{job_slug}`, `{url}`, or `{text, title?, company?}`, and SHALL respond with `{job_slug}`
identifying a `jobs` row usable by the existing tailoring workspace. Submitting more than one of
`job_slug`/`url`/`text`, or none, SHALL be rejected as a client error.

#### Scenario: Resolving an existing catalog slug is a no-op

- **WHEN** an authenticated user submits `{job_slug}` for a job already in the catalog
- **THEN** the response returns that same `job_slug`, and no new `jobs` row is created

#### Scenario: Submitting more than one input is rejected

- **WHEN** an authenticated user submits both `url` and `text` in the same request
- **THEN** the system responds with a client error and creates no `jobs` row

#### Scenario: Unauthenticated request is rejected

- **WHEN** an unauthenticated caller posts to `/api/v1/me/jd/resolve`
- **THEN** the system responds 401 and creates no `jobs` row

### Requirement: A recognized-ATS URL is ingested as a normal public job

The system SHALL, when `url` is recognized by a supported ATS adapter (a host-scoped
`linksource` adapter or `internal/atsboard` board coverage — not the generic fallback), resolve
and write the job through the same canonical write path used by ordinary ingest
(`pipeline.UpsertJob`), producing a public, enrichment-queued, search-indexed `jobs` row exactly
as if it had been crawled normally. This resolution SHALL NOT record a board contribution or
grant any AI-credits reward — that remains the exclusive concern of the existing
`POST /api/v1/jobs/resolve` contribution flow.

#### Scenario: A Greenhouse posting URL becomes a public job

- **WHEN** an authenticated user submits `{url}` for a Greenhouse job-detail page not yet in the
  catalog
- **THEN** the response returns the new job's `job_slug`, the job is enqueued for enrichment, and
  it is eligible for the Meilisearch index like any other ingested job

#### Scenario: A URL for a job already carried by the catalog dedups

- **WHEN** an authenticated user submits `{url}` for a posting the catalog already carries under
  its `(source, external_id)`
- **THEN** the response returns the existing job's `job_slug` and no duplicate row is created

#### Scenario: Resolving a URL never records a contribution

- **WHEN** an authenticated user submits `{url}` for a posting a recognized ATS adapter resolves
- **THEN** no row is written to `link_contributions` and no AI-credits reward is granted as a
  result of this endpoint

### Requirement: An unrecognized URL or pasted text becomes a private job

The system SHALL, when `url` resolves only through the generic scrape fallback (or fetch/parse
fails) or when `text` is submitted, create a new `jobs` row with `is_private = true`,
`created_by` set to the submitting user, and a synthetic `external_id` that is never compared
against the public `(source, external_id)` dedup key space — so no two submissions, by the same
or different users, ever collide or share a row. `source` SHALL be `weblink` for the URL case and
`pasted` for the text case. The system SHALL run `internal/jobderive.Derive` synchronously
against the submitted (or scraped) title/company/description to populate skills/facets, and SHALL
NOT enqueue the row onto `enrichment_outbox`.

#### Scenario: Pasted text creates a private job

- **WHEN** an authenticated user submits `{text: "<JD body>", title: "Backend Engineer"}`
- **THEN** a new `jobs` row is created with `is_private = true`, `created_by` set to that user,
  `source = "pasted"`, and derived skills/facets populated from the submitted text

#### Scenario: An unrecognized URL creates a private job from its scrape

- **WHEN** an authenticated user submits `{url}` for a page no ATS adapter recognizes, and the
  generic scrape successfully reads a title and description
- **THEN** a new `jobs` row is created with `is_private = true`, `source = "weblink"`, and
  `created_by` set to that user

#### Scenario: An unreadable URL is rejected

- **WHEN** an authenticated user submits `{url}` and the page cannot be fetched or no usable
  title/description can be scraped
- **THEN** the system responds 422 and creates no `jobs` row

#### Scenario: Two users pasting the same text each get their own private job

- **WHEN** two different authenticated users each submit the same `{text}`
- **THEN** two independent `jobs` rows are created, each `is_private`, each `created_by` set to
  its own submitter, with no collision on `external_id`

#### Scenario: A private job is never enrichment-queued

- **WHEN** a private `jobs` row is created via this endpoint
- **THEN** no row is added to `enrichment_outbox` for it
