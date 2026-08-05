// Command search-drain is the standalone incremental facet-search worker. It drains
// the search_outbox queue: each wave of jobs cmd/ingest queued (new or content-
// changed, since its last drain) is pushed into the live Meilisearch `jobs` index as
// ONE batch, then the wave's outbox entries are deleted. Run it on a schedule (e.g.
// cron); it drains what is queued and exits. It is the incremental sibling of
// cmd/embed; the full `reindex` swap-rebuild stays the reconciler. It exits non-zero
// when the run had any failures or dead-letters, so cron can alert.
//
// This replaces cmd/ingest's previous inline push (search.Client.SubmitJobs called
// once per ingest run): with ~169 independent per-board ingest processes, that meant
// one Meili task per board per crawl, each forcing a full index re-merge (observed at
// 50-90s regardless of batch size on the production catalogue) — the dominant cause
// of a prod disk-IO saturation that produced nginx 504s. Routing every write through
// one outbox drained on its own schedule collapses however many boards changed in a
// window into one Meili task.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/searchdrain"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Bootstrap owns config + pool, so this required-config check lands just after the
	// pool opens (mirrors cmd/embed). Without a Meili key the worker has nothing to
	// index into.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	dcfg := config.LoadSearchDrain()
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	runner := searchdrain.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client, q: db.New(pool)},
	}

	stats, err := runner.Run(ctx, searchdrain.RunOptions{
		BatchSize:    dcfg.BatchSize,
		LeaseSeconds: dcfg.LeaseSeconds,
		MaxAttempts:  dcfg.MaxAttempts,
		CallTimeout:  dcfg.CallTimeout,
	})
	if err != nil {
		log.Printf("search-drain: %v", err)
		return 1
	}

	log.Printf("search-drain done: indexed=%d failed=%d dead_lettered=%d",
		stats.Indexed, stats.Failed, stats.DeadLettered)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
