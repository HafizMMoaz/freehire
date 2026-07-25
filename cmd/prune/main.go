// Command prune permanently removes jobs that do not belong on an IT job board, and
// reports the boards whose companies have never posted anything technical.
//
// It is the destructive half of the catalogue-pruning loop. Nothing it deletes can be
// recovered from the database — the archive table records identity and title only — so
// every path defaults to reporting and requires an explicit flag to act.
//
// --boards is the report the company-scoped rules depend on. Those rules have no
// counterpart at crawl time, so a deletion under one is undone by the next hourly crawl
// unless the board is struck from sources/*.yml in the same step. This lists the
// candidates for that PR: boards still listed whose company shows no technical evidence
// across its entire history.
//
// Usage:
//
//	go run ./cmd/prune --boards [--sources=sources]   (needs DATABASE_URL)
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	boards := flag.Bool("boards", false, "report board entries whose company has never posted anything technical")
	sourcesDir := flag.String("sources", "sources", "directory holding the board files")
	flag.Parse()

	if !*boards {
		log.Print("nothing to do: pass --boards for the board-retirement report")
		return 1
	}

	// Read the board files before touching the database: an unreadable source
	// directory must stop the run, not yield an empty listing that reads as "every
	// board is retired".
	brd, err := loadBoards(*sourcesDir)
	if err != nil {
		log.Printf("prune: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	rows, err := db.New(pool).CompanyTechEvidence(ctx)
	if err != nil {
		log.Printf("prune: company evidence: %v", err)
		return 1
	}

	if err := reportBoards(os.Stdout, rows, brd); err != nil {
		log.Printf("prune: write report: %v", err)
		return 1
	}
	return 0
}

// reportBoards lists the companies still present in the board files that have never
// shown technical evidence — no technical title or category, and not even a tagged
// skill — across their whole history. Each is a candidate for removal from
// sources/*.yml, which is the precondition for pruning its jobs under a company-scoped
// rule.
//
// Companies with any evidence are omitted, and so are slugs the board files no longer
// mention: those are already retired and need no PR.
func reportBoards(w io.Writer, rows []db.CompanyTechEvidenceRow, brd boards) error {
	var retire []db.CompanyTechEvidenceRow
	for _, r := range rows {
		if r.AnyTech || r.AnySkills || brd.retired(r.Source, r.CompanySlug) {
			continue
		}
		retire = append(retire, r)
	}
	if len(retire) == 0 {
		_, err := fmt.Fprintln(w, "no listed board has a company without technical evidence")
		return err
	}
	sort.Slice(retire, func(i, j int) bool {
		if retire[i].Source != retire[j].Source {
			return retire[i].Source < retire[j].Source
		}
		return retire[i].CompanySlug < retire[j].CompanySlug
	})

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SOURCE\tCOMPANY"); err != nil {
		return err
	}
	for _, r := range retire {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", r.Source, r.CompanySlug); err != nil {
			return err
		}
	}
	return tw.Flush()
}
