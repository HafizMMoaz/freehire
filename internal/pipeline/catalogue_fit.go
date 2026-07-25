package pipeline

import (
	"log"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/sources"
)

// outOfCatalogue reports whether a crawled posting does not belong on an IT job board
// and must not be persisted. A generic ATS board carries a company's whole hiring, not
// its engineering org, so most of what a crawl returns is nurses, cooks and cashiers.
//
// The rule is the non-tech TITLE dictionary, deliberately not the tri-state is_tech. A
// business role — sales, recruiting, finance — resolves is_tech=false through its
// category, but whether it belongs in the catalogue depends on whether the company is
// an IT company, which the crawl cannot know and the prune worker decides later.
// Rejecting on is_tech would quietly drop every sales and recruiting job at every
// employer on the board.
//
// This is catalogue policy, not derivation, which is why it lives here rather than in
// jobderive: the aggregate still constructs a non-technical posting without complaint,
// and the write paths that are never re-crawled — Telegram extraction, user
// submissions, link-source imports — go through that factory untouched. Rejection is
// only safe on a crawled board, where removing an over-broad dictionary term re-admits
// the postings on the next pass.
func outOfCatalogue(j job.Job) bool {
	return classify.IsNonTech(j.Fields().Title)
}

// logRejections reports a board's catalogue-filter rejections, once per board rather
// than once per posting. The share matters more than the count: a board rejecting
// everything is the signature of a dictionary term that is too broad, and since boards
// are crawled hourly that has to be visible within the hour — long before the next
// pruning pass acts on the same dictionary.
func logRejections(e sources.CompanyEntry, rejected, total int) {
	// A rejection is counted only for a posting the board yielded, so total is at
	// least rejected and the division is safe.
	if rejected == 0 {
		return
	}
	log.Printf("ingest: %s board %q (%s): rejected %d/%d postings as non-technical (%.0f%%)",
		e.Provider, e.Board, e.Company, rejected, total, 100*float64(rejected)/float64(total))
}
