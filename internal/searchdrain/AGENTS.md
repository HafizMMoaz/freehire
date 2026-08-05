# Facet-search drain conventions

## Scope
Incremental facet-search indexing via the `search_outbox` queue, mirroring `internal/embed`/`semantic_outbox`; the full `reindex` swap-rebuild remains the reconciler.

## Always true
- Work flows through `search_outbox` — a reference-only queue (`job_id` + lease/retry, not a copy of the job or a target-version/target-model column: the facet index has no staleness key beyond `content_hash`, which `cmd/ingest`'s cheap-write gate already checks before enqueuing).
- `cmd/ingest` enqueues (`EnqueueSearchOutbox`, `ON CONFLICT (job_id) DO NOTHING`) inside the SAME transaction as the job's upsert and the enrichment enqueue — atomic with the write, not best-effort after it.
- The enqueue is unconditional: it does not check `MEILI_MASTER_KEY`. `cmd/search-drain` is the sole gate on whether indexing is actually configured; an unconfigured deployment just never drains the queue.
- `ClaimSearchOutboxBatch` joins `jobs` and filters `closed_at IS NULL AND duplicate_of IS NULL` at claim time (not just enqueue time) — a job that closed or became a non-canonical repost between queueing and draining is simply never claimed. That entry then sits in the table forever (the claim predicate never matches it again); accepted as bounded, low-volume garbage, the same tolerance `enrichment_outbox` already has.
- One wave (`SEARCH_DRAIN_BATCH_SIZE`, default 500) is built and pushed as ONE `IndexJobs` call (awaited — unlike `internal/linkimport`'s `SubmitJobs`, a silently-dropped push here would leave the outbox entry deleted with nothing actually indexed). On a batch-level failure the runner falls back to per-item processing so one poison/corrupted/deleted row can't sink the wave (mirrors `internal/embed`).
- The document is built the same way the old inline ingest push did: `search.FromJob(row)` + the job-reality signal (`jobview.ClassifyReality`) + widening the canon's geography with its role cluster's (`RoleClusterCount`/`RoleClusterGeo`/`MergeClusterGeography`) — lives in `cmd/search-drain`'s `searchIndexer`, deliberately NOT shared code with `cmd/embed`'s near-identical semantic-index version (each is one small adapter file over a different index; not worth a shared abstraction across two call sites).
- This exists because Meilisearch re-merges its inverted index/facet structures across the WHOLE live index on every push, regardless of batch size (observed 50-90s per push at catalogue scale) — routing every write-path push through one drained queue collapses many small, expensive pushes (formerly one per board per crawl, across ~169 independent `cmd/ingest` processes) into few, fat, cheap ones.

## How it works
The facet index (`jobs`, plain keyword/facet, no embedder) can otherwise only be refreshed by a full `reindex` — a swap-rebuild from zero on a multi-hour schedule — or by `internal/linkimport`'s single-document `SubmitJobs` push for its own on-demand imports. `cmd/search-drain` fills the gap `cmd/embed` fills for the semantic index: work flows through `search_outbox`, claimed in waves, indexed in one batch, completed (rows deleted) in one call. The runner lives in `internal/searchdrain` behind `Store` + `Indexer` ports (unit-tested with fakes, mirroring `internal/embed/runner_test.go`); `cmd/search-drain` wires the concrete Postgres + Meilisearch adapters. Tuning via `SEARCH_DRAIN_BATCH_SIZE`/`SEARCH_DRAIN_LEASE_SECONDS`/`SEARCH_DRAIN_MAX_ATTEMPTS`/`SEARCH_DRAIN_CALL_TIMEOUT_SECONDS` (`config.LoadSearchDrain`).

## Limitations
None currently listed.
