package sources

import "context"

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
