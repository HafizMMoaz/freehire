// Skill diff between a tailored CV document and the search/match profile.
// Used by the tailor "Add new skills to profile" control — never runs as a
// side effect of save, Download, or Reset.

import type { Document } from '$lib/generated/contracts';
import type { ProfileSkillSets } from '$lib/profileSkills';

/** Profile wanted-skill cap — mirrors internal/userprofile.maxSkills. */
export const PROFILE_MAX_SKILLS = 200;

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/** Canonical skill tokens listed on the CV (skill group items), lowercased, de-duped. */
export function skillsFromDocument(doc: Document): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const group of doc.skills ?? []) {
    for (const raw of group.items ?? []) {
      const s = raw.trim().toLowerCase();
      if (!s || seen.has(s)) continue;
      seen.add(s);
      out.push(s);
    }
  }
  return out;
}

export interface SkillsToAddResult {
  /** Skills to merge into the profile (already capped). */
  skills: string[];
  /** True when more CV skills were eligible than fit under the cap. */
  capped: boolean;
}

/**
 * Skills on the CV that are not yet wanted and not excluded. Empty when there is
 * no profile (do not invent one) or no remaining diff.
 */
export function skillsToAddToProfile(
  doc: Document,
  profile: ProfileSkillSets | null,
  maxSkills: number = PROFILE_MAX_SKILLS,
): SkillsToAddResult {
  if (!profile) return { skills: [], capped: false };

  const held = profile.skills;
  const excluded = profile.excluded_skills;
  const candidates = skillsFromDocument(doc).filter(
    (s) => !held.some((h) => same(h, s)) && !excluded.some((e) => same(e, s)),
  );

  const room = Math.max(0, maxSkills - held.length);
  if (candidates.length <= room) {
    return { skills: candidates, capped: false };
  }
  return { skills: candidates.slice(0, room), capped: true };
}
