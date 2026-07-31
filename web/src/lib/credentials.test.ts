import { describe, expect, it } from 'vitest';
import { COLLECTIONS } from './generated/contracts';
import { credentialBadges } from './credentials';

describe('credentialBadges', () => {
  it('surfaces a credential tag as a badge naming its issuing register', () => {
    const badges = credentialBadges(['uk-skilled-worker-sponsor']);
    expect(badges).toHaveLength(1);
    expect(badges[0]?.label).toBe('Licensed UK sponsor');
    expect(badges[0]?.issuer).toBe('GOV.UK');
  });

  it('always carries the disclaimer that the licence is the employer’s', () => {
    // The whole point of the badge is that it states a fact about the employer. A
    // reader must not take it as a promise about the role they are looking at.
    for (const badge of credentialBadges(['uk-skilled-worker-sponsor', 'nl-recognised-sponsor'])) {
      expect(badge.tooltip).toMatch(/not a (commitment|promise)/i);
      expect(badge.tooltip).toMatch(/employer/i);
    }
  });

  it('ignores editorial collections', () => {
    expect(credentialBadges(['yc', 'bigtech', 'unicorn'])).toEqual([]);
  });

  it('ignores unknown tags rather than inventing a badge', () => {
    expect(credentialBadges(['not-a-collection', ''])).toEqual([]);
  });

  it('handles an absent tag list', () => {
    expect(credentialBadges(undefined)).toEqual([]);
    expect(credentialBadges([])).toEqual([]);
  });

  it('keeps registry order when a company holds several credentials', () => {
    const both = credentialBadges(['nl-recognised-sponsor', 'uk-skilled-worker-sponsor']);
    expect(both.map((b) => b.slug)).toEqual(['uk-skilled-worker-sponsor', 'nl-recognised-sponsor']);
  });

  it('covers every credential in the generated registry', () => {
    // A credential added to the Go registry with no badge copy here would tag jobs
    // and filter them while rendering nothing — this is what catches that.
    const credentials = COLLECTIONS.filter((c) => c.kind === 'credential').map((c) => c.slug);
    expect(credentialBadges(credentials).map((b) => b.slug)).toEqual(credentials);
  });
});
