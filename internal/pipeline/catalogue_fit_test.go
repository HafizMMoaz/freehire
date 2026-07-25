package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/sources"
)

// A crawled board pours a company's whole hiring into the catalogue, so the postings
// the non-tech dictionary recognises are rejected before the write path. Rejection is
// safe here precisely because the board is re-crawled: if a dictionary term turns out
// to be too broad, removing it re-admits the postings on the next pass.
func TestRunRejectsNonTechPostings(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().Title != "Backend Engineer" {
		t.Fatalf("saved = %+v, want only the technical posting", store.saved)
	}
	if got := stats.Total(); got.Ingested != 1 || got.Rejected != 1 {
		t.Errorf("stats = %+v, want Ingested=1 Rejected=1", got)
	}
}

// The ingest filter reads the non-tech TITLE dictionary, not the tri-state is_tech.
// A business role at an IT company resolves is_tech=false through its category, and
// whether it belongs in the catalogue depends on the company — a judgement the crawl
// cannot make and the prune worker makes later. Rejecting it here would quietly
// delete every sales and recruiting job on the board.
func TestRunKeepsBusinessRolesTheCompanyRuleOwns(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Sales Manager", Company: "Acme"},
		{ExternalID: "2", Title: "Technical Recruiter", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("len(saved) = %d, want 2 — a non-technical category is not a title-dictionary match", len(store.saved))
	}
	if got := stats.Total(); got.Rejected != 0 {
		t.Errorf("stats = %+v, want Rejected=0", got)
	}
}

// A streaming board goes through a separate save loop, so the filter has to be wired
// twice; a rejection reaching only one path would leak on half the catalogue.
func TestRunRejectsNonTechInStreamPath(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Line Cook", Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "jobtech", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().Title != "Backend Engineer" {
		t.Fatalf("saved = %+v, want only the technical posting", store.saved)
	}
	if got := stats.Total(); got.Ingested != 1 || got.Rejected != 1 {
		t.Errorf("stats = %+v, want Ingested=1 Rejected=1", got)
	}
}

// Rejections and skips mean opposite things to an operator: a rejection is the filter
// working, a skip is something broken. Folding them together would make a board whose
// every save fails look like a board full of non-technical postings.
func TestRunCountsRejectionsSeparatelyFromSkips(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{err: errors.New("write failed")}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := stats.Total()
	if got.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1 (the nurse posting)", got.Rejected)
	}
	if got.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (the engineer posting failed to save)", got.Skipped)
	}
}

// The filter belongs to the crawled pipeline, not to the shared aggregate factory that
// Telegram extraction, user submissions and link-source imports also write through.
// Those paths are never re-crawled, so a filter mistake there could not be undone —
// job.New must keep constructing a non-technical posting without complaint.
func TestSharedFactoryDoesNotFilterNonTech(t *testing.T) {
	j, err := job.New(job.Draft{
		Source:     "telegram",
		ExternalID: "chan:1",
		Title:      "Registered Nurse",
		Company:    "Acme",
		URL:        "https://example.test/1",
	})
	if err != nil {
		t.Fatalf("job.New must not reject a non-technical posting: %v", err)
	}
	if j.Fields().Title != "Registered Nurse" {
		t.Errorf("title = %q, want it preserved", j.Fields().Title)
	}
}
