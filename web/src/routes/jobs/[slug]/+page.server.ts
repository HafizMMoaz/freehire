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

// A see-also card's live count. Every current collection is a single facet's
// value under the whole catalogue (skills=react, category=backend,
// collections=yc, …) — read straight off the shared, unfiltered
// facetCounts() distribution passed in, no per-card request. Only the
// remote+region/country combos (two AND'd keys, e.g. work_mode + regions)
// need their own direct query: a facet distribution is a marginal count per
// field under a filter, not a joint count across two fields.
async function seeAlsoCount(
  params: Record<string, string>,
  facetsResult: FacetCounts | null,
  api: Api
): Promise<number | null> {
  const entries = Object.entries(params);
  if (entries.length > 1) {
    return (await api.searchJobs(new URLSearchParams(params), 0, 0).catch(() => null))?.total ?? null;
  }
  const [key, value] = entries[0]!; // length === 1 just checked above
  // A missing entry here is never a genuine zero: every card reaching this
  // branch is either the job's own facet/collection (so the job itself is
  // ≥1) or a fallback pool entry curated to have a healthy count. It means
  // Meilisearch's MaxValuesPerFacet cap dropped this value from the
  // distribution — degrade to unresolved, not a false "0 jobs".
  return facetsResult ? (facetsResult.facets[key]?.[value] ?? null) : null;
}

// The "see also" block's matched collections, each with its live open-job
// count. Callers kick this off the moment `job` resolves (it needs `job`'s
// own facets) so it overlaps with the page's other fetches instead of adding
// a second sequential wave after them. In practice a job matches at most one
// remote+region/country collection, so this is usually 1 request total
// (the shared facetCounts() call), never more than 2.
async function buildSeeAlso(job: Job, api: Api): Promise<SeeAlsoCard[]> {
  const links = relatedCollectionLinks(jobFacetsFromJob(job));
  const facetsResult = await api.facetCounts(new URLSearchParams()).catch(() => null);

  return Promise.all(
    links.map(async (link): Promise<SeeAlsoCard> => {
      const mark = backerBadges([link.slug])[0]?.mark ?? null;
      // relatedCollectionLinks already resolves every slug it returns via this
      // same collectionBySlug, so `resolved` is guaranteed here — this is TS
      // narrowing, not a real fallback path.
      const resolved = collectionBySlug(link.slug);
      const count = resolved ? await seeAlsoCount(resolved.params, facetsResult, api) : null;
      return { slug: link.slug, title: link.title, count, mark };
    })
  );
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
