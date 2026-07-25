// Command mine-titles reports the job titles that still carry no is_tech signal,
// grouped into clusters an operator can act on. It is the read-only half of the
// catalogue-pruning loop: it names the next cluster worth a dictionary term, and,
// run again after a pruning pass, shows whether the unclassified group shrank.
//
// It is a run-once-and-exit worker and writes nothing. Titles are normalized
// (lowercased, trimmed) by the query, so one role spelled inconsistently across
// boards reads as one cluster; closed and duplicate rows are excluded, since only a
// live, canonical posting is worth a term.
//
// Cost, measured on prod (1.8M unclassified rows, 1.06M distinct titles): ~67s per
// run. --limit bounds the output, not the work — the whole group-by runs before the
// final top-N sort, so a small limit costs the same as a large one. The per-worker
// sorts spill to temp space (~100MB across three workers); that is released at the
// end, but it is worth knowing on a disk-constrained box. Acceptable for an operator
// tool run a few times per pruning iteration; it would not belong on a request path.
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

	rows, err := db.New(pool).ResidualUnclassifiedTitles(ctx, int32(*limit))
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
func report(w io.Writer, rows []db.ResidualUnclassifiedTitlesRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no unclassified titles left")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		sources := slices.Clone(r.Sources)
		slices.Sort(sources)
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\n", r.Jobs, r.Title, strings.Join(sources, ", ")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
