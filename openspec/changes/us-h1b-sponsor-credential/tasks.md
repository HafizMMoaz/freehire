## 1. Matching-toolkit extension for US employers

- [x] 1.1 Add `"US": {"usa", "u.s.", "u.s.a.", "united states", "united states of america", "america"}` to `countryAliases` in `internal/collections/register.go`; test that a company with `hq_country` stored as `United States` is recognised as denoting `US`, alongside the existing bare-ISO-code case
- [x] 1.2 Add `inc`, `incorporated`, `corp`, `corporation`, `llc` to `legalSuffixes`; test that `ACME ROBOTICS INC` strips to `acme-robotics` and that `Co` is deliberately **not** in the map (a name ending in `Co` is left unstripped)

## 2. USCIS H-1B Employer Data Hub source

- [ ] 2.1 Implement `discoverRecentFYFiles(page []byte, n int) ([]fyFile, error)` in a new `internal/collections/ush1bsponsor.go`, scraping the archive index page's `<a href="/sites/default/files/document/data/h1b_datahubexport-{YEAR}.csv">FY {YEAR} H-1B Employer Data</a>` links into (year, URL) pairs, sorted descending, top `n` returned; test against a committed fixture of the real index page HTML, including the case where fewer than `n` years are listed (error, not a shorter list)
- [ ] 2.2 Implement `ParseUSH1BSponsorsCSV(data []byte) ([]Record, error)` over the columns `"Fiscal Year",Employer,"Initial Approval","Initial Denial","Continuing Approval","Continuing Denial",NAICS,"Tax ID",State,City,ZIP` via the existing `csvColumns` helper; skip rows with a blank `Employer`; skip rows where `InitialApproval + ContinuingApproval < 1`; emit `Record{Name: employer, Meta: {"tin4": taxID}}`; test against a committed fixture sample of a real year file, covering: a normal approved row kept, a blank-employer row dropped, a denial-only row dropped, and a zero-row parse returning an error
- [ ] 2.3 Implement `FetchUSH1BSponsors(ctx context.Context, client *http.Client) ([]Record, error)`: fetch the archive index page, call `discoverRecentFYFiles(..., 5)`, fetch and parse each of the 5 year files, concatenate into one `[]Record`; test with a fake HTTP transport covering: all 5 years succeed → combined records from all of them; any single year's fetch or parse fails → the whole call errors and returns no records; fewer than 5 years discovered → the whole call errors

## 3. Registry entry

- [ ] 3.1 Add the `us-h1b-sponsor` entry to `internal/collections/collections.go`: `Kind: KindCredential`, `Dataset: &Dataset{Records: FetchUSH1BSponsors, IdentityKey: "tin4"}`, `Gate: RequireCountry("US")`, with title "H-1B sponsor history" and a description carrying the same "not a commitment to sponsor any particular role" disclaimer pattern as the UK/NL entries
- [ ] 3.2 Rename and extend `TestRegistry_HasBothSponsorCredentials` in `register_test.go` (it no longer covers just two) to iterate all three credential slugs, asserting `Kind == KindCredential`, non-empty `Title`/`Description`, `Dataset.Valid()`, and `Gate != nil` for each

## 4. Frontend badge copy

- [ ] 4.1 Add a `COPY['us-h1b-sponsor']` entry to `web/src/lib/credentials.ts` with `issuer: 'USCIS'` and a tooltip stating the credential belongs to the employer and is not a commitment to sponsor the viewed role, mirroring the UK/NL entries' phrasing; confirm `credentials.test.ts` (which already asserts every credential collection has a COPY entry) passes with the new slug
- [ ] 4.2 Run `make gen-contracts`, commit the regenerated `web/src/lib/generated/contracts.ts`, and confirm no other frontend file needs a manual edit — the credential filter group, the `/collections/[slug]` route, and the job-card/company-page badge renderer are all already generic over the registry

## 5. Verify and ship

- [ ] 5.1 `go build ./... && go vet ./... && go vet -tags=integration ./... && go test ./...` green; web build/lint/check at their existing baseline
- [ ] 5.2 Before any write, measure the real resolver/parser/gate against the live USCIS files and the public catalogue API (`/api/v1/companies?countries=US`, paginated by `offset`), following the UK/NL precedent: record matched / gated-out / ambiguous counts, and check the top ~40 grants by eye for a false positive, before this is considered safe to run against production
- [ ] 5.3 Run the real import (`cmd/import-collections`, needs `DATABASE_URL`), then `make reindex` (never stacked with `reindex-companies`), and confirm the `us-h1b-sponsor` facet returns jobs — if this session has no production database access, leave this unchecked and say so explicitly rather than marking it done
