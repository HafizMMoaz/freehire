## MODIFIED Requirements

### Requirement: DB-backed jobs list is index-served with an approximate total

The DB-backed `GET /api/v1/jobs` list endpoint SHALL return open, non-private jobs
(`closed_at IS NULL AND NOT is_private`) ordered newest-added first (`created_at` descending, `id`
descending) with `limit`/`offset` pagination, using the standard list envelope
`{"data": [...], "meta": {...}}`. The ordered page SHALL be served through a partial index
matching that order (no full-table sort at request time), so the endpoint stays responsive at
catalogue scale (millions of open jobs).

The `meta.total` for this endpoint SHALL be an **approximate** estimate of the open, non-private
job count, not an exact `count(*)` over the whole open set — mirroring how `/jobs/search` already
reports an *estimated* total. The endpoint SHALL NOT run a query whose cost grows linearly with
the catalogue size on each request.

#### Scenario: List returns a page ordered newest-added first

- **WHEN** a client requests `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** up to 20 open jobs are returned ordered by `created_at` descending
  (ties broken by `id` descending), in the `{"data": [...], "meta": {...}}`
  envelope

#### Scenario: Meta carries an approximate total and the applied pagination

- **WHEN** a client requests `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** `meta` reports the applied `limit` and `offset` and a `total` that is
  an approximate open-job count (not required to equal an exact `count(*)`)

#### Scenario: A private job never appears in the DB-backed list

- **WHEN** a `jobs` row has `is_private = true` and `closed_at IS NULL`
- **THEN** it is excluded from `GET /api/v1/jobs`'s results and from `meta.total`

### Requirement: Batch reindex keeps the index in sync

The system SHALL provide a batch command that reads jobs from Postgres and
writes their documents to the Meilisearch `jobs` index in batches, suitable for
scheduled execution. The command SHALL ensure the index and its settings
(attributes, ranking rules, embedder) exist before indexing. Reindexing SHALL be
idempotent: running it again with unchanged data SHALL leave the index
representing the same set of jobs.

The index SHALL contain documents only for **open, non-private** jobs: the reindex command
SHALL index open non-private jobs and SHALL remove the documents of jobs that have been
closed (`closed_at` set) or marked private (`is_private` set) since the previous run. A reopened
job SHALL be indexed again on the next run; a job SHALL NOT transition from private to public
outside of this change's scope, but the exclusion is re-evaluated on every run regardless.

#### Scenario: Reindex populates the index

- **WHEN** the reindex command runs against a database containing jobs
- **THEN** the `jobs` index exists with the configured settings and contains one
  document per open, non-private job

#### Scenario: Reindex is idempotent

- **WHEN** the reindex command runs twice with no change to the underlying jobs
- **THEN** the index represents the same set of job documents after the second
  run as after the first

#### Scenario: Closed job is dropped on reindex

- **WHEN** a job is closed and a reindex runs
- **THEN** the job's document is removed from the index and no longer matches any
  search

#### Scenario: Reopened job returns to the index

- **WHEN** a previously closed job is reopened and a reindex runs
- **THEN** the job's document is indexed again

#### Scenario: A private job is never in the index

- **WHEN** a `jobs` row has `is_private = true` and a reindex runs
- **THEN** no document for that row exists in the `jobs` index afterward
