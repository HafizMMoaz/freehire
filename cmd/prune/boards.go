package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
)

// notBoardFiles are files under sources/ that are not board lists. telegram.yml is the
// channel list cmd/tg-ingest crawls — a `channels:` mapping, not a list of companies —
// so parsing it as a board file fails outright. Named explicitly rather than skipped on
// parse error: tolerating parse errors would re-open the fail-open hole this function
// exists to close, since a malformed board file would then read as a directory of
// retired boards.
var notBoardFiles = map[string]bool{"telegram.yml": true}

// boardKey identifies a board entry the way the catalogue does. Slug alone is not
// enough in either direction: the same company slug can be listed under one provider
// while its jobs under another are prunable, and jobs.source is what the write path
// records alongside company_slug.
type boardKey struct{ Provider, CompanySlug string }

// boards is what the board files say: which (provider, company) pairs are still
// crawled, and which providers appear at all.
type boards struct {
	// listed gates the company-scoped rules. Those have no ingest counterpart —
	// nothing at crawl time knows a company's bucket — so a deletion under one is
	// undone by the next hourly crawl unless the board is struck from the files in the
	// same step. A pair still listed here cannot be pruned that way.
	listed map[boardKey]bool
	// crawled is the allow-list of providers a crawl re-admits. Deliberately an
	// allow-list rather than a deny-list of {telegram, manual, …}: a deny-list would
	// silently arm any non-crawled source added later, and that deletion is
	// unrecoverable by construction.
	crawled map[string]bool
}

// loadBoards reads every board file under dir.
//
// It fails closed. An unreadable directory, a file that will not parse, or a directory
// holding no board entries at all is an error rather than a short listing — a missing
// entry reads as "this board is retired", so an empty listing would read as "every
// board is retired" and clear the guard on the entire catalogue.
func loadBoards(dir string) (boards, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return boards{}, fmt.Errorf("prune: scan %s: %w", dir, err)
	}
	b := boards{listed: map[boardKey]bool{}, crawled: map[string]bool{}}
	for _, path := range paths {
		if notBoardFiles[filepath.Base(path)] {
			continue
		}
		cfg, err := sources.LoadConfig(path)
		if err != nil {
			return boards{}, err
		}
		for _, e := range cfg.Sources {
			b.crawled[e.Provider] = true
			b.listed[boardKey{Provider: e.Provider, CompanySlug: normalize.Slug(e.Company)}] = true
		}
	}
	if len(b.listed) == 0 {
		if _, statErr := os.Stat(dir); statErr != nil {
			return boards{}, fmt.Errorf("prune: source directory %s: %w", dir, statErr)
		}
		return boards{}, fmt.Errorf("prune: no board entries under %s", dir)
	}
	return b, nil
}

// retired reports whether a company's board is gone from the files, which is the
// precondition for pruning its jobs under a company-scoped rule.
//
// Known hole: on multi-employer sources the board entry's company is not the employer —
// trudvsem lists regions and the adapter takes each posting's own company — so a real
// employer there is never "listed" and reads as retired while its region board is still
// crawled. Retiring aggregator sources is out of scope for this change, so those
// providers must be kept out of the company-scoped rules by other means; see the design.
func (b boards) retired(provider, companySlug string) bool {
	return !b.listed[boardKey{Provider: provider, CompanySlug: companySlug}]
}
