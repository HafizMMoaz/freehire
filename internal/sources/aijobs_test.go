package sources

import (
	"context"
	"fmt"
	neturl "net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// aijobsListingFixture is a trimmed real listing-page fragment (captured live): two job
// cards plus a company-profile link and a skill-filter link, neither of which is a job
// posting, to guard against over-matching.
const aijobsListingFixture = `<html><body><ul>
<li><a class="font-monospace fw-bold stretched-link" href="/job/data-specialist-petah-tikva-center-district-il-268449/">Data Specialist</a></li>
<li><a class="font-monospace fw-bold stretched-link" href="/job/lead-ai-engineer-tel-aviv-yafo-tel-aviv-district-il-268513/">Lead AI Engineer</a></li>
<li><a href="/company/medison-pharma-16767/">Medison Pharma</a></li>
<li><a href="/jobs/skill-python/">Python</a></li>
</ul></body></html>`

func TestAijobsListingLinksMatchesOnlyJobPostings(t *testing.T) {
	root, err := html.Parse(strings.NewReader(aijobsListingFixture))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	got := aijobsListingLinks(root)
	want := []string{
		"/job/data-specialist-petah-tikva-center-district-il-268449/",
		"/job/lead-ai-engineer-tel-aviv-yafo-tel-aviv-district-il-268513/",
	}
	if !slices.Equal(got, want) {
		t.Errorf("aijobsListingLinks = %v, want %v", got, want)
	}
}

// aijobsListingPage renders a listing page whose only content is one job card per id.
func aijobsListingPage(ids ...string) string {
	var b strings.Builder
	b.WriteString("<html><body><ul>")
	for _, id := range ids {
		b.WriteString(`<li><a href="/job/role-` + id + `/">role</a></li>`)
	}
	b.WriteString("</ul></body></html>")
	return b.String()
}

func seenSet(ids ...string) func(string) bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return func(id string) bool { return set[id] }
}

// aijobsPagedFake serves a fixed listing body per page number (matched on the "page=N"
// query parameter) and a plain session-bootstrap GET, so a test can shape exactly what
// crawlListing discovers per page without a real HTTP round trip.
func aijobsPagedFake(pages map[int]string) aijobsGetPostFake {
	return aijobsGetPostFake{postForm: func(url string) (*html.Node, error) {
		for page, body := range pages {
			if strings.Contains(url, fmt.Sprintf("page=%d", page)) {
				return html.Parse(strings.NewReader(body))
			}
		}
		return nil, fmt.Errorf("aijobsPagedFake: no route for %s", url)
	}}
}

func TestAijobsCrawlListingStopsWhenAPageIsFullySeen(t *testing.T) {
	fake := aijobsPagedFake(map[int]string{
		1: aijobsListingPage("1", "2"),
		2: aijobsListingPage("3"),
		3: aijobsListingPage("4", "5"), // already-seen page: caught up
		4: aijobsListingPage("99"),     // poison: must never be fetched
	})

	unseen, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet("4", "5"), 0)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	want := []string{"/job/role-1/", "/job/role-2/", "/job/role-3/"}
	if !slices.Equal(unseen, want) {
		t.Errorf("unseen = %v, want %v", unseen, want)
	}
}

func TestAijobsCrawlListingStopsAtNewBudget(t *testing.T) {
	fake := aijobsPagedFake(map[int]string{
		1: aijobsListingPage("1", "2", "3"),
		2: aijobsListingPage("99"), // poison: must never be fetched
	})

	unseen, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet(), 2)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	want := []string{"/job/role-1/", "/job/role-2/"}
	if !slices.Equal(unseen, want) {
		t.Errorf("unseen = %v, want %v (budget-capped)", unseen, want)
	}
}

// aijobsGetPostFake lets a test control the GET (session bootstrap) and POST (listing
// page) paths independently. routedHTTP can't: its routes match by URL substring only,
// and the bootstrap GET's URL is always a prefix of every paginated POST's URL (both hit
// the same "https://aijobs.net/[...]" host), so a route meant to fail only the POST would
// also swallow the GET, or vice versa.
type aijobsGetPostFake struct {
	postForm func(url string) (*html.Node, error)
}

func (f aijobsGetPostFake) GetHTML(context.Context, string) (*html.Node, error) {
	return html.Parse(strings.NewReader("<html></html>"))
}

func (f aijobsGetPostFake) PostFormWithHeaders(_ context.Context, url string, _ map[string]string, _ neturl.Values) (*html.Node, error) {
	return f.postForm(url)
}

func (f aijobsGetPostFake) CookieValue(string, string) string { return "test-csrf-token" }

func TestAijobsCrawlListingFirstPageFailureErrors(t *testing.T) {
	fake := aijobsGetPostFake{postForm: func(url string) (*html.Node, error) {
		return nil, fmt.Errorf("no route for %s", url) // bootstrap succeeds, page 1 fails
	}}
	if _, err := (aijobs{http: fake}).crawlListing(context.Background(), seenSet(), 0); err == nil {
		t.Error("expected an error when the first listing page fails, got nil")
	}
}

func TestAijobsCrawlListingLaterPageFailureKeepsWhatWasGathered(t *testing.T) {
	fake := aijobsPagedFake(map[int]string{1: aijobsListingPage("1")}) // no page=2: page 2 fails

	unseen, err := aijobs{http: fake}.crawlListing(context.Background(), seenSet(), 0)
	if err != nil {
		t.Fatalf("crawlListing: %v", err)
	}
	if want := []string{"/job/role-1/"}; !slices.Equal(unseen, want) {
		t.Errorf("unseen = %v, want %v", unseen, want)
	}
}

func TestAijobsJobID(t *testing.T) {
	cases := map[string]string{
		"/job/data-specialist-petah-tikva-center-district-il-268449/": "268449",
		"/job/lead-ai-engineer-268513/":                               "268513",
		"/company/medison-pharma-16767/":                              "",
		"/jobs/skill-python/":                                         "",
		"/job/no-trailing-id/":                                        "",
	}
	for href, want := range cases {
		if got := aijobsJobID(href); got != want {
			t.Errorf("aijobsJobID(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestAijobsProvider(t *testing.T) {
	if got := NewAijobs(nil).Provider(); got != "aijobs" {
		t.Errorf("Provider() = %q, want %q", got, "aijobs")
	}
}

func TestAijobsIsBoardlessAggregator(t *testing.T) {
	s := NewAijobs(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("aijobs should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("aijobs should implement the aggregator marker")
	}
}

func TestAijobsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["aijobs"]; !ok {
		t.Error("All() should register provider aijobs")
	}
	if !slices.Contains(FilterableProviders(), "aijobs") {
		t.Error("FilterableProviders() should include aijobs")
	}
}

func TestAijobsBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/aijobs.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/aijobs.yml fails validation: %v", err)
	}
}
