//go:build integration

// Integration tests for the company half of the ingest write: UpsertJob upserts the posting's
// company in the same statement, and must leave that row alone when its display name has not
// changed. Measured on prod 2026-08-02 the unguarded version ran 286M updates against 305k
// company rows — 937 per row, because a board with 5,000 postings updated its company 5,000
// times per crawl — leaving the table 32% dead tuples at 26 KB per live row.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func companyStamp(t *testing.T, pool *pgxpool.Pool, slug string) (string, time.Time) {
	t.Helper()
	var name string
	var updated time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT name, updated_at FROM companies WHERE slug = $1`, slug).Scan(&name, &updated); err != nil {
		t.Fatalf("read company %q: %v", slug, err)
	}
	return name, updated
}

// changedPosting is a re-crawl whose content DID move. These tests call UpsertJob directly, so
// the varying hash is not needed to reach it — it mirrors what the ingest path now sends here,
// since an unchanged posting takes the cheap write and never arrives.
func changedPosting(externalID, title, company, hash string) UpsertJobParams {
	p := ingestParams(externalID, title)
	p.Company = company
	p.ContentHash = pgtype.Text{String: hash, Valid: true}
	return p
}

func TestUpsertJobWritesTheCompanyOnlyWhenItsNameChanges(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	if _, err := ingestUpsert(ctx, q, changedPosting("acme:1", "Engineer", "Acme", "h1")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE companies SET updated_at = now() - interval '10 days' WHERE slug = 'acme'`); err != nil {
		t.Fatalf("back-date the company stamp: %v", err)
	}
	_, before := companyStamp(t, pool, "acme")

	t.Run("same name leaves the row alone", func(t *testing.T) {
		// Content moved (a new hash), so the full upsert runs — but the company did not move.
		if _, err := ingestUpsert(ctx, q, changedPosting("acme:1", "Senior Engineer", "Acme", "h2")); err != nil {
			t.Fatalf("re-crawl: %v", err)
		}
		if _, got := companyStamp(t, pool, "acme"); !got.Equal(before) {
			t.Errorf("companies.updated_at = %v, want unchanged %v — a crawl that renamed nothing "+
				"must not rewrite the company row once per posting", got, before)
		}
	})

	t.Run("a rename still writes", func(t *testing.T) {
		if _, err := ingestUpsert(ctx, q, changedPosting("acme:1", "Senior Engineer", "Acme Corp", "h3")); err != nil {
			t.Fatalf("rename crawl: %v", err)
		}
		name, got := companyStamp(t, pool, "acme")
		if name != "Acme Corp" {
			t.Errorf("companies.name = %q, want %q — a real rename must propagate", name, "Acme Corp")
		}
		if !got.After(before) {
			t.Errorf("companies.updated_at = %v, want stamped past %v on a rename", got, before)
		}
	})

	// A second company under the same crawl still gets created: the guard is on the UPDATE
	// branch of the conflict, never on the insert.
	t.Run("a new company is still created", func(t *testing.T) {
		p := changedPosting("globex:1", "Engineer", "Globex", "h4")
		p.CompanySlug = "globex"
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("new company: %v", err)
		}
		if name, _ := companyStamp(t, pool, "globex"); name != "Globex" {
			t.Errorf("companies.name = %q, want Globex", name)
		}
	})
}
