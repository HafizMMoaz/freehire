package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

// nodesk adapts nodesk.co, a remote-jobs board. Boardless (one public RSS feed, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. All-remote board, so every posting is remote. The feed
// carries every posting's body inline (no detail call).
//
// Fetched via GetText rather than GetXML: unlike weworkremotely's feed (which wraps its body
// in CDATA, leaving embedded entities as literal text), NoDesk's <description> contains raw,
// unwrapped named entities such as "&rsquo;" that are not part of the XML entity set — the
// standard library's strict decoder rejects them ("invalid character entity"). Decoding with
// Strict=false leaves an unrecognized entity as literal text instead of erroring, and
// html.UnescapeString (same helper weworkremotely already uses for its CDATA content) then
// resolves it.
type nodesk struct {
	http TextGetter
}

const nodeskFeedURL = "https://nodesk.co/remote-jobs/index.xml"

// NewNoDesk builds the NoDesk adapter over the given HTTP client.
func NewNoDesk(c TextGetter) Source { return nodesk{http: c} }

func (nodesk) Provider() string { return "nodesk" }

func (nodesk) boardless() {}

func (nodesk) aggregator() {}

// nodeskItem is one RSS <item>: the title is "Role at Company", description is the plain-text
// body, and guid is the native posting id (same value as link).
type nodeskItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

func (s nodesk) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	raw, err := s.http.GetText(ctx, nodeskFeedURL)
	if err != nil {
		return nil, fmt.Errorf("nodesk: feed: %w", err)
	}
	var feed struct {
		Channel struct {
			Items []nodeskItem `xml:"item"`
		} `xml:"channel"`
	}
	dec := xml.NewDecoder(strings.NewReader(raw))
	dec.Strict = false
	if err := dec.Decode(&feed); err != nil {
		return nil, fmt.Errorf("nodesk: feed: %w", err)
	}
	jobs := make([]Job, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		if job, ok := it.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps an RSS item to a Job, returning ok=false for an unusable item (no guid to key
// on, or no " at " split which would leave the company empty and break the slug).
func (it nodeskItem) toJob() (Job, bool) {
	title, company, ok := strings.Cut(it.Title, " at ")
	if it.GUID == "" || !ok || company == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  it.GUID,
		URL:         it.Link,
		Title:       xhtml.UnescapeString(strings.TrimSpace(title)),
		Company:     xhtml.UnescapeString(strings.TrimSpace(company)),
		Location:    "Remote",
		Description: sanitizeHTML(xhtml.UnescapeString(it.Description)),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseLayout(time.RFC1123Z, it.PubDate),
	}, true
}
