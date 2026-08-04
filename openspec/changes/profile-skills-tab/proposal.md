## Why

On `/my/profile`, Skills and Skills to avoid live inside the Settings tab's form,
bundled with Role, CV upload, headshot, and location preferences under one shared
"Save changes" button. They deserve their own top-level tab, edited independently
with immediate autosave — matching how the job-match block already edits a single
skill today.

## What Changes

- Add a top-level "Skills" tab to `/my/profile` (visible once a profile exists),
  containing the Skills and Skills to avoid controls.
- Toggling a skill or an avoided skill on the new tab writes to the profile
  immediately (one `PUT /me/profile` per toggle, via the existing per-skill
  mutators) — no Save button on this tab.
- Remove the Skills / Skills to avoid controls from the Settings form once a
  profile exists; the Settings form's first section becomes "Role" only. During
  first-time profile creation (no profile yet, no tabs), Skills stays in the
  creation form alongside Role, since creating a profile still requires at least
  one skill up front.
- CV upload against an **existing** profile now writes extracted skills — and
  any specialization the same extraction resolved — straight to the profile in
  one PUT, instead of buffering them in the Settings form until a manual Save.
  CV upload during first-time creation is unchanged.
- Add a `mergeResumeExtraction` mutator to the frontend profile store (single
  PUT for new skills plus specializations together), and a shared
  skill-dictionary loader used by both the Settings form and the new Skills tab.
- No backend changes: same `PUT /api/v1/me/profile` endpoint, called from a
  different place in the frontend.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `search-profiles`: the "Profile management UI" and "Populate profile skills
  from a resume" requirements change — skills editing moves to its own tab with
  autosave instead of being part of the shared-Save settings form.

## Impact

- Frontend only: `web/src/routes/my/profile/+page.svelte`,
  `web/src/lib/components/ProfileForm.svelte`,
  `web/src/lib/profile.svelte.ts`, `web/src/lib/skillDictionary.ts` (new),
  `web/src/lib/components/SkillsView.svelte` (new).
- No migrations, no API contract change, no other package affected.
