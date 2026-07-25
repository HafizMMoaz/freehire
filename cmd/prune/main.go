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
// Without --apply the worker scans, reports what it would remove, and exits. --apply is
// the only way to delete anything, and --limit caps how much a single run takes: the
// first live run should be a small fraction of what matches, not the whole campaign.
//
// Usage:
//
//	go run ./cmd/prune                       # dry run: what would go, and why
//	go run ./cmd/prune --apply --limit=50000 # remove at most 50k rows
//	go run ./cmd/prune --boards              # board-retirement report
//
// Needs DATABASE_URL; MEILI_URL/MEILI_MASTER_KEY when applying, so the search index
// loses the documents in the same step.
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
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

// scanPage is how many rows one keyset page carries, and deleteBatch how many ids one
// delete statement takes. The scan page is large because the rule is cheap and most
// rows do not match; the delete batch is small because each one is a transaction that
// cascades into user_jobs and its ids are mirrored into the search index.
const (
	scanPage    = 5000
	deleteBatch = 500
)

func run() int {
	boardReport := flag.Bool("boards", false, "report board entries whose company has never posted anything technical")
	sourcesDir := flag.String("sources", "sources", "directory holding the board files")
	apply := flag.Bool("apply", false, "actually delete; without it the run only reports")
	limit := flag.Int("limit", 0, "stop after this many deletions (0 = no cap)")
	flag.Parse()

	// Read the board files before touching the database. They gate the irreversible
	// rules, and an unreadable directory must stop the run before anything is removed
	// rather than yield an empty listing that reads as "every board is retired".
	brd, err := loadBoards(*sourcesDir)
	if err != nil {
		log.Printf("prune: %v", err)
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)

	// Company evidence is computed once, before any deletion, so a single run cannot
	// reclassify a company underneath itself as its own rows disappear.
	ev, err := q.CompanyTechEvidence(ctx)
	if err != nil {
		log.Printf("prune: company evidence: %v", err)
		return 1
	}

	if *boardReport {
		if err := reportBoards(os.Stdout, ev, brd); err != nil {
			log.Printf("prune: write report: %v", err)
			return 1
		}
		return 0
	}

	var index *search.Client
	if *apply {
		if cfg.MeiliURL == "" || cfg.MeiliKey == "" {
			log.Print("prune: --apply needs MEILI_URL and MEILI_MASTER_KEY — deleting rows while the index keeps serving them would 404 every result")
			return 1
		}
		index = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	p, err := scan(ctx, q, ev, brd, *limit)
	if err != nil {
		log.Printf("prune: scan: %v", err)
		return 1
	}

	if *apply {
		if err := deleteTargets(ctx, q, index, p.targets); err != nil {
			log.Printf("prune: delete: %v", err)
			return 1
		}
	}
	if err := p.report(os.Stdout, *apply); err != nil {
		log.Printf("prune: write report: %v", err)
		return 1
	}
	if !*apply {
		log.Print("dry run — pass --apply to delete")
	}
	return 0
}

// scan walks the catalogue by keyset and collects what the rule matches, stopping the
// collection (but not the count) at the cap.
func scan(ctx context.Context, q *db.Queries, ev []db.CompanyTechEvidenceRow, brd boards, limit int) (*plan, error) {
	byCompany := make(map[boardKey]evidence, len(ev))
	for _, r := range ev {
		byCompany[boardKey{Provider: r.Source, CompanySlug: r.CompanySlug}] = evidence{anyTech: r.AnyTech, anySkills: r.AnySkills}
	}
	perCompany := perCompanyProviders(ev, brd)

	p := newPlan()
	var after int64
	for {
		rows, err := q.PruneCandidates(ctx, db.PruneCandidatesParams{AfterID: after, PageSize: scanPage})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return p, nil
		}
		for _, row := range rows {
			after = row.ID
			key := boardKey{Provider: row.Source, CompanySlug: row.CompanySlug}
			c := candidate{Source: row.Source, CompanySlug: row.CompanySlug, Title: row.Title, Category: row.Category}
			if row.IsTech.Valid {
				v := row.IsTech.Bool
				c.IsTech = &v
			}
			rule, ok := matchRule(c, byCompany[key], brd.crawled)
			if !ok {
				continue
			}
			// The company-scoped rules have no ingest counterpart, so what they
			// remove comes back on the next crawl unless the board is gone from the
			// files — and on a multi-employer source the guard cannot even tell,
			// because no employer is ever listed there.
			if companyScoped(rule) {
				if !perCompany[row.Source] {
					p.refuse("multi-employer source: " + row.Source)
					continue
				}
				if !brd.retired(row.Source, row.CompanySlug) {
					p.refuse("board still listed: " + row.Source + "/" + row.CompanySlug)
					continue
				}
			}
			if limit > 0 && len(p.targets) >= limit {
				p.matched++ // counted, not taken: the report must say how much is left
				continue
			}
			p.add(row, rule)
		}
	}
}

// deleteTargets removes the planned rows in batches, mirroring each batch into the
// search index by the ids the statement reports actually deleted — the archive and the
// index therefore only ever record real removals.
func deleteTargets(ctx context.Context, q *db.Queries, index *search.Client, targets []target) error {
	for start := 0; start < len(targets); start += deleteBatch {
		end := min(start+deleteBatch, len(targets))
		batch := targets[start:end]

		ids := make([]int64, len(batch))
		rules := make([]string, len(batch))
		for i, t := range batch {
			ids[i], rules[i] = t.id, t.rule
		}
		// PruneJobs pairs the arrays on ordinality, so a length mismatch would
		// silently under-delete rather than fail.
		if len(ids) != len(rules) {
			return fmt.Errorf("prune: %d ids against %d rules", len(ids), len(rules))
		}

		deleted, err := q.PruneJobs(ctx, db.PruneJobsParams{Ids: ids, Rules: rules})
		if err != nil {
			return err
		}
		if err := index.DeleteJobs(ctx, deleted); err != nil {
			return err
		}
	}
	return nil
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
