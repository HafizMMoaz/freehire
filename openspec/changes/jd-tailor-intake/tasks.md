## 1. Schema

- [ ] 1.1 Add a new migration: `jobs.is_private boolean not null default false` (never edit an
      applied migration file).
- [ ] 1.2 Regenerate sqlc (`make sqlc`) so `db.Job` and relevant query params expose `IsPrivate`.

## 2. Private job creation

- [ ] 2.1 Add a synthetic `external_id` generator (fresh UUID per submission) for the private
      path — never compared against the public `(source, external_id)` dedup space.
- [ ] 2.2 Implement a private-job writer: given title/company/description (+ URL when present),
      run `internal/jobderive.Derive` synchronously, insert a `jobs` row with
      `is_private = true`, `created_by = userID`, `source` = `"pasted"` (text) or `"weblink"`
      (URL), and do **not** enqueue it onto `enrichment_outbox`.
- [ ] 2.3 Unit tests for the writer: facets populated from `Derive`, no `enrichment_outbox` row
      created, two calls (same or different user, same input) never collide on `external_id`.

## 3. URL/text resolution branching

- [ ] 3.1 Add a resolver that classifies a submitted `url`: recognized ATS (host-scoped
      `internal/linksource` adapter or `internal/atsboard` board coverage) vs. generic scrape /
      unreadable — reusing `linkimport`'s resolution step directly, without its contribution-
      reward recording.
- [ ] 3.2 Wire the recognized-ATS branch to the existing `pipeline.UpsertJob` write path (public,
      enrichment-queued, indexed — identical to normal ingest).
- [ ] 3.3 Wire the generic/unrecognized-URL branch and the plain-`text` branch to the private-job
      writer from 2.2; an unreadable/unparseable URL yields no row.
- [ ] 3.4 Integration tests: known `job_slug` passthrough; recognized-ATS URL → public job;
      generic/unrecognized URL → private job; unreadable URL → no row; plain text → private job;
      the same text submitted by two different users → two independent private rows.

## 4. Resolve endpoint

- [ ] 4.1 Define request/response types and validation for `POST /api/v1/me/jd/resolve`: exactly
      one of `job_slug` / `url` / `text` required, `text` non-empty when present.
- [ ] 4.2 Implement the handler wiring the 3.x resolver behind `RequireAuth`, returning
      `{job_slug}`.
- [ ] 4.3 Handler tests: 200 + slug for each of the three input kinds; 400 for zero or multiple
      inputs; 401 unauthenticated; 422 for an unreadable URL.

## 5. Visibility gating

- [ ] 5.1 Exclude `is_private` rows from `internal/search`'s Meilisearch-indexing query.
- [ ] 5.2 Exclude `is_private` rows (and their count) from the DB-backed `GET /api/v1/jobs` list
      query.
- [ ] 5.3 Gate `jobs.go`'s `GetJob`: a private job not owned by the (optionally authenticated)
      caller responds 404, identical to an unknown slug.
- [ ] 5.4 Gate `match_analysis.go`'s three handlers (`GetMatchAnalysis`, `PostMatchAnalysis`,
      `StreamMatchAnalysis`) with the same private-not-owned → 404 rule.
- [ ] 5.5 Gate `cv_tailor.go`'s `TailorCV` with the same rule; the bootstrap is rejected the same
      way as for an unknown vacancy.
- [ ] 5.6 Tests: a private job is absent from a Meilisearch reindex and from `GET /api/v1/jobs`;
      its creator can read/analyze/tailor it normally; a different user and an anonymous caller
      get 404 from all three gated handlers.

## 6. Frontend — `/my/cvs` entry point

- [ ] 6.1 Add the API client method for `POST /api/v1/me/jd/resolve`
      (`web/src/lib/api.ts`).
- [ ] 6.2 Add a "Подобрать вакансию для резюме" button + form on
      `web/src/routes/my/cvs/+page.svelte` with three tabs: existing-job search/select, URL
      input, and text input (with optional title/company fields).
- [ ] 6.3 Wire the "our vacancy" tab to redirect straight to `/tailor/[slug]` (no backend call).
- [ ] 6.4 Wire the URL and text tabs to the new endpoint, handling loading/400/422 states, and
      redirect to `/tailor/[job_slug]` on success.
