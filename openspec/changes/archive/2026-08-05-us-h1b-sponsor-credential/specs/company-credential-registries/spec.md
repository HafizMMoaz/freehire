## ADDED Requirements

### Requirement: The US register grants the H-1B sponsor credential from historical approval data

The system SHALL source the `us-h1b-sponsor` credential from the USCIS H-1B
Employer Data Hub's bulk CSV files, published one file per completed fiscal year
at a stable archive index page. Because the set of published fiscal years grows
over time, the system SHALL discover the current set of year files at fetch time
rather than pinning literal year numbers, and SHALL combine the **5 most recent**
fiscal years found into one member list. A discovery that finds fewer than 5
year files SHALL be an error, not a smaller result — a shrunk index is
indistinguishable from a genuine drop in coverage and MUST NOT silently reduce the
window.

Fetching each of the 5 year files SHALL be all-or-nothing: if any one year's file
is unreachable or fails to parse, the whole fetch SHALL fail and no membership
SHALL be written for this credential on that run, leaving the previous run's
membership intact.

Within the combined rows, the system SHALL grant the credential only to an
employer with at least one row whose approvals (initial or continuing) sum to 1 or
more across the 5-year window. An employer whose every row in the window sums to
zero approvals SHALL NOT receive the credential, even if it has denied petitions
on record. This filtering SHALL happen while building the member list, before
company matching — unlike the UK register's route gate, which reads a matched
row's field after matching, because the approval threshold is a fact about the
source row rather than about which company (if any) it matches.

The register does not publish a route or scheme breakdown — every listed petition
is H-1B — so no route gate applies, unlike the UK register.

#### Scenario: The most recent 5 fiscal years are combined into one member list

- **WHEN** the archive index page lists 15 fiscal-year files
- **THEN** only the 5 most recent are fetched and combined; older years are not
  fetched

#### Scenario: An index page listing fewer than 5 years aborts the run

- **WHEN** the archive index page lists fewer than 5 fiscal-year files
- **THEN** the resolve fails with an error and no credential membership is written

#### Scenario: A single unreachable year aborts the whole fetch

- **WHEN** 4 of the 5 target fiscal-year files fetch successfully and 1 fails
- **THEN** the resolve fails with an error, and the previous run's membership for
  this credential is left unchanged

#### Scenario: An employer with only approvals in the window earns the credential

- **WHEN** an employer's rows across the 5-year window sum to at least one initial
  or continuing approval
- **THEN** the employer's row is retained as a candidate for the `us-h1b-sponsor`
  credential

#### Scenario: An employer with only denials in the window does not earn the credential

- **WHEN** an employer's rows across the 5-year window sum to zero approvals,
  regardless of any denials on record
- **THEN** the employer's rows are dropped before company matching and it cannot
  receive the credential from this run

### Requirement: US employer names disambiguate by tax identifier and match by US geography

The system SHALL retain each parsed row's last-4-digit tax identifier (`Tax ID`) as
record metadata and SHALL use it as the ambiguity guard's identity field, the same
role the UK register's town and the Dutch register's KvK number play: two rows
sharing a normalized employer name but different tax identifiers SHALL be treated
as a naming collision and SHALL grant the credential to no company; rows sharing
both a name and a tax identifier — including an employer's rows for different
worksite cities or different fiscal years within the window — SHALL be treated as
one organisation.

The system SHALL apply the existing conservative country-matching guards
(legal-suffix stripping, geography gate, single-token rule, country comparison)
with the United States recognised as a matchable country: the country-comparison
guard SHALL recognise `US` by ISO code or by a spelled-out US name, and the
legal-suffix guard SHALL recognise US corporate forms (`Inc`, `Incorporated`,
`Corp`, `Corporation`, `LLC`) as strippable trailing suffixes, in addition to the
UK/NL forms it already recognises.

#### Scenario: Same-named employers with different tax identifiers are a collision

- **WHEN** two organisations in the register share a normalized name but carry
  different `Tax ID` values
- **THEN** no company receives the credential from that name

#### Scenario: One employer's multi-city, multi-year rows are not mistaken for a collision

- **WHEN** the register lists one employer's rows across several worksite cities
  and several fiscal years within the window, all sharing the same `Tax ID`
- **THEN** every one of those rows survives the ambiguity guard

#### Scenario: A US corporate suffix is stripped before matching

- **WHEN** the register lists `ACME ROBOTICS INC` and the catalogue holds the
  company `acme-robotics`
- **THEN** the names match after suffix stripping and normalization

#### Scenario: A spelled-out US headquarters country is recognised

- **WHEN** a single-token register name matches a company whose headquarters
  country is stored as `United States` rather than `US`
- **THEN** the company receives the credential, the comparison having recognised
  the name as denoting the same country
