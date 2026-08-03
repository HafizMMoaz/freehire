## Context

Full background and the rationale behind each call is written up already in
`docs/superpowers/specs/2026-08-03-jd-tailor-intake-design.md` (approved). This document
restates the decisions in OpenSpec's shape and adds the concrete touch points found during
research.

Today, everything downstream of "start tailoring" is keyed on a real `jobs.id`:
- `internal/handler/match_analysis.go` (`GetMatchAnalysis`/`PostMatchAnalysis`/
  `StreamMatchAnalysis`, the `job-fit-analysis` capability) each independently look up
  `GetJobBySlug`, then read/run the three-stage LLM chain and cache `matchanalysis.Analysis` for
  `(userID, job.ID)`.
- `internal/handler/cv_tailor.go` (`TailorCV`) looks up `GetJobBySlug`, then requires that cached
  analysis to already exist (409 otherwise), then bootstraps the tailored CV
  (`cvs.job_id = job.ID`).
- `internal/handler/cv_job_match.go` (`GetCVJobMatch`) is a separate, deterministic (no LLM)
  CV-vs-job text score. It is keyed by the CV's own id and its already-bound `job_id`, not by a
  fresh slug lookup, so it needs no separate `is_private` gate — an owner-scoped CV can only be
  bound to a job the same owner already reached legitimately.
- None of the above has any path that accepts raw text instead of a job.

Reusable building blocks:
- `internal/jobderive.Derive` (`jobderive.go:90`) is a pure, DB-free function — title, company,
  description, etc. in, skills/facets/category/seniority out (via `skilltag.Parse` and
  `classify.Parse`). It already runs against in-memory structs with no `jobs` row backing them
  (`cmd/backfill-derive` calls it the same way).
- `internal/linkimport` (behind `POST /api/v1/jobs/resolve`, `internal/handler/intake.go`) already
  resolves one arbitrary URL: host-scoped adapters first, `internal/atsboard` board coverage
  second, a generic JSON-LD `JobPosting` scraper (`GenericSource`) last. When a recognized
  adapter resolves it, the job is written through the canonical `pipeline.UpsertJob` path
  (enrichment enqueued, indexed normally). `linkimport` already distinguishes the generic
  fallback from a real adapter match (`GenericSource` type assertion) — that same signal is what
  this change branches on.
- `jobs` has no visibility/indexing flag today (`migrations/0001_init.sql`). `created_by` exists
  and is already used for moderator/manual jobs, but every existing job — including
  `source='manual'` and `source='weblink'` ones — is public, enriched, and indexed
  (`internal/db/jobs_moderation_integration_test.go`).

## Goals / Non-Goals

**Goals:**
- Let a signed-in user reach the existing `/tailor/[slug]` workspace starting from a URL or from
  pasted text, with zero changes to the workspace itself.
- Reuse the existing URL-resolution stack for anything a recognized ATS adapter can parse — no
  parallel importer.
- Keep a pasted/unrecognized-scrape JD fully private: invisible in search, in listings, and to
  every user except the one who submitted it.
- Keep the private path cheap: no LLM enrichment queue for a submission that exists for exactly
  one tailoring session.

**Non-Goals:**
- Deduplicating private submissions against each other or against the public catalog. Every
  paste/unrecognized-URL submission is its own row.
- A UI to browse/manage a user's own past private submissions beyond what the tailored-CV list
  already surfaces.
- Any change to `matchanalysis`'s three-stage prompt chain, `internal/cvedit`, or the tailor
  workspace's own UI/behavior.
- Rewarding a URL submission through this endpoint with `link_contributions` credits — that stays
  exclusively the `/api/v1/jobs/resolve` contribution flow's concern. This endpoint calls the
  underlying resolver/importer directly, not the contribution-reward wrapper around it.

## Decisions

**1. Reuse `linkimport`'s resolver, not its whole endpoint.**
`/api/v1/jobs/resolve` bundles resolution with contribution recording (`link_contributions`,
AI-credit rewards) — semantics that don't belong to "I'm tailoring a CV against this link." The
new endpoint calls the resolution step directly (host-scoped adapters → board coverage →
generic), and branches on whether the winning adapter was `GenericSource`:
- Not generic → hand the parsed job to the same `pipeline.UpsertJob` call the contribution flow
  uses. Public, enriched, indexed. No new code path for the write itself.
- Generic (or the fetch/parse fails) → private row (decision 3).

**2. The "recognized vs. generic" boundary decides public/private, not "has a URL".**
A recognized ATS adapter parses a trusted, structured page — exactly as trustworthy as anything
already in the catalog, so it becomes a normal catalog job. A generic JSON-LD scrape of an
arbitrary site is unverified content, the same trust tier as hand-typed text — neither belongs in
the public catalog or the search index.

**3. Private rows: new `jobs.is_private` column, synthetic `external_id`, no enrichment queue.**
- `is_private boolean not null default false`, new migration.
- `source` = `'weblink'` (URL) or a new `'pasted'` (plain text) — reusing `'weblink'` for the URL
  case keeps it consistent with how the generic resolver already tags non-adapter-matched
  imports elsewhere.
- `external_id` is a fresh synthetic value (e.g. `uuid`) per submission — deliberately outside the
  public dedup key space, so concurrent/repeat submissions from the same or different users never
  collide over `(source, external_id)` uniqueness and never contend over `created_by`-based
  access.
- `created_by` = submitting user. Private-row access is gated on this everywhere a job is looked
  up by slug for tailoring/fit purposes.
- `jobderive.Derive` runs synchronously at creation (it's pure and DB-free — no reason to queue
  it). The row is **not** enqueued onto `enrichment_outbox`: that queue exists to extract
  additional LLM-derived structure for a catalog that gets crawled repeatedly and searched by
  many users; a private, single-tailoring-session row doesn't recoup that cost. `jobderive`'s
  dict-based facets plus the raw description text are what `matchanalysis` needs.

**4. Visibility gating happens in three places — not just the search index.**
Research first assumed all public listings were Meilisearch-backed, which turned out to be
wrong: `GET /api/v1/jobs` is a **separate DB-backed** endpoint (`internal/search`'s Meili index
is not involved), and `GET /api/v1/jobs/:slug` (`jobs.go`'s `GetJob`) is a fully public,
unauthenticated single-job read backed directly by `GetJobBySlug` — a known or guessed slug
reaches it with no index involved at all. Three places need the `is_private` check:
- Meilisearch's job-indexing query (`internal/search`) gains a `WHERE NOT is_private` (or
  equivalent) so private rows are never pushed to the index — covers search and any
  index-derived listing.
- The DB-backed `GET /api/v1/jobs` list query gains the same exclusion — covers the
  Postgres-backed listing that bypasses Meilisearch entirely.
- `GetJob` (`jobs.go`), the three fit-analysis lookups (`match_analysis.go`), and the tailor
  lookup (`cv_tailor.go`) each gain a check: if the resolved job `is_private` and
  `created_by != callerID` (or the caller is anonymous), treat it as not found (404), identical
  to an unknown slug. `GetJob` already carries an optional caller identity via `mw.optional`
  middleware, so this is a cheap comparison, not a new auth requirement; the other four are
  already behind `requireUserID`.

**Explicitly deferred**: the in-app assistant's job-lookup tools also call `GetJobBySlug`
directly and would leak a private job's text into a chat if fed a known/guessed slug — the same
prerequisite as reaching the direct URL. Left unpatched for this change: gating every
`GetJobBySlug` call site (13 across the handler package, including mail-linking and follow-up
flows that require a pre-existing user-owned application record to reach at all) would expand
this change well past its scope. The three points above are the endpoints someone could exploit
with only a slug and nothing else.

**5. `/my/cvs` UI: one form, three tabs, one destination.**
The "our own vacancy" tab needs no backend work — it's a search/select combobox over the existing
job search, redirecting straight to `/tailor/[slug]`. The URL and text tabs both submit to the
new endpoint and get a `job_slug` back, then redirect to the same page. The tailor workspace does
not know or care which tab produced its slug.

## Risks / Trade-offs

- **[Risk] A generic scrape captures a broken/garbage page (paywall, JS-rendered shell, wrong
  JobPosting block) and produces a low-quality private job.** → Mitigation: this is no worse than
  what the generic resolver already does for the public contribution flow today; the difference
  is scope (private, one user) not quality. If the scrape yields no usable title/description at
  all, the endpoint responds 422 rather than creating an empty row.
- **[Risk] Skipping `enrichment_outbox` for private rows means `matchanalysis` runs against
  dict-derived facets only, with no LLM-enriched structure (e.g. parsed requirements list).** →
  Mitigation: a freshly-crawled public job is in the identical position immediately after ingest
  (enrichment is async and can lag); `matchanalysis` already has to tolerate an unenriched job, so
  this isn't a new failure mode, only a permanent instance of an existing one.
- **[Risk] Forgetting the `is_private` filter in some future job-listing query silently leaks a
  private JD.** → Mitigation: centralize the filter in the single query `internal/search` uses
  to select jobs for indexing, so leakage would require a *second* Postgres-backed public listing
  path to be added later without reusing that helper — call this out in that query's code comment.

## Migration Plan

- Add-only migration: `ALTER TABLE jobs ADD COLUMN is_private boolean NOT NULL DEFAULT false`.
  Default `false` means every existing row is unaffected; no backfill needed.
- No rollback complexity: the column is additive and unindexed by anything until this change's
  queries reference it.

## Open Questions

None outstanding — the three prior clarifying rounds (provider-recognized → public ingest;
pasted/generic-scrape → private row with a visibility flag; single new form on `/my/cvs`) resolved
the open branching points before this document was written.
