package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// The retirement guard is blind on multi-employer sources: their board entries name
// regions, not employers, so every employer reads as retired while the board keeps
// crawling. Detecting them by measurement rather than a curated list is what keeps the
// gate from going stale as sources are added.
func TestPerCompanyProvidersSeparatesAggregators(t *testing.T) {
	brd := boards{listed: map[boardKey]bool{
		{"greenhouse", "acme"}:           true,
		{"greenhouse", "beta"}:           true,
		{"trudvsem", "krasnodar-region"}: true,
	}}
	rows := []db.CompanyTechEvidenceRow{
		// greenhouse: both of its companies are board entries
		{Source: "greenhouse", CompanySlug: "acme"},
		{Source: "greenhouse", CompanySlug: "beta"},
		// trudvsem: the catalogue holds employers, the board file holds a region
		{Source: "trudvsem", CompanySlug: "some-hospital"},
		{Source: "trudvsem", CompanySlug: "city-school"},
		{Source: "trudvsem", CompanySlug: "bakery-no-3"},
	}

	got := perCompanyProviders(rows, brd)

	if !got["greenhouse"] {
		t.Error("greenhouse names its employers and must be prunable by company")
	}
	if got["trudvsem"] {
		t.Error("trudvsem lists regions — its employers are never listed, so the guard cannot see them and the company rules must not apply")
	}
}

// A batch dominated by one board is the signature of a broken board title rather than a
// real cluster, and it is only visible next to the other sources. The report is the
// only thing standing between that and an irreversible run.
func TestPlanReportShowsBreakdownAndSample(t *testing.T) {
	p := newPlan()
	p.add(db.PruneCandidatesRow{ID: 1, Title: "Line Cook", Source: "greenhouse"}, ruleTitle)
	p.add(db.PruneCandidatesRow{ID: 2, Title: "Registered Nurse", Source: "ukg"}, ruleTitle)
	p.add(db.PruneCandidatesRow{ID: 3, Title: "Account Manager", Source: "ukg"}, ruleBusiness)
	p.refuse("board still listed")

	var b strings.Builder
	if err := p.report(&b, false); err != nil {
		t.Fatalf("report: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"would delete 3 of 3", "BY RULE", ruleTitle, ruleBusiness,
		"BY SOURCE", "greenhouse", "ukg", "REFUSED", "board still listed",
		"SAMPLE", "Line Cook",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

// The cap bounds what a run removes, but the operator still has to see how much was
// matched — otherwise a capped run reads as a finished one.
func TestPlanReportDistinguishesMatchedFromDeleted(t *testing.T) {
	p := newPlan()
	for i := range 5 {
		p.add(db.PruneCandidatesRow{ID: int64(i), Title: "Line Cook", Source: "greenhouse"}, ruleTitle)
	}
	p.targets = p.targets[:2] // the cap trimmed the run

	var b strings.Builder
	if err := p.report(&b, true); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(b.String(), "deleted 2 of 5") {
		t.Errorf("a capped run must report both numbers:\n%s", b.String())
	}
}

// An empty plan must still render — a run that found nothing is the campaign's goal,
// not a failure to report.
func TestPlanReportOnEmptyPlan(t *testing.T) {
	var b strings.Builder
	if err := newPlan().report(&b, false); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(b.String(), "would delete 0 of 0") {
		t.Errorf("empty plan must say so:\n%s", b.String())
	}
}
