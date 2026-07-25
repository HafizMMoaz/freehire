//go:build integration

// Integration test for the residual-title miner query: ResidualUnclassifiedTitles reports
// the most frequent titles that still carry no is_tech signal, so each pruning iteration can
// be aimed at the next real cluster and the remaining group measured. A SQL behavior,
// verifiable only against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// residualJob upserts a job with the given external id, title and source, then stamps the
// tri-state is_tech directly: UpsertJob does not carry the column, and the miner's whole
// purpose is to select on it.
func residualJob(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext, title, source string, isTech *bool) Job {
	t.Helper()
	p := ingestParams(ext, title)
	p.Source = source
	j, err := ingestUpsert(ctx, New(pool), p)
	if err != nil {
		t.Fatalf("upsert %s: %v", ext, err)
	}
	if isTech != nil {
		if _, err := pool.Exec(ctx, "UPDATE jobs SET is_tech = $1 WHERE id = $2", *isTech, j.ID); err != nil {
			t.Fatalf("set is_tech on %s: %v", ext, err)
		}
	}
	return j
}

func TestResidualUnclassifiedTitlesGroupsAndFilters(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	yes, no := true, false

	// Three unclassified postings of one role, spelled inconsistently and spread over two
	// providers — the miner must see one cluster of three, not three clusters of one.
	residualJob(ctx, t, pool, "acme:1", "Registered Behavior Technician", "greenhouse", nil)
	residualJob(ctx, t, pool, "acme:2", "registered behavior technician", "greenhouse", nil)
	residualJob(ctx, t, pool, "acme:3", "  Registered Behavior Technician  ", "ukg", nil)

	// A smaller unclassified cluster, to prove ordering is by count.
	residualJob(ctx, t, pool, "acme:4", "Line Cook", "greenhouse", nil)

	// Already classified either way — not residual, so out of the report.
	residualJob(ctx, t, pool, "acme:5", "Senior Software Engineer", "greenhouse", &yes)
	residualJob(ctx, t, pool, "acme:6", "Registered Nurse", "greenhouse", &no)

	// Unclassified but not live: a closed posting and a duplicate of a canonical row.
	closed := residualJob(ctx, t, pool, "acme:7", "Team Member", "greenhouse", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closed.ID); err != nil {
		t.Fatalf("close acme:7: %v", err)
	}
	dup := residualJob(ctx, t, pool, "acme:8", "Line Cook", "greenhouse", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of = $1 WHERE id = $2", closed.ID, dup.ID); err != nil {
		t.Fatalf("mark acme:8 duplicate: %v", err)
	}

	rows, err := q.ResidualUnclassifiedTitles(ctx, 10)
	if err != nil {
		t.Fatalf("ResidualUnclassifiedTitles: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("rows = %d (%+v), want 2 — classified, closed and duplicate rows must be excluded", len(rows), rows)
	}

	top := rows[0]
	if top.Title != "registered behavior technician" {
		t.Errorf("top title = %q, want %q (normalized: lowercased and trimmed)", top.Title, "registered behavior technician")
	}
	if top.Jobs != 3 {
		t.Errorf("top jobs = %d, want 3 — case and surrounding whitespace must collapse into one cluster", top.Jobs)
	}
	sources := slices.Clone(top.Sources)
	slices.Sort(sources)
	if !slices.Equal(sources, []string{"greenhouse", "ukg"}) {
		t.Errorf("top sources = %v, want [greenhouse ukg] distinct", sources)
	}

	if rows[1].Title != "line cook" || rows[1].Jobs != 1 {
		t.Errorf("second row = %q/%d, want \"line cook\"/1 — the duplicate must not be counted", rows[1].Title, rows[1].Jobs)
	}
}

func TestResidualUnclassifiedTitlesHonoursLimit(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Three distinct residual clusters of descending size: 3, 2, 1.
	for _, ext := range []string{"a:1", "a:2", "a:3"} {
		residualJob(ctx, t, pool, ext, "Caregiver", "greenhouse", nil)
	}
	for _, ext := range []string{"b:1", "b:2"} {
		residualJob(ctx, t, pool, ext, "Housekeeper", "greenhouse", nil)
	}
	residualJob(ctx, t, pool, "c:1", "Dishwasher", "greenhouse", nil)

	rows, err := q.ResidualUnclassifiedTitles(ctx, 2)
	if err != nil {
		t.Fatalf("ResidualUnclassifiedTitles: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 — the limit caps the report", len(rows))
	}
	if rows[0].Title != "caregiver" || rows[1].Title != "housekeeper" {
		t.Errorf("rows = %q, %q; want caregiver then housekeeper (busiest first)", rows[0].Title, rows[1].Title)
	}
}
