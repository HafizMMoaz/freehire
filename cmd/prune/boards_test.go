package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

func writeBoardFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// The company-scoped rules have no ingest counterpart, so a deletion under one is
// undone by the next hourly crawl unless the board is struck from the source files.
// The worker therefore has to know which companies are still listed — matched by the
// same slug normalization ingest writes, or the guard would compare different strings.
func TestListedCompaniesMatchesIngestSlugging(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", `
- company: "Acme Corp."
  board: acme
- company: "Beta & Co"
  board: beta
`)
	writeBoardFile(t, dir, "custom.yml", `
- company: "Gamma"
  provider: lever
  board: gamma
`)
	// Not a board file: must not be read as one.
	writeBoardFile(t, dir, "README.md", "not yaml")

	listed, err := listedCompanies(dir)
	if err != nil {
		t.Fatalf("listedCompanies: %v", err)
	}

	for _, want := range []string{"acme-corp", "beta-co", "gamma"} {
		if !listed[want] {
			t.Errorf("company slug %q not listed; got %v", want, keys(listed))
		}
	}
	if len(listed) != 3 {
		t.Errorf("listed %d companies, want 3 — README.md is not a board file", len(listed))
	}
}

// A slug the source files do not mention is retired, and only those may be deleted
// under a company-scoped rule.
func TestListedCompaniesOmitsUnlisted(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: Acme\n  board: acme\n")

	listed, err := listedCompanies(dir)
	if err != nil {
		t.Fatalf("listedCompanies: %v", err)
	}
	if listed["retired-co"] {
		t.Error("a company absent from every board file must not read as listed")
	}
}

// A source directory that cannot be read must stop the run rather than yield an empty
// set: an empty set reads as "every board is retired", which would let the company
// rules delete the whole catalogue.
func TestListedCompaniesFailsClosed(t *testing.T) {
	if _, err := listedCompanies(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("an unreadable source directory must be an error, not an empty listing — empty means every board is retired")
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The report is what a retirement PR is written from, so it must name only companies
// that are both still listed and genuinely without technical evidence. A false entry
// costs a live board; a missing one leaves jobs the company rules cannot touch.
func TestReportBoardsListsOnlyRetirableCompanies(t *testing.T) {
	listed := map[string]bool{"nurse-co": true, "tech-co": true, "skills-co": true}
	rows := []db.CompanyTechEvidenceRow{
		{Source: "ukg", CompanySlug: "nurse-co"},
		{Source: "greenhouse", CompanySlug: "tech-co", AnyTech: true},
		{Source: "greenhouse", CompanySlug: "skills-co", AnySkills: true},
		{Source: "workday", CompanySlug: "already-retired"}, // no evidence, but not listed
	}

	var b strings.Builder
	if err := reportBoards(&b, rows, listed); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	out := b.String()

	if !strings.Contains(out, "nurse-co") {
		t.Errorf("a listed company with no evidence must be reported:\n%s", out)
	}
	for _, absent := range []string{"tech-co", "skills-co", "already-retired"} {
		if strings.Contains(out, absent) {
			t.Errorf("%q must not be reported:\n%s", absent, out)
		}
	}
}

// An empty report is the state the campaign is working towards, so it has to say so
// rather than print nothing and read as a broken worker.
func TestReportBoardsSaysSoWhenNothingToRetire(t *testing.T) {
	var b strings.Builder
	if err := reportBoards(&b, []db.CompanyTechEvidenceRow{{Source: "greenhouse", CompanySlug: "tech-co", AnyTech: true}},
		map[string]bool{"tech-co": true}); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	if strings.TrimSpace(b.String()) == "" {
		t.Error("an empty result must print an explicit message")
	}
}
