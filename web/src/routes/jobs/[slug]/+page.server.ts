import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { backerBadges } from '$lib/backers';
import {
  collectionBySlug,
  jobFacetsFromJob,
  relatedCollectionLinks,
  type SeeAlsoCard,
} from '$lib/collections';
import { serverApi } from '$lib/server/api';
import type { FacetCounts, Job } from '$lib/types';
import type { PageServerLoad } from './$types';

type Api = ReturnType<typeof serverApi>;

// A missing facets[key][value] entry is never a genuine zero: every card
// reaching seeAlsoCount is either the job's own facet/collection (so the job
// itself is ≥1) or a fallback pool entry curated to have a healthy count. It
// means Meilisearch's MaxValuesPerFacet cap dropped this value from the
// distribution — degrade to unresolved, not a false "0 jobs".
function readCount(facetsResult: FacetCounts | null, key: string, value: string): number | null {
  return facetsResult ? (facetsResult.facets[key]?.[value] ?? null) : null;
}

// The "see also" block's matched collections, each with its live open-job
// count. Kicked off the moment `job` resolves (it needs `job`'s own facets)
// so it overlaps with the page's other fetches instead of adding a second
// sequential wave after them.
//
// Every current collection's count is readable off a facetCounts()
// distribution, never a per-card searchJobs call:
// - A single-key collection (skills=react, category=backend, collections=yc,
//   …) reads straight off ONE shared, unfiltered distribution covering the
//   whole catalogue.
// - A multi-key collection (the ten remote+region/country combos, e.g.
//   work_mode=remote AND regions=global) needs its LAST key's value read
//   from a distribution filtered by its other keys — a facet distribution is
//   a marginal count per field under a filter, not a joint count across
//   fields. All ten currently share the same filter (work_mode=remote), so
//   they're grouped by filter and share ONE extra call: a job open in
//   several countries at once (matching several remote-* collections) still
//   costs one request, not one per match.
async function buildSeeAlso(job: Job, api: Api): Promise<SeeAlsoCard[]> {
  const links = relatedCollectionLinks(jobFacetsFromJob(job));
  const resolvedLinks = links.map((link) => ({ link, resolved: collectionBySlug(link.slug) }));

  const baseFacets = await api.facetCounts(new URLSearchParams()).catch(() => null);

  const multiKeyGroups = new Map<string, { filter: Record<string, string>; lastKey: string }>();
  for (const { resolved } of resolvedLinks) {
    const entries = resolved ? Object.entries(resolved.params) : [];
    if (entries.length <= 1) continue;
    const [lastKey] = entries[entries.length - 1]!;
    const filter = Object.fromEntries(entries.slice(0, -1));
    multiKeyGroups.set(new URLSearchParams(filter).toString(), { filter, lastKey });
  }
  const groupedFacets = new Map<string, FacetCounts | null>(
    await Promise.all(
      [...multiKeyGroups.entries()].map(
        async ([groupKey, { filter }]): Promise<[string, FacetCounts | null]> => [
          groupKey,
          await api.facetCounts(new URLSearchParams(filter)).catch(() => null),
        ]
      )
    )
  );

  return resolvedLinks.map(({ link, resolved }): SeeAlsoCard => {
    const mark = backerBadges([link.slug])[0]?.mark ?? null;
    // relatedCollectionLinks already resolves every slug it returns via this
    // same collectionBySlug, so `resolved` is guaranteed here — this is TS
    // narrowing, not a real fallback path.
    if (!resolved) return { slug: link.slug, title: link.title, count: null, mark };

    const entries = Object.entries(resolved.params);
    let count: number | null;
    if (entries.length === 1) {
      const [key, value] = entries[0]!;
      count = readCount(baseFacets, key, value);
    } else {
      const [lastKey, lastValue] = entries[entries.length - 1]!;
      const groupKey = new URLSearchParams(Object.fromEntries(entries.slice(0, -1))).toString();
      count = readCount(groupedFacets.get(groupKey) ?? null, lastKey, lastValue);
    }
    return { slug: link.slug, title: link.title, count, mark };
  });
}

// Server-render the job detail: fetch by slug so the article content is in the
// initial HTML. All fetches stay awaited (not streamed) so every section is in
// the SSR HTML for internal-link crawlability.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);

  const jobPromise = api.getJob(params.slug).catch((e) => {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    throw e;
  });
  const seeAlsoPromise = jobPromise.then((job) => buildSeeAlso(job, api));

  // Similar jobs are a non-essential discovery aid: a failure (search disabled,
  // no neighbours yet) must not break the page, so it degrades to an empty list.
  //
  // The application form is known for a minority of postings — only a few ATS platforms
  // publish one we can read — so its absence is the ordinary case, not a failure. It
  // degrades to null for the same reason the two below degrade to empty: nothing on this
  // page may be able to break the page.
  const [job, similar, copiesResult, applyForm, seeAlso] = await Promise.all([
    jobPromise,
    api.getSimilarJobs(params.slug).catch(() => []),
    // A small preview of the other-locations tab (the full list is /jobs/:slug/copies).
    // Non-essential and only meaningful for a mass-posted role, so it degrades to empty.
    api.getJobCopies(params.slug, 10).catch(() => ({ copies: [], total: 0 })),
    api.getApplyForm(params.slug).catch(() => null),
    seeAlsoPromise,
  ]);

  return {
    job,
    similar,
    copies: copiesResult.copies,
    copiesTotal: copiesResult.total,
    applyForm,
    seeAlso,
  };
};
