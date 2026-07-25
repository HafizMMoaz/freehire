package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// How many sources a group spans separates a real catalogue-wide role from one
// employer's verbose titles shredded into fragments: on prod every genuine role sat
// on dozens of sources while every fragment sat on one or two. The count leads the
// source column so that read is immediate, and the list is truncated because some
// groups span ninety sources and printing them all makes the table unreadable.
func TestReportLeadsWithSourceBreadthAndTruncates(t *testing.T) {
	var b strings.Builder
	rows := []db.ResidualTitleGroupsRow{
		{Grp: "behavior technician", Jobs: 25939, Sources: []string{
			"ukg", "workday", "icims", "paycom", "lever", "greenhouse"}},
		{Grp: "afternoon registered", Jobs: 2957, Sources: []string{"apploi"}},
	}

	if err := report(&b, rows); err != nil {
		t.Fatalf("report: %v", err)
	}
	out := b.String()

	if !strings.Contains(out, "6") || !strings.Contains(out, "greenhouse, icims, lever") {
		t.Errorf("wide group must show its source count and the first few sources:\n%s", out)
	}
	if strings.Contains(out, "workday") {
		t.Errorf("the source list must be truncated, not printed whole:\n%s", out)
	}
	if !strings.Contains(out, "+3") {
		t.Errorf("truncation must say how many sources were elided:\n%s", out)
	}
	if !strings.Contains(out, "apploi") || strings.Contains(out, "apploi, ") {
		t.Errorf("a single-source group must render its one source plainly:\n%s", out)
	}
}

// The query orders busiest-first; report must render the rows in the order it was
// given rather than imposing one of its own.
func TestReportRendersFieldsInGivenOrder(t *testing.T) {
	var b strings.Builder
	rows := []db.ResidualTitleGroupsRow{
		{Grp: "registered behavior technician", Jobs: 3650, Sources: []string{"ukg", "workday"}},
		{Grp: "line cook", Jobs: 1600, Sources: []string{"greenhouse"}},
	}

	if err := report(&b, rows); err != nil {
		t.Fatalf("report: %v", err)
	}
	out := b.String()

	for _, want := range []string{"3650", "registered behavior technician", "ukg, workday", "1600", "line cook", "greenhouse"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if i, j := strings.Index(out, "registered behavior"), strings.Index(out, "line cook"); i > j {
		t.Errorf("rows must render in the given order, got:\n%s", out)
	}
}

// Postgres happens to return array_agg(DISTINCT ...) sorted, because it implements
// the DISTINCT by sorting — but that is an implementation detail of the aggregate,
// not a documented guarantee. The report sorts on its own so its output stays
// byte-identical between runs over unchanged data regardless, and so the renderer is
// testable without a database.
func TestReportSortsSources(t *testing.T) {
	var b strings.Builder
	rows := []db.ResidualTitleGroupsRow{
		{Grp: "caregiver", Jobs: 10, Sources: []string{"workday", "greenhouse", "ukg"}},
	}

	if err := report(&b, rows); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(b.String(), "greenhouse, ukg, workday") {
		t.Errorf("sources must be sorted, got:\n%s", b.String())
	}
}

// An empty report is the campaign's success condition, so it must say so rather than
// print nothing and read as a broken worker.
func TestReportOnEmptyResultSaysSo(t *testing.T) {
	var b strings.Builder
	if err := report(&b, nil); err != nil {
		t.Fatalf("report: %v", err)
	}
	if strings.TrimSpace(b.String()) == "" {
		t.Error("empty result must print an explicit message, not nothing")
	}
}
