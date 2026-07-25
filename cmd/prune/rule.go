package main

import (
	"slices"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobderive"
)

// The three rules that make a job a deletion target. The name is recorded on every
// archive row, so a rule that turns out to be too broad can be audited — and undone in
// judgement, if not in data — on its own.
const (
	// ruleTitle: the non-tech title dictionary recognises the posting. Enforced at
	// ingest too, which is what makes it self-sufficient: the same term that deletes
	// the row also stops the next crawl from re-admitting it.
	ruleTitle = "title"
	// ruleBusiness: a business role — sales, recruiting, finance — at a company that
	// has never posted anything technical. The role itself is in scope at an IT
	// company; the company is what disqualifies it.
	ruleBusiness = "business_at_nontech_company"
	// ruleUnknown: a job no dictionary could place, at a company that has shown no
	// technical signal of any kind, not even a tagged skill.
	ruleUnknown = "unknown_at_empty_company"
)

// candidate is the part of a job the rule reads. Everything here is a stored column;
// the rule derives its own signals from them rather than trusting a stored is_tech,
// because the campaign edits the dictionaries between runs and a stale label would
// decide a permanent deletion.
type candidate struct {
	Source      string
	CompanySlug string
	Title       string
	Category    string
	// IsTech is the stored tri-state, used only to tell "no dictionary placed this"
	// (nil) from "a dictionary placed it as non-technical" (false). The positive case
	// is re-derived, never read from here.
	IsTech *bool
}

// evidence is what a company has ever shown, across its entire history.
type evidence struct {
	anyTech   bool
	anySkills bool
}

// matchRule reports which rule makes a job a deletion target, if any. The empty result
// means keep.
//
// Order matters. A job a crawl cannot re-admit is never touched at all: deleting it is
// unrecoverable, whereas a mistake on a crawled board is undone by removing the
// dictionary term. Technical evidence then vetoes everything, for the same reason it
// vetoes the ingest filter — the non-tech dictionary matches anywhere in a title and
// was written on the assumption that the tech check runs first, so "Backend Engineer —
// Teller Systems" must survive its accidental match on "teller".
//
// Only then do the three rules apply, and only the first two are ever enabled together
// with a board retirement; see companyScoped.
func matchRule(c candidate, ev evidence, crawledSources map[string]bool) (string, bool) {
	if !crawledSources[c.Source] {
		return "", false
	}
	if classify.ConfirmedNonTech(c.Title, jobderive.TechEvidence(c.Category, c.Title)) {
		return ruleTitle, true
	}
	// The company-scoped rules rest on a company's history, so a posting with no
	// company has none to rest on. Without this they would all pool under the empty
	// slug and be decided together.
	if c.CompanySlug == "" {
		return "", false
	}
	if jobderive.TechEvidence(c.Category, c.Title) {
		return "", false
	}
	if !ev.anyTech && slices.Contains(enrich.NonTechCategories, c.Category) {
		return ruleBusiness, true
	}
	if !ev.anyTech && !ev.anySkills && c.IsTech == nil {
		return ruleUnknown, true
	}
	return "", false
}

// companyScoped reports whether a rule depends on the company's history rather than on
// the posting alone. It matters because boards are re-crawled hourly on an unchanged
// dedup key: the title rule is mirrored by the ingest filter, so what it deletes stays
// deleted, but nothing at crawl time knows a company's bucket. A company-scoped
// deletion whose board is still listed in sources/*.yml undoes itself within the hour,
// which is why the worker refuses to run one without retiring the board in the same
// step.
func companyScoped(rule string) bool {
	return rule == ruleBusiness || rule == ruleUnknown
}
