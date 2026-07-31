## 1. One tailored copy per vacancy

- [x] 1.1 Add an owner-scoped read for the caller's existing tailored CV for a vacancy (newest first); `make sqlc`
- [x] 1.2 Return it from `cv.Store.Tailor` instead of creating a second copy, leaving a different vacancy to create its own
- [x] 1.3 Test at the store level: two bootstraps for one vacancy reach one CV, a second vacancy gets its own

## 2. One conversation per tailored CV

- [x] 2.1 Reuse the conversation already bound to the CV; mint one only when there is none, or when the bound id no longer resolves
- [x] 2.2 Integration test over real Postgres: two bootstraps, one CV, one session, one debit, and a message written after the first bootstrap survives the second

## 3. The address names the CV

- [x] 3.1 Replace the workspace's address with `?cv=<id>` as soon as the bootstrap answers (replaceState, no scroll, keep focus)
- [x] 3.2 Verified by the resume path's existing tests; the markup change itself is not unit-tested (no Svelte component renderer in the web suite)

## 4. Verification

- [x] 4.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`, integration suites, `svelte-check`, web tests
- [x] 4.2 Confirm on production: open the workspace, reload, and see the same conversation — and that `/my/cvs` stops gaining a row per refresh

**Verified on production 2026-07-31, from the data rather than by clicking.** The fix
landed at 2026-07-29 20:32 UTC (#1258); the newest duplicate pair predates it at 20:05
UTC. Ten tailored CVs have been created since, across ten distinct (user, vacancy) pairs —
zero duplicates — so the absence is a result and not merely an absence of traffic. Before
the fix, eleven pairs carried duplicates, one of them eight copies deep. No CV has more
than one assistant session, which is the conversation half of the same claim.
