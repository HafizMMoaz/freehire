## 1. Residual-title miner

- [x] 1.1 Add the SQL query that groups open, non-duplicate jobs with `is_tech IS NULL` by normalized title, returning count and distinct sources, ordered by count descending with a limit
- [ ] 1.2 Add `cmd/mine-titles`: read-only run-once worker reading `DATABASE_URL`, `--limit` flag (default 100), printing title, count, and sources
- [ ] 1.3 Run it against prod read-only and record the top clusters in the change notes, confirming the residual set matches the design's expectation

## 2. Ingest-time rejection

- [ ] 2.1 Add the catalogue-fit predicate as a package-private helper in `internal/pipeline`, taking the constructed `job.Job` and reporting whether the posting is rejected by the non-tech title rule
- [ ] 2.2 Add `Rejected` to the pipeline run stats, kept distinct from `Skipped`
- [ ] 2.3 Wire the predicate between `normalizeJob` and `Store.Save` in the batch path (`internal/pipeline/pipeline.go:336`)
- [ ] 2.4 Wire the same predicate in the stream path (`internal/pipeline/pipeline.go:476`)
- [ ] 2.5 Log one line per board with a non-zero rejected count, including the rejected share of that board's postings
- [ ] 2.6 Assert in tests that `cmd/tg-extract` and the other non-crawled write paths remain unfiltered

## 3. Deletion archive

- [ ] 3.1 Add the migration creating `pruned_jobs(id, source, external_id, title, company_slug, rule, pruned_at)` with no description or enrichment column
- [ ] 3.2 Regenerate sqlc and add the batch archive-insert query
- [ ] 3.3 Note in the change that the migration must be applied to prod by hand before the worker first runs

## 4. Prune rule

- [ ] 4.1 Add the company-evidence query: per `(source, company_slug)` over the entire history including closed jobs, whether any job ever had technical evidence and whether any ever had tagged skills
- [ ] 4.2 Implement the pure rule predicate — title rule, non-tech category at a company without technical evidence, unknown at a company with no evidence at all — table-driven tested across bucket × `is_tech` × category
- [ ] 4.3 Implement the non-crawled source exclusion (Telegram, submissions, link-source imports) with tests
- [ ] 4.4 Implement the guard that refuses company-scoped rules for a company whose board is still listed in `sources/*.yml`, reporting those companies instead of deleting

## 5. Prune worker

- [ ] 5.1 Add `cmd/prune` skeleton: keyset scan over candidate rows, `--dry-run` default, `--apply`, `--limit`, `--sample` flags
- [ ] 5.2 Compute company evidence once at start, before any deletion
- [ ] 5.3 Implement the batched delete extending each batch to its duplicate cluster (`id = ANY(batch) OR duplicate_of = ANY(batch)`)
- [ ] 5.4 Mirror each batch to the facet index via `search.Client.DeleteJobs`
- [ ] 5.5 Write archive rows for every deleted job with the rule that matched
- [ ] 5.6 Dry-run output: random sample of pending titles plus counts broken down by rule and by source
- [ ] 5.7 Honour `--limit` as a hard cap and report how many rows matched versus were deleted

## 6. Board-retirement report

- [ ] 6.1 Add `cmd/prune --boards`: read the `sources/*.yml` entries, slugify each `company` with the same normalization ingest uses, and list the entries whose company has no technical evidence
- [ ] 6.2 Test the slug matching against a board file fixture, including an entry whose company name differs in case and punctuation

## 7. First dictionary iteration

- [ ] 7.1 Add anchored terms for the behavior-technician cluster to `classify.nonTechTitleTerms`, each with a positive test and a negative test naming a real technical title
- [ ] 7.2 Add anchored terms for the maintenance/service technician and car-rental-driver clusters, same test discipline
- [ ] 7.3 Add anchored terms for the medical-speciality cluster, same test discipline
- [ ] 7.4 Verify no added term matches any title in a fixture of real technical titles sampled from prod

## 8. Documentation

- [ ] 8.1 Update `docs/agents/job-lifecycle.md` and `CLAUDE.md`: closing remains soft, with the catalogue-pruning hard delete as the stated exception
- [ ] 8.2 Add `internal/pipeline/AGENTS.md` notes for the rejection path and the `Rejected` counter
- [ ] 8.3 Document the iteration loop and the end-of-campaign `backfill-derive` plus `reindex` step
