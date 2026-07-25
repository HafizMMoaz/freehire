//go:build integration

// Integration tests for the catalogue-pruning queries. ResidualUnclassifiedTitles reports
// the most frequent titles that still carry no is_tech signal, so each pruning iteration can
// be aimed at the next real cluster and the remaining group measured. SQL behavior,
// verifiable only against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// residualJob upserts a job with the given external id, title and source, carrying the
// tri-state is_tech (nil = unclassified) that the miner selects on.
func residualJob(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext, title, source string, isTech *bool) Job {
	t.Helper()
	p := ingestParams(ext, title)
	p.Source = source
	if isTech != nil {
		p.IsTech = pgtype.Bool{Bool: *isTech, Valid: true}
	}
	j, err := ingestUpsert(ctx, New(pool), p)
	if err != nil {
		t.Fatalf("upsert %s: %v", ext, err)
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

	// A second, smaller cluster whose live member is the ONLY row that may be reported.
	// Every excluded row below shares its title and carries source "ukg", so an exclusion
	// that leaked would inflate this cluster's count or add "ukg" to its sources — a
	// filter moved into a HAVING, or dropped from the aggregate, fails here rather than
	// passing because each excluded row happened to have a title of its own.
	live := residualJob(ctx, t, pool, "acme:4", "Line Cook", "greenhouse", nil)
	residualJob(ctx, t, pool, "acme:5", "Line Cook", "ukg", &no) // classified non-tech
	closed := residualJob(ctx, t, pool, "acme:6", "Line Cook", "ukg", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closed.ID); err != nil {
		t.Fatalf("close acme:6: %v", err)
	}
	dup := residualJob(ctx, t, pool, "acme:7", "Line Cook", "ukg", nil)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of = $1 WHERE id = $2", live.ID, dup.ID); err != nil {
		t.Fatalf("mark acme:7 duplicate: %v", err)
	}

	// Classified technical — the same predicate as acme:5, kept with a realistic title.
	residualJob(ctx, t, pool, "acme:8", "Senior Software Engineer", "ukg", &yes)

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

	cook := rows[1]
	if cook.Title != "line cook" {
		t.Fatalf("second title = %q, want %q", cook.Title, "line cook")
	}
	if cook.Jobs != 1 {
		t.Errorf("line cook jobs = %d, want 1 — the classified, closed and duplicate rows must not be counted", cook.Jobs)
	}
	if !slices.Equal(cook.Sources, []string{"greenhouse"}) {
		t.Errorf("line cook sources = %v, want [greenhouse] — every excluded row is on ukg, so its presence means one leaked", cook.Sources)
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
