package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

func TestApplySeedContentPreservesPresentation(t *testing.T) {
	keep := cvedit.State{
		Title:      "Tailored for Acme",
		TemplateID: "compact",
		Document: cv.Document{
			Margins: cv.Margins{Top: 0.75, Right: 0.6, Bottom: 0.75, Left: 0.6},
			Style:   cv.Style{FontFamily: "tinos", FontSize: 11, LineHeight: 0.55},
			Header:  cv.Header{FullName: "Old Name"},
			Summary: "old summary",
		},
	}
	seeded := cv.Document{
		Header:  cv.Header{FullName: "Ada Lovelace", Email: "ada@example.com"},
		Summary: "new summary from résumé",
		Skills:  []cv.SkillGroup{{Items: []string{"Go"}}},
	}

	got := applySeedContent(keep, seeded)

	if got.Title != "Tailored for Acme" || got.TemplateID != "compact" {
		t.Fatalf("title/template = %q/%q, want preserved", got.Title, got.TemplateID)
	}
	if got.Margins != keep.Margins || got.Style != keep.Style {
		t.Fatalf("presentation not preserved: margins=%+v style=%+v", got.Margins, got.Style)
	}
	if got.Header.FullName != "Ada Lovelace" || got.Summary != "new summary from résumé" {
		t.Fatalf("content not applied: header=%+v summary=%q", got.Header, got.Summary)
	}
	if len(got.Skills) != 1 || len(got.Skills[0].Items) != 1 || got.Skills[0].Items[0] != "Go" {
		t.Fatalf("skills = %+v, want seeded Go", got.Skills)
	}
}

func TestApplySeedContentEqualYieldsNoDiff(t *testing.T) {
	seeded := cv.Document{
		Header:  cv.Header{FullName: "Ada"},
		Summary: "same",
	}
	keep := cvedit.State{
		Title:      "My CV",
		TemplateID: cv.DefaultTemplateID,
		Document: cv.Document{
			Margins: cv.DefaultMargins(),
			Header:  seeded.Header,
			Summary: seeded.Summary,
		},
	}
	next := applySeedContent(keep, seeded)
	// Sanitizer runs inside CommitDocument; Diff here checks the structural equality the
	// helper is responsible for before that step.
	if ops := cvedit.Diff(keep, next); len(ops) != 0 {
		t.Fatalf("Diff = %+v, want empty when only seed content already matches", ops)
	}
}
