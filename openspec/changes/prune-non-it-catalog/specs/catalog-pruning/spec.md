## ADDED Requirements

### Requirement: Catalogue-fit rule decides which jobs are removed

The system SHALL decide a job's catalogue fit from three independent rules, evaluated live against the current non-tech dictionary rather than from the stored `is_tech` column, so an iteration needs no prior re-derivation pass. A job MUST be removed when the non-tech title detector flags its title, when its category is one of the non-technical categories AND its company has never shown any technical evidence, or when its `is_tech` is unknown AND its company has never shown any technical evidence nor any tagged skill. A job from a source that is never re-crawled — Telegram extraction, user submissions, link-source imports — MUST NOT be removed by any rule, because a dictionary mistake there cannot be undone by a later crawl.

A company's technical evidence MUST be evaluated over its entire history including closed jobs, because "this company never posts anything technical" needs the maximum available evidence, and MUST be computed once per run before any deletion so the classification cannot shift underneath the run.

#### Scenario: Blue-collar title is removed everywhere

- **WHEN** a job's title is flagged by the non-tech title detector
- **THEN** the job is removed regardless of its company's technical evidence

#### Scenario: Business role at a non-technical company is removed

- **WHEN** a job's category is a non-technical category and its company has never posted a job with technical evidence
- **THEN** the job is removed

#### Scenario: Business role at a technical company is kept

- **WHEN** a job's category is a non-technical category but its company has posted jobs with technical evidence
- **THEN** the job is kept

#### Scenario: Unknown job at a company with no evidence at all is removed

- **WHEN** a job's `is_tech` is unknown and its company has never shown technical evidence nor any tagged skill across its whole history
- **THEN** the job is removed

#### Scenario: Unknown job at a company with some evidence is kept

- **WHEN** a job's `is_tech` is unknown and its company has posted at least one job with technical evidence or tagged skills
- **THEN** the job is kept

#### Scenario: A non-crawled source is never removed

- **WHEN** a job originates from Telegram extraction, a user submission, or a link-source import and would otherwise match a removal rule
- **THEN** the job is kept

### Requirement: Company-scoped removal requires retiring the board

The system SHALL apply the company-scoped rules — non-technical category at a company without technical evidence, and unknown at a company with no evidence at all — only to companies whose board entries are removed from the source board files in the same step. Boards are re-crawled hourly on the unchanged dedup key, so a company-scoped deletion that leaves the board in place is undone within one crawl cycle; the ingest-time rejection covers only the title rule and cannot substitute for board retirement.

#### Scenario: Company-scoped deletion without board retirement is refused

- **WHEN** a run would apply a company-scoped rule to a company whose board is still listed in the source board files
- **THEN** the run reports those companies and does not delete their jobs

#### Scenario: Retirement candidates are reported

- **WHEN** the operator requests the board-retirement report
- **THEN** the system lists the board-file entries whose company has never shown technical evidence, identified by the same slug normalization ingest uses

### Requirement: Deletion is batched, capped, and mirrored to the search index

The system SHALL delete matching jobs in bounded batches and MUST extend every batch to the duplicate cluster of each row it deletes, because `jobs.duplicate_of` is a restricting foreign key and a canonical row with a live duplicate cannot be deleted alone. Each batch MUST remove the same rows from the Meilisearch facet index by primary key, so the index does not serve documents whose rows no longer exist. A run MUST honour an operator-supplied cap on the number of rows it deletes.

#### Scenario: Duplicate cluster is deleted with its canonical

- **WHEN** a batch selects a canonical job that other rows reference through `duplicate_of`
- **THEN** the referencing rows are deleted in the same batch and the delete does not fail on the foreign key

#### Scenario: Search index is cleaned in the same step

- **WHEN** a batch of jobs is deleted from the database
- **THEN** the same identifiers are removed from the facet search index

#### Scenario: Run stops at the cap

- **WHEN** a run is given a cap lower than the number of matching rows
- **THEN** the run deletes at most that many rows and reports how many matched

### Requirement: Deletion is dry-run by default and gated on inspection

The system SHALL default to reporting what it would delete and MUST require an explicit flag to delete anything. A dry run MUST print a random sample of the pending batch's titles and MUST break the batch down by rule and by source, so a batch dominated by a single board — the signature of a too-broad term rather than a real cluster — is visible before any row is removed.

#### Scenario: Default run deletes nothing

- **WHEN** the worker runs without the explicit apply flag
- **THEN** it reports the matching rows and deletes none

#### Scenario: Dry run surfaces a sample and a breakdown

- **WHEN** a dry run completes
- **THEN** it prints a random sample of the pending titles and counts grouped by rule and by source

### Requirement: Deletions are archived for audit

The system SHALL record every deleted job in an archive holding its identity, title, company, and the rule that removed it, and MUST NOT copy the description or the enrichment payload — the storage those columns occupy is the reason for the deletion. The archive is the only way to answer, after an irreversible removal, whether something was removed that should have been kept.

#### Scenario: A deleted job leaves an archive row

- **WHEN** a job is deleted by the worker
- **THEN** an archive row records its identifier, source, external id, title, company slug, the rule that matched, and the time

#### Scenario: Archive stays small

- **WHEN** archive rows are written
- **THEN** they carry no description and no enrichment payload

### Requirement: Residual unclassified titles are reportable

The system SHALL provide a read-only report of the most frequent word pairs occurring in the titles of open, non-duplicate jobs that still carry no `is_tech` signal, each with the number of distinct jobs containing it and the sources those jobs came from. Clustering MUST be by word pair rather than by whole title: boards append location, schedule and requisition detail to titles, so half the unclassified mass is titles that occur exactly once, and whole-title grouping splits one role across many singleton clusters. A word pair is also the unit the non-tech dictionary accepts, so a reported cluster is directly usable as an anchored term.

Tokenization MUST be Unicode-aware, so accented Latin and non-Latin scripts survive it — a large share of the catalogue is Portuguese, Spanish and Russian. The report MUST exclude pairs containing a stop word, a token shorter than three characters, or a purely numeric token, since employment type, schedule notation and posting boilerplate otherwise dominate the ranking. Each job MUST be counted at most once per pair, so a pair repeated within one title does not inflate its cluster.

#### Scenario: Operator sees the largest remaining clusters

- **WHEN** the report runs with a requested limit
- **THEN** it lists that many word pairs by descending count of distinct jobs, each with its count and sources

#### Scenario: One role spelled inconsistently forms one cluster

- **WHEN** several jobs share a role phrase but differ in the location, schedule or requisition detail appended to their titles
- **THEN** they are counted in the same word-pair cluster

#### Scenario: Accented and non-Latin titles survive tokenization

- **WHEN** a title contains accented Latin or non-Latin words
- **THEN** those words are tokenized whole rather than split at the non-ASCII character

#### Scenario: Boilerplate is not reported as a cluster

- **WHEN** a title contains employment type, schedule notation or posting boilerplate
- **THEN** no word pair built from those tokens appears in the report

#### Scenario: Classified jobs are excluded

- **WHEN** a job carries a technical or non-technical signal
- **THEN** it does not appear in the residual report
