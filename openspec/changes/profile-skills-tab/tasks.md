## 1. Profile store: bulk skill autosave

- [x] 1.1 Add `addSkills(skills: string[])` to `ProfileStore` (`web/src/lib/profile.svelte.ts`): fold `withSkill` (from `profileSkills.ts`) over every new skill and issue a single `PUT` through the existing `#writeSkills`/`#queue` machinery. **Superseded in task 4.2**: replaced by `mergeResumeExtraction`, its only call site's needs having grown to include specializations too — see task 4.2's note.

## 2. Shared skill-dictionary loader

- [x] 2.1 Extract the skills-typeahead fetch+sort (`api.facetCounts` → `FacetOption[]` sorted by descending count) out of `ProfileForm.svelte`'s `loadSkills()` into `web/src/lib/skillDictionary.ts`, exporting `loadSkillDistribution(): Promise<FacetOption[]>`.
- [x] 2.2 Update `ProfileForm.svelte` to call the shared loader instead of its inline copy.

## 3. New Skills tab

- [x] 3.1 Create `web/src/lib/components/SkillsView.svelte`: reads `profileStore.profile?.skills` / `?.excluded_skills` reactively (no local buffer), renders the Skills and Skills to avoid `RemoteSearchSelect` blocks (moved from `ProfileForm`, same copy/styling), loads its typeahead universe via `loadSkillDistribution()`.
- [x] 3.2 Wire toggle handlers to `profileStore.addSkill`/`removeSkill`/`avoidSkill`/`unavoidSkill`, with a single `pending` boolean disabling both controls while a write is in flight, and a `failed: string | null` rendering `Could not update {failed} in your profile. Try again.` on rejection (mirrors `JobMatch.svelte`). Also blocks — client-side, before the write — removing or avoiding the profile's one remaining skill, which the server would always reject.
- [x] 3.3 Add `{ id: 'skills', label: 'Skills' }` to `TABS` in `web/src/routes/my/profile/+page.svelte` (after `settings`) and render `<SkillsView />` in the tab body for that id.

## 4. Settings form: drop Skills once a profile exists

- [x] 4.1 In `ProfileForm.svelte`, gate the Skills / Skills to avoid blocks behind `!editing` (keep them for first-time profile creation only); relabel the `main` form-tab from "Skills & role" to "Role" when `editing`.
- [x] 4.2 Update `analyzeResume()`: when `!editing`, keep merging extracted skills into the local `skills` buffer (unchanged creation-flow behavior); when `editing`, persist extracted skills directly instead of touching local state. Adjust the `resumeNote` copy for the `editing` case to point at the Skills tab instead of "below". **Revised during review**: a skills-only write (the originally-planned `profileStore.addSkills(cv.skills)`) triggers `ProfileForm`'s own `{#key profile.updated_at}` remount and silently discards any specialization the same extraction resolved into local state. Fixed by committing both together via `profileStore.mergeResumeExtraction(cv.skills, nextSpecializations)` (replaces the task-1.1 `addSkills` mutator, since it had no other caller) — see `design.md`'s "CV upload against an existing profile autosaves..." decision.

## 5. Verification

- [x] 5.1 `go vet ./...` and the web build/lint/check scripts pass (no backend behavior changed, but confirm nothing else broke). Verified: `go build ./...`, `go vet ./...`, `pnpm run build`, `pnpm run check` (0 errors), `pnpm run lint` (exit 0, no touched-file findings), `pnpm exec vitest run` (830/830) all green.
- [ ] 5.2 Manual: create a profile from scratch — Role and Skills still required together in the set-up form, Save gating unchanged. **Skipped**: user opted out of standing up a full local stack (Postgres/Meilisearch/env) for browser QA in this worktree, given the automated coverage above plus two rounds of independent code review. Not verified in a live browser.
- [ ] 5.3 Manual: edit an existing profile — toggle a skill and an avoided skill on the new Skills tab, confirm each persists immediately with no Save click and survives a reload; spot-check the disabled-while-pending and error states. **Skipped**, same reason as 5.2.
- [ ] 5.4 Manual: upload a CV against an existing profile — confirm extracted skills land in the profile automatically and are visible on the Skills tab, without touching Settings' Save. **Skipped**, same reason as 5.2. This was also meant to empirically settle whether `resumeNote` renders under the `{#key profile.updated_at}` remount timing (see `design.md`'s residual-risk entry) — that question remains open.
