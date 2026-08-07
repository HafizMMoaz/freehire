import { describe, it, expect } from 'vitest';
import type { Document } from '$lib/generated/contracts';
import { withSkills } from '../profileSkills';
import { PROFILE_MAX_SKILLS, skillsFromDocument, skillsToAddToProfile } from './skillDiff';

function docWithSkills(...items: string[]): Document {
  return {
    margins: { top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 },
    style: {},
    header: {},
    skills: [{ group: 'Skills', items }],
  };
}

describe('skillsFromDocument', () => {
  it('collects and lowercases items across groups', () => {
    const doc: Document = {
      margins: { top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 },
      style: {},
      header: {},
      skills: [
        { group: 'Lang', items: ['Go', 'TypeScript'] },
        { group: 'Ops', items: ['kafka', 'Go'] },
      ],
    };
    expect(skillsFromDocument(doc)).toEqual(['go', 'typescript', 'kafka']);
  });

  it('returns empty when skills are missing', () => {
    expect(
      skillsFromDocument({
        margins: { top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 },
        style: {},
        header: {},
      }),
    ).toEqual([]);
  });
});

describe('skillsToAddToProfile', () => {
  it('returns empty when there is no profile', () => {
    expect(skillsToAddToProfile(docWithSkills('kafka'), null)).toEqual({
      skills: [],
      capped: false,
    });
  });

  it('lists CV skills not yet wanted and not excluded', () => {
    expect(
      skillsToAddToProfile(docWithSkills('Go', 'kafka', 'Redis'), {
        skills: ['go'],
        excluded_skills: ['redis'],
      }),
    ).toEqual({ skills: ['kafka'], capped: false });
  });

  it('returns empty when there is no remaining diff', () => {
    expect(
      skillsToAddToProfile(docWithSkills('Go'), {
        skills: ['go'],
        excluded_skills: [],
      }),
    ).toEqual({ skills: [], capped: false });
  });

  it('caps additions so wanted skills stay within maxSkills', () => {
    const held = Array.from({ length: PROFILE_MAX_SKILLS - 1 }, (_, i) => `s${i}`);
    const result = skillsToAddToProfile(docWithSkills('kafka', 'terraform', 'nix'), {
      skills: held,
      excluded_skills: [],
    });
    expect(result.skills).toEqual(['kafka']);
    expect(result.capped).toBe(true);
  });

  it('matches held/excluded case-insensitively', () => {
    expect(
      skillsToAddToProfile(docWithSkills('Kafka'), {
        skills: ['KAFKA'],
        excluded_skills: [],
      }),
    ).toEqual({ skills: [], capped: false });
  });

  it('composes with withSkills for the tailor confirm path', () => {
    const profile = { skills: ['go'], excluded_skills: [] as string[] };
    const { skills, capped } = skillsToAddToProfile(docWithSkills('kafka', 'redis'), profile);
    expect(capped).toBe(false);
    expect(withSkills(profile, skills)).toEqual({
      skills: ['go', 'kafka', 'redis'],
      excluded_skills: [],
    });
  });
});
