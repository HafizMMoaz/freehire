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
	"math/rand/v2"
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
	flag.Bool("dry-run", false, "no-op: reporting is the default, --apply is what deletes")
	limit := flag.Int("limit", 0, "stop after this many target rows; required with --apply, -1 to run uncapped")
	sampleSize := flag.Int("sample", 200, "how many random matched titles the report prints")
	seed := flag.Uint64("seed", 1, "sampling seed, so a dry run can be reproduced")
	flag.Parse()

	// An uncapped run has to be asked for in as many words. The first live run should
	// be a small fraction of what matches, and a bare --apply (or a typo'd --limit=0)
	// would otherwise remove everything in one unattended pass.
	if *apply && *limit == 0 {
		log.Print("prune: --apply requires --limit (use --limit=-1 to run uncapped, deliberately)")
		return 1
	}
	if *sampleSize < 0 {
		log.Print("prune: --sample must not be negative")
		return 1
	}

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
		if err := reportBoards(ctx, os.Stdout, q, brd); err != nil {
			log.Printf("prune: write report: %v", err)
			return 1
		}
		return 0
	}

	var index docDeleter
	if *apply {
		if cfg.MeiliURL == "" || cfg.MeiliKey == "" {
			log.Print("prune: --apply needs MEILI_URL and MEILI_MASTER_KEY — deleting rows while the index keeps serving them would 404 every result")
			return 1
		}
		index = search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	}

	p, err := scan(ctx, q, ev, brd, *limit, *sampleSize, rand.New(rand.NewPCG(*seed, 0)))
	if err != nil {
		log.Printf("prune: scan: %v", err)
		return 1
	}

	if !*apply {
		if err := p.report(os.Stdout, false); err != nil {
			log.Printf("prune: write report: %v", err)
			return 1
		}
		log.Print("dry run — pass --apply to delete")
		return 0
	}

	// Print the plan before removing anything, so the run's own log records what it
	// was about to do even if it dies partway.
	if err := p.report(os.Stdout, false); err != nil {
		log.Printf("prune: write report: %v", err)
		return 1
	}

	code := 0
	if err := deleteTargets(ctx, q, index, p); err != nil {
		// Batches already committed stay committed, so the outcome has to be printed
		// on this path too — otherwise a failure leaves one error line and no record
		// of what went. pruned_jobs holds the durable version.
		log.Printf("prune: delete: %v (rows already removed are recorded in pruned_jobs)", err)
		code = 1
	}
	if err := p.report(os.Stdout, true); err != nil {
		log.Printf("prune: write report: %v", err)
		return 1
	}
	return code
}

// scan walks the catalogue by keyset and collects what the rule matches, stopping the
// collection (but not the count) at the cap.
func scan(ctx context.Context, q candidateSource, ev []db.CompanyTechEvidenceRow, brd boards, limit, sampleSize int, rnd *rand.Rand) (*plan, error) {
	type companyKey struct{ source, slug string }
	byCompany := make(map[companyKey]evidence, len(ev))
	for _, r := range ev {
		byCompany[companyKey{r.Source, r.CompanySlug}] = evidence{anyTech: r.AnyTech, anySkills: r.AnySkills}
	}

	p := newPlan(sampleSize, rnd)
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
			c := candidate{CompanySlug: row.CompanySlug, Title: row.Title, Category: row.Category}
			if row.IsTech.Valid {
				v := row.IsTech.Bool
				c.IsTech = &v
			}
			known := brd.knownProvider(row.Source)
			rule, ok := matchRule(c, byCompany[companyKey{row.Source, row.CompanySlug}],
				known, brd.crawls(row.Source, row.ExternalID))
			if !ok {
				// Surface what the source gate turned down. Without this the operator
				// sees only what would go, never what the guards held back — and the
				// guards are the reason to trust the number at all.
				if !known && wouldMatchButForTheSource(c) {
					p.refuse("source is not a crawled board platform: " + row.Source)
				}
				continue
			}
			if limit > 0 && len(p.targets) >= limit {
				p.matched++ // counted, not taken: the report must say how much is left
				p.sample("")
				continue
			}
			p.add(row, rule)
		}
	}
}

// deleteTargets removes the planned rows in batches, mirroring each batch into the
// search index by the ids the statement reports actually deleted — the archive and the
// index therefore only ever record real removals.
func deleteTargets(ctx context.Context, q batchDeleter, index docDeleter, p *plan) error {
	for start := 0; start < len(p.targets); start += deleteBatch {
		end := min(start+deleteBatch, len(p.targets))
		batch := p.targets[start:end]

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
		p.deleted += len(deleted)
		// The facet index and the semantic index are separate. Search is served
		// straight from Meilisearch with no Postgres hydration, so a document left in
		// either one keeps appearing in results whose row is gone.
		if err := index.DeleteJobs(ctx, deleted); err != nil {
			return err
		}
		if err := index.DeleteSemanticJobs(ctx, deleted); err != nil {
			return err
		}
	}
	return nil
}

// wouldMatchButForTheSource reports whether a rule would have fired had the posting come
// from a crawled board platform. It exists only so the report can count what the source
// gate held back; it must never decide a deletion.
func wouldMatchButForTheSource(c candidate) bool {
	_, ok := matchRule(c, evidence{}, true, false)
	return ok
}

// The two dependencies the destructive path needs, named so it can be tested without a
// database or a search engine.
type (
	candidateSource interface {
		PruneCandidates(context.Context, db.PruneCandidatesParams) ([]db.PruneCandidatesRow, error)
	}
	batchDeleter interface {
		PruneJobs(context.Context, db.PruneJobsParams) ([]int64, error)
	}
	docDeleter interface {
		DeleteJobs(context.Context, []int64) error
		DeleteSemanticJobs(context.Context, []int64) error
	}
)

// reportBoards lists the boards still in the source files whose postings have never
// shown anything technical — no technical title or category, and not one tagged skill.
// Each is a candidate for the retirement PR, which is the precondition for pruning its
// jobs under a company-scoped rule.
//
// It groups by BOARD rather than by company because that is the identity the source
// files and the catalogue share exactly; the company slug diverges wherever an adapter
// takes the name from the posting payload, which on some providers is most of them.
func reportBoards(ctx context.Context, w io.Writer, q candidateSource, brd boards) error {
	evidence := map[boardKey]bool{}
	var after int64
	for {
		rows, err := q.PruneCandidates(ctx, db.PruneCandidatesParams{AfterID: after, PageSize: scanPage})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			after = row.ID
			board, ok := brd.boardOf(row.Source, row.ExternalID)
			if !ok {
				continue // not from a listed board: nothing to retire
			}
			key := boardKey{Provider: row.Source, Board: board}
			evidence[key] = evidence[key] || (row.IsTech.Valid && row.IsTech.Bool) || row.HasSkills
		}
	}

	var retire []boardKey
	for key, hasEvidence := range evidence {
		if !hasEvidence {
			retire = append(retire, key)
		}
	}
	if len(retire) == 0 {
		_, err := fmt.Fprintln(w, "every listed board has posted something technical")
		return err
	}
	sort.Slice(retire, func(i, j int) bool {
		if retire[i].Provider != retire[j].Provider {
			return retire[i].Provider < retire[j].Provider
		}
		return retire[i].Board < retire[j].Board
	})

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "PROVIDER\tBOARD"); err != nil {
		return err
	}
	for _, k := range retire {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", k.Provider, k.Board); err != nil {
			return err
		}
	}
	return tw.Flush()
}
