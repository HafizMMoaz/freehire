// Command mine-titles reports the word groups occurring most often in the titles of
// jobs that still carry no is_tech signal. It is the read-only half of the
// catalogue-pruning loop: it names the next cluster worth a dictionary term, and,
// run again after a pruning pass, shows whether the unclassified group shrank.
//
// It is a run-once-and-exit worker and writes nothing. Grouping is by word group
// rather than by whole title because boards append location, schedule and
// requisition detail: measured on prod, half the unclassified mass has a title
// occurring exactly once, so whole-title clustering reached 6.6% of it against
// 15.2% for word groups. A word group is also the unit the non-tech dictionary
// accepts, so a reported cluster can be copied into it as an anchored term.
//
// Two costs to expect. --limit bounds the output, not the work: the whole group-by
// runs before the final top-N sort, so a small limit costs the same as a large one.
// And expanding every title into overlapping groups multiplies the rows the
// aggregate sees, so this is minutes, not seconds, and it spills to temp sort space
// — fine for an operator tool run a few times per pruning iteration, and firmly off
// any request path.
//
// The report is a shortlist for a human, not a verdict. The same measurement found
// roughly a fifth of the top 100 to be technical or IT-relevant phrases that must
// NOT reach the non-tech dictionary ("systems engineer", "team lead"), and another
// quarter to be fragments of one verbose employer's titles. Read it before acting.
//
// Usage: go run ./cmd/mine-titles [--limit=100]   (needs DATABASE_URL)
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	limit := flag.Int("limit", 100, "how many title clusters to report, busiest first")
	flag.Parse()
	if *limit <= 0 {
		log.Printf("--limit must be positive, got %d", *limit)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	rows, err := db.New(pool).ResidualTitleGroups(ctx, db.ResidualTitleGroupsParams{
		StopWords:  stopWords,
		Connectors: connectors,
		RowLimit:   int32(*limit),
	})
	if err != nil {
		log.Printf("mine-titles: %v", err)
		return 1
	}
	if err := report(os.Stdout, rows); err != nil {
		log.Printf("mine-titles: write report: %v", err)
		return 1
	}
	return 0
}

// report writes the clusters as an aligned table in the order given (the query orders
// them busiest-first). Sources are sorted here rather than trusted from the aggregate,
// so an unchanged catalogue renders byte-identically between runs — an operator
// diffing two iterations should see only real movement.
func report(w io.Writer, rows []db.ResidualTitleGroupsRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no unclassified title groups left")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		sources := slices.Clone(r.Sources)
		slices.Sort(sources)
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\n", r.Jobs, r.Grp, strings.Join(sources, ", ")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
