// Presentation for credential collections — company tags drawn from an
// authoritative public register rather than from our editorial judgement.
//
// Membership and the editorial/credential split come from the generated registry
// (cmd/gen-contracts ← internal/collections). What lives here is only the copy a
// badge needs: which body issued the licence, and the disclaimer that keeps the
// badge from reading as a promise about the role being viewed. A credential added
// upstream with no entry below renders nothing, which a test catches.
import { COLLECTIONS } from './generated/contracts';

export type CredentialBadge = {
  slug: string;
  /** The registry's own title, e.g. "Licensed UK sponsor". */
  label: string;
  /** The body that publishes the register, so a reader can verify it themselves. */
  issuer: string;
  /** Hover copy: what the credential means, and what it does not. */
  tooltip: string;
};

// The disclaimer is not optional decoration. A licence is held by an employer, for
// a route, at a moment in time; none of that says the role in front of the reader
// will be sponsored, and a candidate acting on the stronger reading wastes an
// application at best.
const COPY: Record<string, { issuer: string; tooltip: string }> = {
  'uk-skilled-worker-sponsor': {
    issuer: 'GOV.UK',
    tooltip:
      'This employer appears on the GOV.UK register of licensed sponsors for the Skilled Worker and related work routes. The licence belongs to the employer and is not a commitment to sponsor this role.',
  },
  'nl-recognised-sponsor': {
    issuer: 'IND',
    tooltip:
      'This employer appears on the IND public register of recognised sponsors for work and highly skilled migrants. The recognition belongs to the employer and is not a commitment to sponsor this role.',
  },
};

/**
 * Badges for the credential tags in a company's or job's collection list, in
 * registry order. Editorial collections, unknown slugs and an absent list all yield
 * nothing — a tag we cannot describe is better shown as nothing than as a guess.
 */
export function credentialBadges(collections: string[] | undefined | null): CredentialBadge[] {
  if (!collections?.length) return [];
  const held = new Set(collections);
  return COLLECTIONS.filter((c) => c.kind === 'credential' && held.has(c.slug)).flatMap((c) => {
    // flatMap, not filter-then-map: only the lookup narrows COPY's index signature,
    // so reading it once is what makes the result total rather than a cast.
    const copy = COPY[c.slug];
    return copy ? [{ slug: c.slug, label: c.title, issuer: copy.issuer, tooltip: copy.tooltip }] : [];
  });
}
