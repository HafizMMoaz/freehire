package viewlog

import "strings"

// botMarkers are lowercased substrings of well-known crawler/preview User-Agents.
// The list is deliberately small and conservative: it catches the high-volume
// crawlers that hit SSR pages for SEO and link previews, not every possible bot.
// Missed bots only inflate the page-view number (a transparency figure), so this
// stays a light filter rather than an exhaustive blocklist.
//
// The generic "bot" is deliberately absent, mirroring internal/tracerlink's own
// classifier: it appears inside "CUBOT," a phone brand, so a bare substring match
// misclassifies a real visitor's page view as a crawler's. Generic bot names
// (googlebot, bingbot, ahrefsbot, ...) are caught by botSuffixes instead.
var botMarkers = []string{
	"crawl", // crawler variants
	"spider",
	"slurp", // Yahoo
	"facebookexternalhit",
	"embedly",
	"prerender",
	"headlesschrome",
}

// botSuffixes catch the general shape of a bot's name — "…bot" followed by a version
// separator, as in "Googlebot/2.1" or "bingbot;" — without claiming every product
// whose name happens to contain those three letters (see botMarkers).
var botSuffixes = []string{"bot/", "bot;", "bot)", "bot ", "bot-"}

// isBot reports whether a User-Agent looks like a known crawler or link-preview
// fetcher. Applied only to page-open signals; API reads are never bot-filtered.
func isBot(ua string) bool {
	lower := strings.ToLower(ua)
	for _, m := range botMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	for _, suffix := range botSuffixes {
		if strings.Contains(lower, suffix) {
			return true
		}
	}
	return strings.HasSuffix(lower, "bot")
}
