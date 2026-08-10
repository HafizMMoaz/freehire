package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

// cryptocurrencyjobs adapts cryptocurrencyjobs.co, a Web3/crypto-niche remote-jobs board.
// Boardless (one public RSS feed, no per-tenant board) and multi-company, so it stays in the
// source facet and takes each posting's company from the feed. All-remote board, so every
// posting is remote. The feed carries every posting's body inline (no detail call).
//
// Fetched via GetText rather than GetXML and decoded leniently (Strict=false), same as the
// nodesk adapter: this board runs the same underlying feed generator (identical "Role at
// Company" / plain-text-description / guid-equals-link shape, same host family as nodesk.co),
// which is known to embed raw named entities such as "&rsquo;" outside CDATA — invalid XML
// that the strict decoder rejects. html.UnescapeString then resolves whatever the lenient
// decode left as literal entity text.
type cryptocurrencyjobs struct {
	http TextGetter
}

const cryptocurrencyjobsFeedURL = "https://cryptocurrencyjobs.co/index.xml"

// NewCryptocurrencyJobs builds the Cryptocurrency Jobs adapter over the given HTTP client.
func NewCryptocurrencyJobs(c TextGetter) Source { return cryptocurrencyjobs{http: c} }

func (cryptocurrencyjobs) Provider() string { return "cryptocurrencyjobs" }

func (cryptocurrencyjobs) boardless() {}

func (cryptocurrencyjobs) aggregator() {}

// cryptocurrencyjobsItem is one RSS <item>: the title is "Role at Company", description is
// the plain-text body, and guid is the native posting id (same value as link).
type cryptocurrencyjobsItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

func (s cryptocurrencyjobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	raw, err := s.http.GetText(ctx, cryptocurrencyjobsFeedURL)
	if err != nil {
		return nil, fmt.Errorf("cryptocurrencyjobs: feed: %w", err)
	}
	var feed struct {
		Channel struct {
			Items []cryptocurrencyjobsItem `xml:"item"`
		} `xml:"channel"`
	}
	dec := xml.NewDecoder(strings.NewReader(raw))
	dec.Strict = false
	if err := dec.Decode(&feed); err != nil {
		return nil, fmt.Errorf("cryptocurrencyjobs: feed: %w", err)
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
func (it cryptocurrencyjobsItem) toJob() (Job, bool) {
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
