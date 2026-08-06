# Search conventions

Meilisearch-backed keyword and hybrid search over jobs and companies. The package doc in
`client.go` explains the index topology; this file covers what the code can't tell you.

## Always true

- **A job whose category is unresolved by both the title dictionary and the LLM never
  enters the index.** `search.CategoryUnresolved` (`document.go`) reports true when
  `jobs.category` is empty (`internal/classify` found nothing in the title) AND the raw
  LLM enrichment's own `category` is also empty or the catch-all `"other"` — read from
  the raw JSON, not `jobview`'s folded `Enrichment.Category`, which the dictionary
  column always overwrites (`internal/classify/AGENTS.md`) and so never carries the
  LLM's answer. Both `cmd/reindex`'s `splitJobs` and `cmd/search-drain`'s `IndexBatch`
  apply it — added because this bucket was measured at ~65% of the open catalogue
  (broad multi-industry ATS crawls contribute postings like "Industrial Painter" or
  "Backhoe Loader Operator" that neither dictionary was ever meant to place), diluting
  every keyword and category-filtered search with undifferentiated noise. A job later
  categorized by a dictionary update or a fresh LLM pass is picked up by the next full
  `cmd/reindex` run, not incrementally — `SetJobEnrichment` does not enqueue
  `search_outbox`, so there is no faster path today.
- **Meilisearch has ONE serial task queue.** Two rebuilds do not run concurrently — the
  second queues behind the first and looks like a hang while the engine is genuinely busy.
  Before triggering any rebuild, check `ps aux | grep reindex` and
  `GET /tasks?statuses=processing`. Never stack `reindex-companies` with `make reindex`, and
  never stack anything with a `--semantic` pass.
- **Killing the reindex client does not cancel enqueued Meili tasks.** To actually stop:
  `POST /tasks/cancel?indexUids=<uid>&statuses=enqueued,processing`. That cancelation itself
  queues, and it is irreversible — don't fire it on an unconfirmed diagnosis.
- **A failed rebuild leaves an orphan `*_rebuild` index.** `Rebuild.Promote` drops the old
  data only *after* a successful swap, so an aborted run leaves a full-size index on disk
  (~55 GB at catalogue scale). That has filled the production disk and put rebuilds into a
  death spiral: ENOSPC → orphan → less disk → ENOSPC. Reclaim with
  `DELETE /indexes/<uid>_rebuild`. `Rebuild.Prepare` also drops a leftover before starting.
- **Live reads are never affected by a rebuild** — the swap is atomic.
- A full rebuild (`scope=full`) pushes **every** open, non-private, categorized document
  unconditionally to the fresh rebuild index — `content_hash` is never read in
  `cmd/reindex`; the `indexed=X skipped=Y` log line counts rows `ResilientPage` skipped
  for corruption, not hash-skipped ones. (An older version of this doc claimed
  content_hash-incremental full rebuilds; that behavior does not exist in the code —
  verified 2026-08-05.) `content_hash`-gating only exists one layer up, at the
  ingest→`search_outbox` enqueue decision (`cmd/ingest`'s `needsIndex`) — the reason a
  full reindex is the correct, if slow, way to surface any change a write path forgot
  to enqueue, including `is_tech` (deliberately excluded from `jobhash.Of()`, so an
  is_tech-only flip needs a full reindex or some other change to the same row to reach
  the index).
- `SubmitJobs` submits **without awaiting** the Meili task (`internal/linkimport`'s single
  on-demand doc push — `cmd/resolve-url` and the browser extension's "add this page", both
  human-triggered and low-volume, so one unawaited push per action is fine); `IndexJobs`
  awaits (the reindex path AND `cmd/search-drain`'s wave push — a wrong/silently-dropped
  push there would leave the outbox entry deleted with nothing actually indexed). Pick
  deliberately: **never call `SubmitJobs` from a high-frequency caller** — Meilisearch
  re-merges its inverted index/facet structures across the WHOLE live index on every push
  regardless of batch size (observed 50-90s per push at catalogue scale), so many small
  unawaited pushes queue up and saturate host disk IO. That is exactly what happened when
  `cmd/ingest` called it once per crawl across ~169 independent per-board processes; the
  fix routes that traffic through `search_outbox` + `cmd/search-drain` instead (see
  `internal/searchdrain`), collapsing many small pushes into few, fat, awaited ones.
- `swapIndexes` calls `POST /swap-indexes` over raw HTTP, not the SDK: the pinned
  meilisearch-go always serializes a `rename` field that engine v1.13 rejects.
- Indexed descriptions are capped at `maxIndexedDescriptionRunes`; `maxTotalHits` is the
  count-honesty cap, **not** the pagination guard (that's `maxSearchWindow` in the handler).

## Adding a filterable attribute

Adding one creates a hard-500 window: the app emits the new filter the moment the image
rolls out, but the attribute only becomes filterable when the rebuild swaps in — ~26 min at
catalogue scale. Meili answers `invalid_search_filter` (400), and the handler maps any Meili
error to 500, so the whole filtered page breaks rather than degrading.

Either run the reindex **before** rolling out the app image, or push the new index settings
to the **live** index first (settings updates are cheap; documents lag, so results are stale
or empty — never a 500).

## Limitations

- A Meili filter error 500s the page instead of degrading. That's the robustness seam.
- `jobs_semantic` is built by a separate, much slower pass and is only queried when
  `SemanticRatio > 0`. Always scope a semantic rebuild (`--posted-within 30d`); a bare full
  embed of the whole catalogue takes hours and monopolizes the queue.
