package telegram

import (
	"github.com/strelov1/freehire/internal/sources"
)

// TextToHTML converts an extracted plain-text description into minimal safe HTML:
// blank-line-separated chunks become paragraphs, single newlines become <br>, a run of
// bullet-marker lines (•, -, *, –, —, ...) becomes a <ul>, and everything is
// entity-escaped so post content can never inject markup. Stored descriptions are HTML
// across all sources (the SPA renders them directly), so telegram jobs must match that
// contract.
//
// Delegates to sources.PlainTextToHTML: the extraction prompt explicitly asks the model
// to preserve "bullet points, numbered lists, and paragraphs on separate lines," so a
// Telegram-extracted description carries the exact plain-text-with-bullets shape that
// helper already reconstructs correctly for the ATS adapters that hand it raw text.
func TextToHTML(text string) string {
	return sources.PlainTextToHTML(text)
}
