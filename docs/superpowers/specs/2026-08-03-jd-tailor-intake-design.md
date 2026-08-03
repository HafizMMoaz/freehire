# Tailor CV from an arbitrary JD (paste text / paste URL / pick own vacancy)

## Problem

Today the only way to tailor a CV is to start from an existing `jobs` row on
freehire (`/tailor/[slug]`, entered from a vacancy's match page). There is no
way to tailor against a job posted elsewhere on the web, or against raw JD
text the user has in hand. `TailorCV` and `GetCVJobMatch`
(`internal/handler/cv_tailor.go`, `internal/handler/cv_job_match.go`) are both
hard-keyed to a real `jobs.id` looked up by slug — no code path runs
match-analysis or tailoring off ad hoc text.

## Goal

Add an entry point on `/my/cvs` that accepts three input formats and routes
all three to the existing `/tailor/[slug]` workspace:

1. **Our own vacancy** — pick from the catalog (search/select).
2. **External URL** — a job posting link on another site.
3. **Plain text** — a pasted JD with no URL.

All three formats must produce a real `jobs.id`, since the entire downstream
tailor/match-analysis stack depends on one. The only new machinery is how
formats 2 and 3 become a `jobs` row.

## A. UI entry point

A button on `/my/cvs` ("Подобрать вакансию для резюме") opens a form with
three tabs:

1. **Наша вакансия** — a search/select combobox over the existing job
   catalog. On pick, redirect straight to `/tailor/[slug]` for that job. No
   backend change — this tab is pure frontend wiring reusing the existing job
   search.
2. **Ссылка** — a URL text input.
3. **Текст** — a required JD textarea, plus optional "должность"/"компания"
   fields (they improve `classify`/`skilltag` derivation but are not
   required).

Tabs 2 and 3 submit to a new resolve endpoint; on success the frontend
redirects to `/tailor/[job_slug]` for the returned slug — identical
destination to tab 1.

## B. Backend resolution

New endpoint, e.g. `POST /api/v1/me/jd/resolve`, accepting one of:
`{job_slug}` / `{url}` / `{text, title?, company?}`. Returns `{job_slug}`.

Resolution branches:

| Input | Resolution | Outcome |
|---|---|---|
| `job_slug` | none — already a real job | used as-is |
| `url`, **recognized ATS** (host-scoped `linksource` adapter or board coverage via `internal/atsboard`) | reuse the existing single-link resolution machinery already used by the board-contribution flow (`internal/linkimport`/`internal/linksource`) → normal `pipeline.UpsertJob` | **public** `jobs` row: normal enrichment queue, normal Meilisearch indexing, becomes part of the catalog |
| `url`, **unrecognized** (falls through to the generic JSON-LD scrape / `GenericSource`, or the fetch itself fails) | take whatever the generic scrape yielded; if fetch/parse fails entirely, return 422 | **private** `jobs` row (see below) |
| `text` | none — used directly | **private** `jobs` row (see below) |

The boundary is "recognized ATS provider" vs. "generic scrape or raw text",
not "has a URL" vs. "doesn't". A recognized-provider page is exactly as
trustworthy as anything else already in the catalog and gets ingested
normally. A generic scrape of an arbitrary site is unverified content, same
trust tier as pasted text — neither should enter the public catalog or
Meilisearch.

## C. Schema change

New column: `jobs.is_private boolean not null default false` (new migration
file — never edit an applied one).

For rows created via the private path:

- `source` = `'weblink'` (URL scrape) or a new value `'pasted'` (plain text).
- `external_id` — a synthetic value (e.g. a UUID), deliberately **not**
  deduped against the public catalog's `(source, external_id)` key space.
  Every submission gets its own row; two different users (or two attempts by
  the same user) never collide and never contend over access.
- `created_by` = submitting user's id (column already exists, used today for
  manual/moderator-created jobs).

## D. Access control and catalog exclusion

- Public listing queries (`/jobs`, `/companies`, search) gain a
  `WHERE NOT is_private` (or equivalent) filter.
- Meilisearch reindex query in `internal/search` excludes `is_private` rows.
- Slug-keyed lookups used by tailor/match/job-detail handlers
  (`cv_tailor.go`, `cv_job_match.go`, the public job page) must check: if
  `is_private`, only serve it when `created_by` matches the requesting user;
  otherwise 404 (not 403, to not reveal existence).

## E. Derive/enrichment scope

Private rows do **not** get enqueued onto `enrichment_outbox` — no LLM
enrichment. The pure, DB-free `jobderive.Derive` (`internal/jobderive`) runs
synchronously at creation time to populate skills/facets from the
title/company/description fields provided, which is what match-analysis
needs. This avoids spending LLM budget and a background queue on
single-use, ephemeral private submissions.

## Out of scope (explicitly not building)

- Any UI to list/manage a user's own past pasted JDs beyond what's already
  reachable through their tailored-CV list.
- Deduping a generic-scrape/pasted-text private row against another user's
  identical submission — every submission is independent by design (see C).
- Any change to the existing `/tailor/[slug]` workspace itself — it's reused
  unmodified for all three formats.
