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
// initial HTML. A 404 from the API becomes a SvelteKit 404 page (not a 200
// shell); other failures bubble to the 500 page.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  // Both fetches key only on the slug and are independent, so run them in parallel
  // — serialising them cost a full API round-trip on every job page. They stay
  // awaited (not streamed) so the "Similar jobs" rows remain in the SSR HTML for
  // internal-link crawlability.
  //
  // Similar jobs are a non-essential discovery aid: a failure (search disabled,
  // no neighbours yet) must not break the page, so it degrades to an empty list.
  //
  // The application form is known for a minority of postings — only a few ATS platforms
  // publish one we can read — so its absence is the ordinary case, not a failure. It
  // degrades to null for the same reason the two below degrade to empty: nothing on this
  // page may be able to break the page.
  const [job, similar, copiesResult, applyForm] = await Promise.all([
    api.getJob(params.slug).catch((e) => {
      if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
      throw e;
    }),
    api.getSimilarJobs(params.slug).catch(() => []),
    // A small preview of the other-locations tab (the full list is /jobs/:slug/copies).
    // Non-essential and only meaningful for a mass-posted role, so it degrades to empty.
    api.getJobCopies(params.slug, 10).catch(() => ({ copies: [], total: 0 })),
    api.getApplyForm(params.slug).catch(() => null),
  ]);

  // The "see also" block's matched collections, each with its live open-job
  // count — needs `job`'s own facets, so it can't join the Promise.all above.
  // Bounded to relatedCollectionLinks' target (~5), so this is a handful of
  // small parallel requests, not a scan.
  const seeAlso: SeeAlsoCard[] = await Promise.all(
    relatedCollectionLinks(jobFacetsFromJob(job)).map(async (link): Promise<SeeAlsoCard> => {
      const resolved = collectionBySlug(link.slug);
      let count: number | null = null;
      if (resolved) {
        count = (await api.searchJobs(new URLSearchParams(resolved.params), 0, 0).catch(() => null))
          ?.total ?? null;
      }
      return { slug: link.slug, title: link.title, count, mark: backerBadges([link.slug])[0]?.mark ?? null };
    })
  );

  return {
    job,
    similar,
    copies: copiesResult.copies,
    copiesTotal: copiesResult.total,
    applyForm,
    seeAlso,
  };
};
