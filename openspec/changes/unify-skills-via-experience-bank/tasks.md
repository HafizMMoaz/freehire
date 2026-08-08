## 1. Backend: profile skills merge primitive

- [ ] 1.1 Add `userprofile.Service.MergeSkills(ctx context.Context, userID int64, skills []string) error`: read the current profile, fold `skills` into `Skills` (case-insensitive de-dup, same rule as `web/src/lib/profileSkills.ts` `withSkills`), skip anything in `ExcludedSkills`, cap the result at the existing max-skills limit, then `Upsert` the full profile back. When no profile exists for the user, return nil without creating one.
- [ ] 1.2 Unit tests for `MergeSkills`: no-op when no profile exists; new skills get added; a skill already present (any case) is not duplicated; a skill in `excluded_skills` is skipped; the merge never exceeds the cap.

## 2. Backend: bank-to-profile sync wiring

- [ ] 2.1 In `internal/experience`, define a narrow `ProfileSkills` interface with one method, `MergeSkills(ctx context.Context, userID int64, skills []string) error`, and add it as an optional field on `Store` (nil-safe — existing `NewStore(repo)` callers and tests keep compiling).
- [ ] 2.2 Add a way to set the dependency (either a `NewStore` parameter or a setter), and wire `internal/userprofile.Service` as the implementation where `Store` is constructed (`cmd/server`, alongside the existing `userprofile.Service` construction).
- [ ] 2.3 In `Store.AddAtom`, after a successful persist, call the sync with the atom's canonical `Skills` when the dependency is set. On sync error, log and still return the persisted atom — the atom write must not fail because of it.
- [ ] 2.4 Apply the same call in `Store.UpdateAtom` after a successful persist.
- [ ] 2.5 Unit tests: `AddAtom`/`UpdateAtom` invoke the injected `ProfileSkills.MergeSkills` with the atom's skills on success; a `MergeSkills` error does not surface as an error from `AddAtom`/`UpdateAtom` and does not prevent the atom from being returned; a `Store` built without the dependency (nil) behaves exactly as it does today.
- [ ] 2.6 Integration test (`internal/db`, `-tags=integration`): banking an atom for a user with a saved profile results in the profile's `skills` including the atom's skills; banking an atom for a user with no profile leaves no profile row behind.

## 3. Frontend: remove the redundant tailor control

- [ ] 3.1 Delete `web/src/lib/tailor/skillDiff.ts` and `web/src/lib/tailor/skillDiff.test.ts`.
- [ ] 3.2 Remove the "Add N skills to profile" state, compute/confirm/apply logic, and button from `web/src/routes/tailor/[slug]/+page.svelte` (the `skillsToAddToProfile` import and its call sites).
- [ ] 3.3 Visually verify the tailor page still renders and functions correctly with the control gone (no leftover empty space, no dangling error state).

## 4. Verification

- [ ] 4.1 `go build ./...` and `go vet ./...`
- [ ] 4.2 `go vet -tags=integration ./...`
- [ ] 4.3 `go test ./...`
- [ ] 4.4 `go test -tags=integration ./internal/db/` (requires Docker/testcontainers) and any other integration suites touching `internal/experience` or `internal/userprofile`
- [ ] 4.5 Web build/lint per project conventions; confirm no remaining reference to the deleted `skillDiff.ts`
