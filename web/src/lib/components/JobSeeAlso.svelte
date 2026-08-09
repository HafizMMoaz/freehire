<script lang="ts">
  import { resolve } from '$app/paths';
  import { relatedCollectionLinks, type JobFacets } from '$lib/collections';
  import type { Job } from '$lib/types';

  // A bounded row of links into existing /collections/:slug pages, built from
  // this job's own facets and its company's collections — see design's
  // relatedCollectionLinks. No fetch: everything it needs is already on `job`.
  let { job }: { job: Job } = $props();

  const facets = $derived<JobFacets>({
    category: job.enrichment.category,
    seniority: job.enrichment.seniority,
    skills: job.skills,
    workMode: job.work_mode,
    countries: job.countries,
    regions: job.regions,
    collections: job.collections,
  });

  const links = $derived(relatedCollectionLinks(facets));
</script>

{#if links.length > 0}
  <section class="mt-8">
    <h2 class="mb-3 text-sm font-semibold text-muted-foreground">See also</h2>
    <div class="flex flex-wrap gap-2">
      {#each links as link (link.slug)}
        <a
          href={resolve('/collections/[slug]', { slug: link.slug })}
          class="rounded-md border border-border px-3 py-1.5 text-sm font-medium text-foreground hover:bg-muted"
        >
          {link.title}
        </a>
      {/each}
    </div>
  </section>
{/if}
