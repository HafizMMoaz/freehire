## MODIFIED Requirements

### Requirement: Curated collections are a company-level membership fact

The system SHALL model a curated collection as a company-level fact: each company
MAY belong to zero or more collections, stored as a set of collection slugs on the
company. A collection slug SHALL come from a fixed, code-owned registry. Each
registry entry SHALL carry a `slug`, a human `title`, a `description`, a `kind`,
and a membership source — exactly one of a static hand list of canonical company
slugs or a remote dataset. Adding a collection SHALL be a single registry entry.
Membership SHALL NOT be derivable from a job's text or its ATS source — it is a
fact about the company, populated only from the registry's sources.

The `kind` SHALL distinguish an **editorial** collection (a curated theme, such as
Big Tech or Unicorns) from a **credential** (a verifiable fact drawn from an
authoritative public register). The kind SHALL be part of the registry contract
shared with the frontend, because it determines how a tag is presented; it SHALL
NOT be hand-mirrored in a second place where it could drift from the Go registry.

A dataset SHALL be defined by a parser yielding **records** rather than bare names.
A record SHALL carry the member's name and MAY carry source metadata (for example a
route, a locality, or a registry identifier) for later gating. A dataset SHALL
supply its payload by exactly one of a fixed URL, an embedded blob, or a resolver
that determines the URL at fetch time — the last of these for a source whose
published URL changes between snapshots.

A registry entry MAY carry a **gate**: a predicate over the candidate company and
the matched record that SHALL hold before the tag is applied. An entry with no gate
SHALL be matched on name alone, unchanged from prior behaviour.

#### Scenario: A company belongs to multiple collections

- **WHEN** a company qualifies for two collections (e.g. `yc` and `bigtech`)
- **THEN** the company's collection set contains both slugs

#### Scenario: The registry defines each collection's display copy, kind, and source

- **WHEN** the collection registry is read
- **THEN** each entry exposes a slug, title, description, kind, and exactly one
  membership source (a static slug list or a dataset)

#### Scenario: A dataset parser yields records carrying source metadata

- **WHEN** a dataset whose source publishes per-member attributes is parsed
- **THEN** each record exposes the member's name plus that source's metadata,
  available to the entry's gate

#### Scenario: A gateless entry matches on name alone

- **WHEN** a registry entry defines no gate
- **THEN** every name-matched company is tagged, with no additional condition
  applied

#### Scenario: A dataset resolves its URL at fetch time

- **WHEN** a dataset defines a resolver instead of a fixed URL
- **THEN** the URL is determined during the run and the payload is fetched from it

### Requirement: The import worker resolves and populates membership idempotently

The system SHALL provide a run-once-and-exit import worker that, for each
collection in the registry, resolves its member companies — a dataset collection is
fetched and parsed to records, a static-list collection uses its slugs — matches
them onto existing companies by **normalized name** (the same normalization as
company slugs; unmatched candidates are omitted and logged, never guessed), applies
the entry's gate where one is defined, writes `companies.collections` for the tags
it manages, and propagates the result onto `jobs.collections`. The worker SHALL be
idempotent and re-runnable (re-running with the same inputs yields the same
membership) and SHALL only modify the collection tags it manages, leaving any other
tags on a company untouched. If any collection's source cannot be resolved (e.g. a
dataset fetch fails, or a parse yields zero records) the worker SHALL abort before
writing — a partial resolve would reconcile a collection's tag off every company.
After propagation the worker SHALL signal that a search reindex is required.

Where a gate needs company attributes beyond the slug and the current tag set, the
worker SHALL load those attributes alongside the membership it reconciles.

The worker SHALL additionally abort before writing when a tag it manages would lose
at least half of the companies currently holding it. This covers the failure the
fetch and empty-parse aborts cannot see: a source whose values have been relabelled
upstream still parses to its full row count and then matches nobody, which would
otherwise reconcile the tag off every holder with no error. A tag no company
currently holds SHALL NOT trigger this — that is a new collection's first run — and
an explicit override SHALL be available for a run where the loss is genuine.

The worker SHALL support a **dry run** that performs every resolve, match, and gate
evaluation and reports what it would write — per collection, the number of matched,
gated-out, and unmatched candidates — without writing any membership. A dry run
SHALL leave the database untouched.

#### Scenario: Re-running the import is idempotent

- **WHEN** the import worker runs twice with the same inputs
- **THEN** the resulting `companies.collections` and `jobs.collections` are
  identical after each run

#### Scenario: Unmatched dataset companies are omitted and logged

- **WHEN** a dataset entry has no company whose normalized name matches
- **THEN** no company is tagged for that entry and the unmatched count is logged

#### Scenario: A failed dataset resolve aborts before writing

- **WHEN** a collection's dataset cannot be fetched or parsed
- **THEN** the worker aborts without writing any membership (no collection is
  reconciled off existing companies)

#### Scenario: An empty parse is treated as a failure

- **WHEN** a dataset is fetched successfully but parses to zero records
- **THEN** the worker treats it as a failed resolve and aborts before writing

#### Scenario: Static-list membership comes from the hand list

- **WHEN** a static-list collection (e.g. `bigtech`) is resolved
- **THEN** exactly the existing companies whose slugs are in the registry's hand
  list are tagged with that collection

#### Scenario: A gated-out company keeps its other tags

- **WHEN** a company matches a gated collection by name but fails its gate
- **THEN** the company is not tagged for that collection, and every other tag it
  holds is preserved

#### Scenario: A collapsing tag aborts the run

- **WHEN** a source parses to its usual size but, after gating, would leave a managed
  tag with fewer than half its current holders
- **THEN** the worker reports the shortfall and aborts before writing, unless the
  override is given

#### Scenario: A new collection's first run is not treated as a collapse

- **WHEN** a newly added collection matches some companies and no company currently
  holds its tag
- **THEN** the run proceeds and writes the new membership

#### Scenario: A dry run reports without writing

- **WHEN** the worker runs in dry-run mode
- **THEN** it reports the matched, gated-out, and unmatched counts per collection
  along with any tag that would collapse, and neither `companies.collections` nor
  `jobs.collections` is modified

### Requirement: Collections are a job-search facet plus a discovery hub

The system SHALL expose `collections` as a selectable facet in the main job-search
filter sidebar (`/jobs`), rendering one option per **company-collection** registry
entry, so a user can filter the job feed by collection — composably with every
other facet — and the filter is reflected in the URL (`/jobs?collections=<slug>`).
Options SHALL be grouped by the registry entry's kind, so editorial collections and
credentials are visually distinct while remaining a single filter axis over one
query parameter.

The system SHALL also expose a discovery hub at `/collections` listing **both**
kinds of collection — company collections and filter collections — as visually
uniform cards, each with its title, description, and a count of its open jobs. A
company-collection card's count SHALL come from the `collections` search-facet
distribution; a filter-collection card's count SHALL come from a job-search total
for its filter `params`. Counts are decorative: a failed count fetch SHALL degrade
to no count rather than failing the page. The hub's first render SHALL be
server-rendered.

Each collection — of both kinds — SHALL have a dedicated landing page at
`/collections/<slug>`, and every hub card SHALL link to its collection's landing
page (not to `/jobs`). The landing page SHALL be server-rendered on first paint,
SHALL be self-canonical (`<origin>/collections/<slug>`, never the bare `/jobs` its
raw filter would otherwise resolve to), SHALL emit breadcrumb structured data, and
SHALL render the collection's jobs as a scoped `/jobs` feed that pins — and hides
the controls for — the collection's own constraint.

#### Scenario: Collection is a facet on the job search

- **WHEN** a user opens `/jobs` and selects the `yc` collection in the sidebar
- **THEN** the URL carries `collections=yc` and the feed contains only open jobs
  whose `collections` include `yc`, composable with the other facets

#### Scenario: Credential options are grouped apart from editorial ones

- **WHEN** a user opens the collection facet and the registry holds both kinds
- **THEN** editorial and credential options appear under separate group headings,
  and selecting either kind writes the same `collections` query parameter

#### Scenario: The hub lists company collections linking to their landing pages

- **WHEN** a user opens `/collections`
- **THEN** the page lists `yc` and `bigtech`, each with its title, description, and
  the number of its open jobs, linking to `/collections/<slug>`

#### Scenario: The hub lists filter collections linking to their landing pages

- **WHEN** a user opens `/collections`
- **THEN** the page lists the `remote-worldwide` filter collection with its title,
  description, and open-job count, linking to `/collections/remote-worldwide`

#### Scenario: A collection landing page is a self-canonical SEO page

- **WHEN** a crawler fetches `/collections/python`
- **THEN** the server-rendered HTML carries `rel="canonical"` of
  `<origin>/collections/python`, breadcrumb JSON-LD, and the scoped job feed for
  that collection's filter

#### Scenario: A failed count fetch does not break the hub

- **WHEN** a collection's open-job count cannot be fetched
- **THEN** the hub still renders that collection's card, without a count
