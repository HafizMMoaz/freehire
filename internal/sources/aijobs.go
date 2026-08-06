package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// aijobs adapts aijobs.net, a large AI/ML job aggregator (~47k postings). It is boardless
// (one global feed, no per-tenant board) and multi-company, so it stays in the source facet
// and takes each posting's company from the feed. The listing is a Django CSRF-protected
// POST endpoint (not a public JSON API), so the adapter needs a cookie-jar-backed client
// (built with newCookieClient in the registry, mirroring taleo's session-bound flow) rather
// than the shared client.
type aijobs struct {
	http aijobsHTTP
}

// aijobsHTTP is the transport aijobs needs: the CSRF-protected listing POST and the
// per-posting detail-page GET.
type aijobsHTTP interface {
	HeaderFormPoster
	HTMLGetter
	CookieReader
}

// NewAijobs builds the aijobs adapter over the given HTTP client (a cookie-jar-backed
// client in production, see cookieSessionSource in registry.go).
func NewAijobs(c aijobsHTTP) Source { return aijobs{http: c} }

func (aijobs) Provider() string { return "aijobs" }

// aijobs needs no board id (one global feed), so its config carries no board.
func (aijobs) boardless() {}

// aijobs aggregates postings from many companies, so it stays in the source facet.
func (aijobs) aggregator() {}

// Fetch is a placeholder satisfying the Source interface; the real listing walk and
// hydrating fetch land in later tasks (FetchNew is the path cmd/ingest actually drives).
func (aijobs) Fetch(context.Context, CompanyEntry) ([]Job, error) { return nil, nil }

const aijobsBaseURL = "https://aijobs.net"

// aijobsJobIDPattern matches a job-detail path (/job/<slug>-<id>/) and captures the
// trailing numeric id, anchored so a company or skill-filter link (which shares no such
// suffix) never matches.
var aijobsJobIDPattern = regexp.MustCompile(`^/job/[^/]+-(\d+)/?$`)

// aijobsJobID extracts the native aijobs.net posting id from a job-detail href, "" when
// href is not a job-detail link.
func aijobsJobID(href string) string {
	m := aijobsJobIDPattern.FindStringSubmatch(href)
	if m == nil {
		return ""
	}
	return m[1]
}

// aijobsListingLinks collects every job-detail link on a listing page, in document order
// (the feed's own newest-first sort), filtering out the company/skill/education/role
// filter links a listing card also carries.
func aijobsListingLinks(root *html.Node) []string {
	var hrefs []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := attr(n, "href"); aijobsJobIDPattern.MatchString(href) {
				hrefs = append(hrefs, href)
			}
		}
		return true
	})
	return hrefs
}

// aijobsMaxPages is the hard pagination safety cap, independent of the seen-based stop
// (see crawlListing): ~47k postings at 50/page is ~956 pages, so this leaves headroom
// while still bounding a run if the feed's sort order or markup ever silently changes.
// A var, not a const, so a test can shrink it rather than paginate to the real cap.
var aijobsMaxPages = 1200

// bootstrapSession GETs the aijobs.net home page to obtain the csrftoken cookie the
// listing POST must echo back (Django's double-submit CSRF pattern — see
// Client.CookieValue). Returns an error if the site sets no such cookie.
func (a aijobs) bootstrapSession(ctx context.Context) (string, error) {
	if _, err := a.http.GetHTML(ctx, aijobsBaseURL+"/"); err != nil {
		return "", err
	}
	token := a.http.CookieValue(aijobsBaseURL+"/", "csrftoken")
	if token == "" {
		return "", fmt.Errorf("aijobs: no csrftoken cookie set by %s", aijobsBaseURL)
	}
	return token, nil
}

// listPage requests one listing page, authorized by the session's csrfToken (echoed as
// both the x-csrftoken header and the csrfmiddlewaretoken form field — the value the
// cookie already carries, so no separate per-page token scrape is needed). Referer is
// required: aijobs.net's CSRF check rejects a same-value token without it.
func (a aijobs) listPage(ctx context.Context, csrfToken string, page int) (*html.Node, error) {
	values := url.Values{
		"csrfmiddlewaretoken": {csrfToken},
	}
	headers := map[string]string{
		"x-csrftoken": csrfToken,
		"referer":     aijobsBaseURL + "/",
		// The site's own frontend paginates this endpoint via htmx (confirmed live), which
		// marks every one of its requests this way; sent for parity with the real client.
		"hx-request": "true",
	}
	return a.http.PostFormWithHeaders(ctx, fmt.Sprintf("%s/?page=%d", aijobsBaseURL, page), headers, values)
}

// crawlListing walks the listing pages (newest posting first) and returns the job-detail
// hrefs of postings seen reports as NOT already in the catalogue, in discovery order. It
// stops — without requesting a further page — as soon as either condition holds:
//   - a whole page's postings are all already seen: the feed has been caught up to, so
//     every following page (older still) would be too;
//   - newBudget unseen postings have been found (0 means unbounded): this run's
//     per-run detail-fetch budget (see AIJOBS_MAX_NEW_PER_RUN); the postings past the
//     cap remain unseen and are picked up on a later run.
//
// A first-page failure is a board-level error; a later page failing ends the walk with
// what was gathered so far (a partial crawl survives a mid-listing hiccup).
func (a aijobs) crawlListing(ctx context.Context, seen func(externalID string) bool, newBudget int) ([]string, error) {
	csrfToken, err := a.bootstrapSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("aijobs: session bootstrap: %w", err)
	}

	var unseen []string
	for page := 1; page <= aijobsMaxPages; page++ {
		root, err := a.listPage(ctx, csrfToken, page)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("aijobs: listing page %d: %w", page, err)
			}
			break
		}
		links := aijobsListingLinks(root)
		if len(links) == 0 {
			break // empty page, or a page number clamped past the last one
		}
		allSeen := true
		for _, href := range links {
			id := aijobsJobID(href)
			if id == "" || seen(id) {
				continue
			}
			allSeen = false
			unseen = append(unseen, href)
			if newBudget > 0 && len(unseen) >= newBudget {
				return unseen, nil
			}
		}
		if allSeen {
			break // caught up to already-known postings; every later page is older still
		}
	}
	return unseen, nil
}
