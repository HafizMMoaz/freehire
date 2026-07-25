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

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	for _, want := range []boardKey{
		{"greenhouse", "acme-corp"}, {"greenhouse", "beta-co"}, {"lever", "gamma"},
	} {
		if !b.listed[want] {
			t.Errorf("%+v not listed; got %v", want, b.listed)
		}
	}
	if len(b.listed) != 3 {
		t.Errorf("listed %d entries, want 3 — README.md is not a board file", len(b.listed))
	}
	// The same slug under another provider is a different board and stays prunable.
	if !b.retired("workday", "acme-corp") {
		t.Error("a slug listed under greenhouse must not shield the same slug under workday")
	}
}

// A slug the source files do not mention is retired, and only those may be deleted
// under a company-scoped rule.
func TestListedCompaniesOmitsUnlisted(t *testing.T) {
	dir := t.TempDir()
	writeBoardFile(t, dir, "greenhouse.yml", "- company: Acme\n  board: acme\n")

	b, err := loadBoards(dir)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}
	if !b.retired("greenhouse", "retired-co") {
		t.Error("a company absent from every board file must read as retired")
	}
}

// A source directory that cannot be read must stop the run rather than yield an empty
// set: an empty set reads as "every board is retired", which would let the company
// rules delete the whole catalogue.
func TestListedCompaniesFailsClosed(t *testing.T) {
	if _, err := loadBoards(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("an unreadable source directory must be an error, not an empty listing — empty means every board is retired")
	}
}

// The guard runs against the real sources/ directory on every invocation, and that
// directory holds files that are not board lists — telegram.yml is the channel list for
// cmd/tg-ingest and does not parse as one. A fixture-only test cannot see that, and did
// not: --boards errored on every real run until this case existed.
func TestLoadBoardsReadsTheRealSourcesDirectory(t *testing.T) {
	b, err := loadBoards("../../sources")
	if err != nil {
		t.Fatalf("loadBoards on the real directory: %v", err)
	}
	if len(b.listed) < 100 {
		t.Errorf("listed %d entries, want the real catalogue's boards", len(b.listed))
	}
	if !b.crawled["greenhouse"] {
		t.Error("greenhouse must be in the crawled allow-list")
	}
	for _, notCrawled := range []string{"telegram", "manual", ""} {
		if b.crawled[notCrawled] {
			t.Errorf("%q is not a board provider and must not be in the crawled allow-list", notCrawled)
		}
	}
}

// The report is what a retirement PR is written from, so it must name only companies
// that are both still listed and genuinely without technical evidence. A false entry
// costs a live board; a missing one leaves jobs the company rules cannot touch.
func TestReportBoardsListsOnlyRetirableCompanies(t *testing.T) {
	brd := boards{listed: map[boardKey]bool{
		{"ukg", "nurse-co"}: true, {"greenhouse", "tech-co"}: true, {"greenhouse", "skills-co"}: true,
	}}
	rows := []db.CompanyTechEvidenceRow{
		{Source: "ukg", CompanySlug: "nurse-co"},
		{Source: "greenhouse", CompanySlug: "tech-co", AnyTech: true},
		{Source: "greenhouse", CompanySlug: "skills-co", AnySkills: true},
		{Source: "workday", CompanySlug: "already-retired"}, // no evidence, but not listed
	}

	var b strings.Builder
	if err := reportBoards(&b, rows, brd); err != nil {
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
		boards{listed: map[boardKey]bool{{"greenhouse", "tech-co"}: true}}); err != nil {
		t.Fatalf("reportBoards: %v", err)
	}
	if strings.TrimSpace(b.String()) == "" {
		t.Error("an empty result must print an explicit message")
	}
}
