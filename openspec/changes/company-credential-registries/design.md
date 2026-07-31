## Context

`internal/collections` is the code-owned registry of curated company themes. Its pipeline is already the shape this change needs: `Dataset{URL, Data, Parse}` yields company names, `Match` resolves them onto catalogue companies via `normalize.Slug`, `Reconcile` writes only the tags the registry manages (leaving foreign tags alone), the result lands in `companies.collections`, and the import worker denormalizes it onto `jobs.collections`. Critically, `collections` is **already** a filterable Meilisearch attribute on both the jobs and companies indexes (`internal/search/client.go`), so a new tag needs no migration, no new attribute, and none of the ~26-minute hard-500 deploy window that a new filterable attribute costs.

Two facts about the surrounding system shaped the decisions below.

**Company signals have been fragmenting.** `remote_regions`, `yc_batch`/`yc_status`, `yc_stage`/`yc_flags`, `maturity`, and `subindustry` arrived as five separate migrations with five separate facets. YC in particular now flows through **two independent paths**: `cmd/import-yc` writes the `yc_*` columns matching by current-name slug *or any `former_names` slug*, while `cmd/import-collections` tags the `yc` collection matching by `normalize.Slug` — the same yc-oss dataset fetched twice under two different matching rules, landing in two different data models. This change does not fix that, but it deliberately does not add a sixth path.

**Sponsorship already has a job-level signal.** `enrichment.visa_sponsorship` is an LLM reading of the posting text, surfaced as a facet and consumed by `internal/hardconstraint` (`appendWorkAuth` raises a blocker when a job explicitly does *not* sponsor and the CV's country is outside the job's). The register signal is a different claim about a different subject, and conflating them would corrupt a signal that already drives user-visible advice.

## Goals / Non-Goals

**Goals:**

- Grant a company-level "licensed visa sponsor" credential from the UK and NL public registers, with no migration and no new filterable attribute.
- Generalize `internal/collections` just far enough that the *next* register is one registry entry — a kind, record-shaped datasets, an optional gate, and a resolvable URL.
- Keep the register signal legible: never merged with `enrichment.visa_sponsorship`, always carrying what it does and does not mean.
- Make the frontend registry mirror generated rather than hand-kept, now that a field on it (`kind`) drives correctness rather than copy.
- Bias hard toward false negatives. An unbadged sponsor is a missed opportunity; a badged non-sponsor sends someone into a visa conversation that cannot happen.

**Non-Goals:**

- Migrating `yc_*`, `maturity`, or `subindustry` into the tag model. The `yc_*` columns carry *values* (`Winter 2012`), not flags, and rewriting `import-yc`, `recount-companies`, the `/companies` facets, the FilterModal, the company page, and the OG cards would be churn without value. The seam is noted; the work is not in scope.
- Unifying the two YC ingestion paths. Same reasoning; noted, not fixed.
- Storing raw register rows in Postgres. They are worker input, and the outcome is fully expressed by the tag. If auditing a specific company's badge ever becomes a real need, that is the point to add the table — not before.
- Any change to `enrichment.visa_sponsorship`, `hardconstraint`, or the ingest pipeline.
- Registers beyond UK and NL. Two is enough to prove the abstraction; a third would be speculative generality.

## Decisions

### Extend `internal/collections` rather than build `internal/companyregistry` beside it

A sibling package would mean two near-identical pipelines — two `Match`, two `Reconcile`, two workers, a second `text[]` column on both `companies` and `jobs`, a second filterable attribute, and a second deploy window. That is precisely the fragmentation described above, one more instance of it.

The cost of extending instead is a widened package contract (`Parse` returns records; the entry gains `Kind` and `Gate`) touching eight existing parsers. Those adapt trivially, and `Gate == nil` preserves current behaviour exactly, so no existing collection changes semantics.

*Alternative considered:* a private `visa_sponsor_countries text[]` column. Shortest path to the feature and the most precise type, but it is the sixth migration in the series and leaves the next register no better off.

### `Kind` discriminates editorial from credential, and it is part of the generated contract

"Big Tech" is an editorial judgement; "licensed under the Skilled Worker route" is a legal fact with an issuing authority and a snapshot date. Presenting them in one undifferentiated pill list would flatten that distinction at exactly the moment a user is deciding whether to rely on it.

The kind therefore has to reach the frontend, and `web/src/lib/collections.ts` is currently a hand-kept mirror whose own comment asks to be folded into `cmd/gen-contracts` "if the set grows". It is growing, and the field being added is one where drift is a correctness bug: a missing or wrong `kind` puts a tag in the wrong group, or drops its pill entirely while the tag still filters. Generating it removes the failure mode rather than documenting it. `FILTER_COLLECTIONS` stays frontend-only — it has no Go counterpart to drift from.

### Records, not names, and a resolver for the URL

The UK register needs `Route` to gate on, so a `[]string` parser cannot express it. `Record{Name string; Meta map[string]string}` keeps the metadata untyped on purpose: each register's columns are its own, a typed union across registers would grow a field per source, and only the entry's own gate reads its own keys.

The GOV.UK CSV lives at a URL carrying the snapshot date (`…_Web_Register_-_2026-06-30.csv`) and is republished monthly, so a constant `URL` breaks within weeks. `ResolveURL` runs at fetch time: GOV.UK Content API → `details.attachments` → select by title, falling back to scraping the publication page. Two mechanisms because they fail independently — the API can change shape while the page still renders, and vice versa.

### Matching: exact match, then four guards

`normalize.Slug("ACME ROBOTICS LIMITED")` is `acme-robotics-limited`, which never matches our `acme-robotics`, so a legal-suffix strip has to run first — whole word, end of string only, or `Limited Brands` loses its first word.

Exact-match alone then fails in the other direction. The UK register is ~120k organisations across every industry: cafés, schools, scaffolders. `Spark`, `Nova`, `Apex` are all in there. Three guards close that:

| Guard | Rule | What it stops |
|---|---|---|
| Geography | register's country ∈ `companies.countries` | a US-only company inheriting a UK licence |
| Single-token | 1-token names additionally require `hq_country` = register's country | Apple, which *does* hire in London, inheriting some unrelated `APPLE LTD` |
| Ambiguity | a normalized name appearing ≥2× in the register grants to nobody | generic names that cannot identify one organisation |
| Route (UK) | ≥1 row on Skilled Worker / Global Business Mobility / Scale-up | a farm licensed only for Seasonal Worker badging its one IT role |

The single-token rule is the load-bearing one and also the riskiest: it leans entirely on `hq_country`, and single-word brands (Monzo, Revolut, Wise) are exactly the UK companies worth surfacing. If `hq_country` turns out sparse in production, this rule silently discards them. Hence the dry run below, before any write.

*Alternative considered:* trigram similarity with a review queue for near-matches, per `companyname.Accept`. Higher coverage, but it introduces a standing human workload and a decisions table for a facet that is supposed to be dict-only. Rejected for now; if coverage proves too thin, this is the escalation.

### `-dry-run` before the first production write

Every threshold above is a guess until measured against the real catalogue. The worker gains a mode that resolves, matches, and gates exactly as a real run would, reports matched / gated-out / unmatched per collection, and writes nothing. This is how the `hq_country` question gets answered before it can do damage, not after.

### The register is never stored; the tag is the whole outcome

~132k rows across the two registers, refetched per run, held only in memory during it. No table, no snapshot history, no audit trail. The consequence is that "why does this company have this badge?" is answerable only by re-running with `-dry-run` — an acceptable trade for not carrying a table nothing reads.

## Risks / Trade-offs

**`hq_country` is sparse → the single-token rule discards the best UK brands.** → The mandatory pre-write `-dry-run` measures this directly. If the coverage is poor, the fallback is to widen single-token acceptance to `countries` plus a corroborating signal (a `.co.uk` domain in `companies.domains`, or a UK city on the company's jobs) rather than to drop the guard.

**GOV.UK changes the Content API payload or the page markup → the parse breaks.** → Three aborts cover the three shapes this failure takes. A failed fetch or resolve aborts (pre-existing). A zero-row parse aborts (added here). And a run in which a tag would lose most of its current holders aborts (added after review): an upstream *relabelling* — a renamed route value, a recased column — leaves the row count fully intact, parses cleanly, matches nobody, and would otherwise reconcile the credential off every company with a zero exit code. A truncated snapshot is the same failure at partial scale. `-force` overrides it for the rare run where the loss is genuine.

**A closing UK office silently revokes the badge.** → `companies.countries` is job-derived; when a company's last UK job closes, `GB` leaves the facet and the next import drops the credential even though the licence is intact. This is the correct behaviour (we do not advertise sponsorship where nobody is hiring) but it is surprising, so it is documented rather than fixed.

**Two filter specs on one query parameter may not be supported.** → `web/src/lib/facets.ts` declares `{ param: 'collections', control: 'pills' }`, and it is unverified whether the filter machinery tolerates two specs writing the same param. Verify during implementation; if it does not, render one spec whose options carry a group heading. This affects presentation only — the query contract is one `collections` param either way.

**Legal-suffix stripping over-matches.** → Two genuinely different UK entities (`Foo Ltd`, `Foo PLC`) normalize alike. The ambiguity guard catches this only when they are *different organisations* — it keys on the register's identity field (town / KvK), because the UK register lists one organisation once per route it holds and a bare repeated-name rule would delete exactly the companies holding several licences. Same name, same town collapses to one body and survives; same name, different towns grants to nobody. Spelling variants of one organisation therefore pass through, which is the intended outcome.

**The register is a point-in-time snapshot.** → A licence revoked the day after publication still reads as valid until the next import. The badge names its issuing register so a user can verify independently, and the disclaimer copy does not overclaim.

**Package-contract churn.** → Changing `Parse` touches eight parsers that are working today. Mitigated by their triviality and by `Gate == nil` preserving existing behaviour; the existing collection tests are the regression net and must stay green untouched.

## Migration Plan

No database migration. `companies.collections` and `jobs.collections` exist; `collections` is already filterable on both Meili indexes; no new filterable attribute is introduced, so there is no rebuild-before-rollout ordering constraint and no hard-500 window.

1. Ordinary release of the application image.
2. `go run ./cmd/import-collections -dry-run` against production. **Gate:** inspect `hq_country` coverage and the per-collection matched / gated-out counts. If the single-token rule is discarding recognisable brands, stop and revisit the threshold before writing.
3. `go run ./cmd/import-collections` for the real write.
4. `make reindex` — never stacked with `reindex-companies` (Meilisearch has one serial task queue; stacking them has deadlocked it before).

**Rollback:** remove the two entries from the registry and re-run the worker. `Reconcile` manages only registry-declared tags, so the credentials are reconciled off every company and nothing else is touched. No data loss, no migration to reverse.

## Open Questions

- **`hq_country` density in production.** Partly answered ahead of the dry run: sampling 1,000 companies spread across the whole GB-hiring cohort found the column populated for ~61% and **every** populated value an ISO code (mixed case). The single-token rule therefore ships as designed, and the comparison now accepts spelled-out names as well, so it no longer depends on `cmd/backfill-company-info` continuing to receive ISO codes from its upstream. Step 2 of the migration plan still measures the real per-collection grant counts.
- **Do two facet specs on one query param work?** Answered by reading `facets.ts` and the filter machinery during implementation. Presentation-only either way.
- **Refresh cadence.** UK republishes monthly, NL continuously. `cmd/import-collections` runs on the existing collections schedule; whether the registers warrant their own cadence is deferred until the first month of drift is observed.
