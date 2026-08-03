## Why

Tailoring a CV today only works if the job already exists as a catalog `jobs` row reachable by
slug — there is no way to tailor against a posting that lives on another site, or against a JD
the user only has as pasted text. `/my/cvs` needs an entry point that accepts a job from three
sources (our own catalog, an external URL, or plain text) and turns all three into a real
`jobs.id` so the existing tailor/fit stack — which is hard-keyed to one — can run unmodified.

## What Changes

- Add a button/form on `/my/cvs` with three input tabs: pick an existing freehire vacancy, paste
  an external URL, or paste plain JD text. All three redirect to the existing `/tailor/[slug]`
  workspace once resolved.
- Add `POST /api/v1/me/jd/resolve`, accepting `{job_slug}` / `{url}` / `{text, title?, company?}`
  and returning `{job_slug}`.
  - `job_slug`: used as-is, no new work.
  - `url` recognized by a supported ATS (host-scoped `linksource` adapter or board coverage):
    resolved through the existing single-link import path and written as a normal **public**
    job (enrichment queue, Meilisearch indexing — identical to any other ingested job).
  - `url` that only a generic scrape can read, or whose fetch/parse fails outright (422), and
    plain `text`: written as a new **private** `jobs` row.
- Add `jobs.is_private boolean not null default false` (new migration). Private rows get a
  synthetic `external_id` (never deduped against the public `(source, external_id)` space),
  `created_by` set to the submitting user, and synchronous `internal/jobderive.Derive` for
  skills/facets — but are never enqueued onto `enrichment_outbox` (no LLM enrichment spend on a
  one-off private submission).
- Exclude `is_private` rows from the Meilisearch index and from public job listing/search.
- Job-lookup-by-slug paths used by fit-analysis and CV tailoring must treat a private job not
  owned by the caller as not found (same as an unknown slug).

## Capabilities

### New Capabilities
- `jd-tailor-intake`: the `/api/v1/me/jd/resolve` endpoint, the URL-vs-text resolution branching,
  the `is_private` job concept (synthetic external_id, creator-only visibility, no enrichment
  queue), and the `/my/cvs` entry-point UI.

### Modified Capabilities
- `job-search`: the Meilisearch index must exclude `is_private` jobs, and the separate DB-backed
  `GET /api/v1/jobs` list must also exclude them, so a private job never appears in any public
  listing or search surface.
- `job-public-identity`: `GET /api/v1/jobs/:slug` — the direct public job-detail read, reachable
  by anyone without auth — must treat a private job not owned by the caller as an unknown slug
  (404). This is the actual disclosure surface (Meilisearch exclusion alone does not stop a
  guessed/known slug from being read directly).
- `job-fit-analysis`: a private job not owned by the calling user resolves as an unknown slug
  (404), not merely as an inaccessible-but-existing job.
- `cv-tailoring`: tailoring bootstrap against a private job not owned by the calling user is
  rejected the same way as tailoring against an unknown vacancy.

Explicitly out of scope for this change: the in-app assistant's job-lookup tools
(`assistant_tools.go`, `assistant_interview_tools.go`, `assistant.go`) are not gated. A user would
have to already possess another user's private slug to feed it to the assistant — the same
prerequisite as guessing it for the direct URL — so the exposure is real but deliberately deferred
rather than expanding this change's blast radius.

## Impact

- **Schema**: new migration adding `jobs.is_private`.
- **Backend**: new handler + route for `/api/v1/me/jd/resolve`; reuse of
  `internal/linkimport`/`internal/linksource` for the recognized-ATS branch; reuse of
  `internal/jobderive.Derive` for the private branch; an access-gate check added to
  `jobs.go`'s `GetJob` (public detail), `match_analysis.go`'s three handlers, and
  `cv_tailor.go`'s `TailorCV`; an `is_private` exclusion added to both the Meilisearch-indexing
  query in `internal/search` and the DB-backed `GET /api/v1/jobs` list query.
- **Frontend**: new form/tabs on `web/src/routes/my/cvs/+page.svelte`, reusing the existing job
  search combobox and the existing `/tailor/[slug]` workspace unchanged.
- **No changes** to `internal/cvedit`, `internal/matchanalysis`'s prompt chain, or the
  `/tailor/[slug]` workspace UI itself.
