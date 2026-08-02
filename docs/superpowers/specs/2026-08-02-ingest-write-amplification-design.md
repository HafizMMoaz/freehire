# Ingest write amplification — design

**Date:** 2026-08-02
**Status:** approved, ready for planning

## Problem

Every crawl rewrites every row it re-sees, whether or not anything changed.

`UpsertJob` (`internal/db/queries/jobs.sql:194`) ends in `ON CONFLICT (source, external_id)
DO UPDATE SET ...` with no `WHERE`. A re-ingest of an untouched posting therefore writes a
full new tuple: the ~2.5 KB `description`, every `text[]`, `content_hash`, plus
`last_seen_at = now()` and `updated_at = now()`. In Postgres that is a new row version — a
re-TOAST of the description, an entry in each of the table's 21 indexes when the update
cannot go HOT, WAL for all of it, and a dead tuple for autovacuum to reclaim later.

The write path already computes the signal that says none of this was needed. `UpsertJob`
returns `changed` (the stored `content_hash` differs from the incoming one) and
`cmd/ingest/store.go:66` uses it to skip the Meilisearch push. The saving is taken in the
search index and nowhere else: by the time the flag is read, Postgres has already rewritten
the row.

The same statement carries a second, worse offender. Its `company_upsert` CTE
(`internal/db/queries/jobs.sql:215`) does `ON CONFLICT (slug) DO UPDATE SET name, updated_at
= now()` unconditionally, once per job. A board with 5,000 postings updates its company row
5,000 times in a single crawl.

### Measured on prod, 2026-08-02

Database `hire` = 102 GB, of which `jobs` = 92 GB (heap 19 GB, indexes 9.8 GB, TOAST 63 GB).

Live TOAST content, from `TABLESAMPLE SYSTEM (0.3)` over 16,995 rows extrapolated to 5.6M:

| column | avg on-disk | populated | live total |
|---|---|---|---|
| `description` | 2,504 B | 100% | ~14 GB |
| `semantic_embedding` | 3,092 B | 32% | ~5.5 GB |
| `enrichment` | 159 B | 100% | ~0.9 GB |
| | | | **~20 GB in a 63 GB TOAST relation** |

So roughly **43 GB — 42% of the whole database — is bloat**, not data.

`pg_stat_user_tables`, since the stats began at the host-2 restore (~11 days):

| | `jobs` | `companies` |
|---|---|---|
| live tuples | 5,608,085 | 305,094 |
| `n_tup_upd` | 330,948,733 | 286,000,570 |
| updates per row | **59** | **937** |
| `n_tup_ins` | 791,136 | 31,096 |
| HOT share | 62.3% | 53.9% |
| dead tuples | 19.8% | 32.0% |
| heap per live row | 3.4 KB | **26 KB** |

125M of the `jobs` updates went non-HOT; each maintained all 21 indexes.

Amplifiers, both fixable here:

- `jobs` has an empty `reloptions`: `fillfactor` is 100 (no room on a page for a HOT
  update) and `autovacuum_vacuum_scale_factor` is the default 0.2, so autovacuum wakes only
  past 1.1M dead tuples. `n_dead_tup` is 1.39M — the table sits permanently at the
  threshold. (`companies` was already given 0.02 and is still 32% dead, because 937
  updates per row outruns any threshold.)

### What this is not

The originating hypothesis was that Postgres is the host's I/O bottleneck. It is not, and
that was already measured on 2026-07-31: per-process read attribution on host-2 gave
`meilisearch` 330 MB/s against `postgres` 22.6 MB/s. Postgres is ~7% of the disk band; the
saturation is a 45 GB memory-mapped Meili index on a 30 GB-RAM host. This change is aimed
at the *storage* half of the problem, and reaches the I/O half only indirectly — 43 GB
returned to the filesystem is 43 GB of page cache no longer held by dead tuples, on a host
where Meili is starved for exactly that.

## Goal

Stop rewriting rows that did not change. Specifically: an ingest pass over a board whose
postings are all unchanged must write only `jobs.last_seen_at`, must not touch any index,
and must not touch `companies` at all.

Non-goals — deliberately deferred:

- Reclaiming the 43 GB already accreted (`pg_repack`), dropping the two ~1 GB indexes with
  near-zero scans, and raising `autovacuum_max_workers` / `vacuum_cost_limit`. A repack
  before the write path is fixed just re-bloats.
- Purging `semantic_embedding` (5.5 GB for a feature disabled on prod).
- Anything about Meilisearch's index size.

## Design

### 1. `RefreshUnchangedJob` — the cheap write

A new query in `internal/db/queries/jobs.sql`:

```sql
-- name: RefreshUnchangedJob :one
UPDATE jobs
SET last_seen_at = now()
WHERE source = sqlc.arg(source)
  AND external_id = sqlc.arg(external_id)
  AND content_hash = sqlc.arg(content_hash)
  AND closed_at IS NULL
RETURNING sqlc.embed(jobs);
```

Three constraints, each load-bearing:

- **`last_seen_at` is the only column written.** It appears in none of the 21 indexes, so
  the update is HOT-eligible and maintains no index. Every other column `UpsertJob` writes
  is provably equal already (see below) or is bookkeeping we specifically want to stop
  writing.
- **`updated_at` is not stamped.** The column comes to mean "content last changed" rather
  than "last crawled". Two readers see it today: the jobs sitemap serves it as `<lastmod>`
  (`internal/handler/sitemap.go`), where a timestamp that stops claiming every posting
  changed on every crawl is the honest signal, and `jobview` puts it on the public wire. It
  is also the precondition for an incremental reindex — `ListJobsUpdatedAfter` exists but is
  dormant (no caller, and `cmd/reindex` has no `--since` flag, whatever the query's comment
  says), because a column stamped on every crawl selects everything.
- **`closed_at IS NULL` is required for correctness.** Without it, a closed posting that
  reappears on the board with identical content would have its liveness refreshed and stay
  closed. Falling through to `UpsertJob` is what reopens it.

**Why the stale-derivation question does not arise.** `RoleFingerprint`
(`internal/jobhash/rolefingerprint.go:23`) reads `company_slug`, `title` and `description`;
all three are inputs to `jobhash.Of` (`internal/jobhash/jobhash.go:23`). Equal
`content_hash` therefore implies equal `role_fingerprint`, so skipping its refresh cannot
leave it stale. The same subset argument covers every deterministic facet the upsert
rewrites.

**The known hash/description divergence is unaffected.** When a detail fetch fails, the
upsert preserves the stored `description` but stores a hash computed over the empty one
(documented at `jobs.sql:249-256`). Such a row simply fails the `content_hash` match on the
next crawl and takes the full path, exactly as today.

### 2. `save` tries the cheap write first

`cmd/ingest/store.go` — `Save` and `SaveWithApplyForm` already funnel into one private
`save`, so this is one call site. Replace the `qtx.UpsertJob(ctx, params)` call with a
helper that tries `RefreshUnchangedJob` and falls back to `UpsertJob` on `pgx.ErrNoRows`,
returning a `db.UpsertJobRow` either way — with `Inserted` and `Changed` false on the cheap
branch.

The rest of `save` then flows unmodified, and correctly: `clustersByRole` and `needsIndex`
are both already gated on `Inserted`/`Changed` and are false for an unchanged row, so the
role-cluster lookups and the index push are skipped by the existing logic rather than by a
new branch. The enrichment enqueue, the apply-form write and the crawled-set record all
still run — the enqueue deliberately so, keeping today's behaviour that a never-enriched
job is re-offered to the queue on every crawl (it is already gated to no-op for an enriched
row and idempotent on `UNIQUE (job_id, target_version)`).

Cost on the hot path is one statement, the same as today. The changed path pays two; it is
the rare case.

### 3. `TouchJob` stops stamping `updated_at`

`TouchJob` (`jobs.sql:855`) is the same idea already built for hydrating sources, and it
has the same defect. A reopen *is* a change the reindex must see; a plain liveness refresh
is not:

```sql
updated_at = CASE WHEN closed_at IS NOT NULL THEN now() ELSE updated_at END
```

### 4. Guard the company upsert

In `UpsertJob`'s `company_upsert` CTE only (`jobs.sql:215`):

```sql
ON CONFLICT (slug) DO UPDATE SET
    name       = EXCLUDED.name,
    updated_at = now()
WHERE companies.name IS DISTINCT FROM EXCLUDED.name
```

The two other `company_upsert` CTEs, in `UpsertManualJob` and `UpdateManualJob`, fire once
per moderator action and are left alone.

Side effect, and an improvement: `companies.updated_at` is read as sitemap `<lastmod>`
(`internal/db/queries/companies.sql:92`). Today every hiring company reports "just now" on
every crawl, which is the pattern search engines learn to ignore. It becomes truthful.

### 5. Storage parameters

```sql
ALTER TABLE jobs SET (fillfactor = 90, autovacuum_vacuum_scale_factor = 0.02);
```

`fillfactor` applies only to pages written from here on — existing pages stay packed at
100% until the deferred repack, so the HOT share improves gradually rather than at once.
This is a catalog-only change with no table rewrite, but it takes a brief ACCESS EXCLUSIVE
lock, which will queue behind a long-running read and block every reader behind it. It goes
out as its own migration with a `lock_timeout`, not batched with others.

## Testing

Integration tests (`//go:build integration`, `internal/db`, testcontainers):

- Re-ingesting an identical posting advances `last_seen_at`, leaves `updated_at` untouched,
  and reports `inserted=false, changed=false`.
- A changed posting takes the full path and reports `changed=true`.
- A **closed** posting re-ingested with identical content is reopened (`closed_at IS NULL`,
  strikes reset) — the regression guard for the `closed_at IS NULL` predicate.
- Re-ingesting an identical posting leaves `companies.updated_at` untouched; a renamed
  company still updates it.
- `TouchJob` on an open row leaves `updated_at`; on a closed row it reopens and stamps it.

Unit test in `internal/jobhash`: assert every `RoleFingerprint` input is also an `Of` input,
so a future field added to the fingerprint but not the hash fails here instead of silently
going stale on the cheap path.

Per `CLAUDE.md`, `go vet -tags=integration ./...` before pushing — `go test ./...` compiles
none of the above.

## The risk this design rests on

The saving is proportional to the share of re-crawls whose `content_hash` matches, and that
share is **not currently measured anywhere** — nothing records the `changed` outcome. It is
not safe to assume it is high.

`jobhash.Of` covers 19 fields, and a provider only has to vary one of them per crawl for
its rows to keep taking the full path forever: a `url` carrying a session token or a
tracking parameter, a `posted_at` the board re-serializes on each render, a `location`
string whose whitespace is not stable. Such a provider saves nothing here, and no counter
would say so — the failure mode is silence, the same shape as `ingested=0 failed=0` on a
dead board.

So the rollout logs the cheap-path hit rate per provider (one aggregate line per ingest run,
not per row) and the first check after release is that distribution, not the global write
counters. A provider at ~0% is a finding in its own right: it means a hashed field is
churning, which is worth fixing at the adapter regardless of this change, because it has
also been forcing a pointless Meili re-push on every crawl.

## Rollout and verification

The claim that this cuts writes by a large factor is a prediction, not a result, and is
verified on prod rather than asserted:

1. Snapshot `pg_stat_user_tables` for `jobs` and `companies` before the release.
2. Release; run the storage-parameter migration separately.
3. Re-snapshot after 24 h. Expected: `n_tup_upd` on `companies` down by orders of
   magnitude, `n_tup_upd` on `jobs` down to roughly the rate of genuine content change,
   HOT share up, `n_dead_tup` falling.

If the counters do not move, the diagnosis was wrong and part B (repack) should not be run
on its strength.
