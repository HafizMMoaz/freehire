// The skills typeahead's candidate universe — canonical tokens with live job counts,
// shared by the profile form and the Skills tab (both need the same source the job-search
// filter panel uses, not a bundled/static list).

import { api } from '$lib/api';
import { dynamicOptions, type FacetOption } from '$lib/facets';

/** Fetch the live skills distribution and shape it into sorted typeahead options.
 *  Best-effort: any failure (network, decode) resolves to an empty list rather than
 *  throwing, so a caller can render "nothing to suggest yet" instead of an error. */
export async function loadSkillDistribution(): Promise<FacetOption[]> {
  try {
    const counts = await api.facetCounts(new URLSearchParams());
    return dynamicOptions('skills', counts.facets?.skills ?? {}, []);
  } catch {
    return [];
  }
}
