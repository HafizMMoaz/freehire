import type { RequestHandler } from './$types';

// A real robots file (not the SPA shell): allow crawling the public pages, keep
// personal pages out, and point at the sitemap.
//
// The wildcard already admits every AI crawler (GPTBot, ClaudeBot, PerplexityBot,
// …), so no per-bot Allow block is listed — a redundant one would only invite
// drift from this rule. llms.txt has no robots directive of its own, so it is
// advertised as a comment, the convention crawlers look for.
//
// /jobs/*/discussion/new and /companies/*/discussion/new are the empty
// new-thread form, linked from every single job and company page — crawling it
// costs a full SSR render per job/company for a page with no unique content, and
// it drove a real accept-queue incident (2026-08-05, ClaudeBot alone made
// ~108k requests to it in 12.5h). Actual thread pages (/discussion,
// /discussion/[id]) hold real content and stay crawlable.
export const GET: RequestHandler = ({ url }) => {
  const body = `User-agent: *
Allow: /
Disallow: /my/
Disallow: /jobs/*/discussion/new
Disallow: /companies/*/discussion/new

Sitemap: ${url.origin}/sitemap.xml
# llms.txt: ${url.origin}/llms.txt
`;
  return new Response(body, {
    headers: {
      'content-type': 'text/plain; charset=utf-8',
      'cache-control': 'public, max-age=86400',
    },
  });
};
