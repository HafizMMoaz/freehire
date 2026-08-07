package handler

import (
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

// applySeedContent builds the next editable state from a résumé seed while keeping the
// candidate's presentation choices. Title and template stay on the row; margins and style
// stay on the document. Everything else comes from cv.Seed (header, summary, experience, …).
//
// Shared by Reset from résumé for both the base CV and the tailored copy so the two cannot
// drift on what "preserve presentation" means.
func applySeedContent(keep cvedit.State, seeded cv.Document) cvedit.State {
	doc := seeded
	doc.Margins = keep.Margins
	doc.Style = keep.Style
	return cvedit.State{
		Title:      keep.Title,
		TemplateID: keep.TemplateID,
		Document:   doc,
	}
}
