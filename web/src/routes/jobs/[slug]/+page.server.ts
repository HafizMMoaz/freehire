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
import type { PageServerLoad } from './$types';

// Server-render the job detail: fetch by slug so the article content is in the
// initial HTML. All fetches stay awaited (not streamed) so every section is in
// the SSR HTML for internal-link crawlability.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);

  const jobPromise = api.getJob(params.slug).catch((e) => {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    throw e;
  });

  // The "see also" block's matched collections, each with its live open-job
  // count. It needs `job`'s own facets, so it can only start once `job`
  // resolves — but it's kicked off right here (not after the Promise.all
  // below) so it overlaps with similar/copies/applyForm instead of adding a
  // second sequential wave after them.
  //
  // Every current collection's count is a single facet's value under the
  // WHOLE catalogue (skills=react, category=backend, collections=yc, …) —
  // readable straight off one shared, unfiltered facetCounts() distribution,
  // no per-card request needed. Only the remote+region/country combos
  // (work_mode AND regions/countries — two keys) can't be read that way: a
  // facet distribution is a marginal count per field under a filter, not a
  // joint count across two fields, so those still need their own direct
  // query. In practice a job matches at most one such combo, so this is
  // usually 1 request total, never more than 2.
  const seeAlsoPromise = jobPromise.then(async (job) => {
    const links = relatedCollectionLinks(jobFacetsFromJob(job));
    const facetsResult = await api.facetCounts(new URLSearchParams()).catch(() => null);

    return Promise.all(
      links.map(async (link): Promise<SeeAlsoCard> => {
        const mark = backerBadges([link.slug])[0]?.mark ?? null;
        // relatedCollectionLinks already resolves every slug it returns via this
        // same collectionBySlug, so `resolved` is guaranteed here — this is TS
        // narrowing for the `resolved.params` access below, not a real fallback path.
        const resolved = collectionBySlug(link.slug);
        if (!resolved) return { slug: link.slug, title: link.title, count: null, mark };

        const paramEntries = Object.entries(resolved.params);
        let count: number | null;
        if (paramEntries.length === 1) {
          // Just verified length === 1, so index 0 exists; noUncheckedIndexedAccess
          // can't see that from the length check alone.
          const [key, value] = paramEntries[0]!;
          count = facetsResult ? (facetsResult.facets[key]?.[value] ?? 0) : null;
        } else {
          count =
            (await api.searchJobs(new URLSearchParams(resolved.params), 0, 0).catch(() => null))
              ?.total ?? null;
        }
        return { slug: link.slug, title: link.title, count, mark };
      })
    );
  });

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
