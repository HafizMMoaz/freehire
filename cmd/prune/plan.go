package main

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/strelov1/freehire/internal/db"
)

// perCompanyListedShare is the fraction of a provider's companies that must appear in
// its board entries for the provider to count as per-company. Real per-company
// providers sit near 1 (every board entry is an employer); multi-employer sources sit
// near 0 (trudvsem's entries are regions, so no employer is ever listed). Any threshold
// in between separates them, and half keeps the margin wide.
const perCompanyListedShare = 0.5

// perCompanyProviders reports which providers name employers in their board entries.
//
// It exists because the retirement guard is blind on the others. The guard allows a
// company-scoped deletion once a company is absent from the board files — but on a
// multi-employer source no employer is ever present, so every one of them reads as
// retired while the region board keeps crawling and re-admits everything within the
// hour. Deleting there is pure loss: the rows go, the archive fills, and the catalogue
// is unchanged an hour later.
//
// Measured rather than curated, so it cannot go stale as sources are added.
func perCompanyProviders(rows []db.CompanyTechEvidenceRow, brd boards) map[string]bool {
	total, listed := map[string]int{}, map[string]int{}
	for _, r := range rows {
		total[r.Source]++
		if !brd.retired(r.Source, r.CompanySlug) {
			listed[r.Source]++
		}
	}
	out := map[string]bool{}
	for provider, n := range total {
		if float64(listed[provider])/float64(n) >= perCompanyListedShare {
			out[provider] = true
		}
	}
	return out
}

// target is one row the rule matched, and the rule that matched it.
type target struct {
	id   int64
	rule string
}

// plan is what a scan found: the rows to delete, why, and everything an operator needs
// to decide whether the run is sane before any of it happens.
type plan struct {
	targets  []target
	byRule   map[string]int
	bySource map[string]int
	samples  []string
	// refused counts rows a company-scoped rule matched but the guard turned down,
	// per reason. They are not deletions and not errors: they are the guard working.
	refused map[string]int
	// matched is every row the rule matched, including those past the cap, so the
	// report can say how much of the work this run is doing.
	matched int
}

// sampledTitles is how many titles a dry run prints. Enough to recognise a cluster
// gone wrong, few enough that an operator actually reads them.
const sampledTitles = 20

func newPlan() *plan {
	return &plan{byRule: map[string]int{}, bySource: map[string]int{}, refused: map[string]int{}}
}

func (p *plan) add(row db.PruneCandidatesRow, rule string) {
	p.matched++
	p.targets = append(p.targets, target{id: row.ID, rule: rule})
	p.byRule[rule]++
	p.bySource[row.Source]++
	if len(p.samples) < sampledTitles {
		p.samples = append(p.samples, fmt.Sprintf("[%s] %s — %s", rule, row.Title, row.Source))
	}
}

func (p *plan) refuse(reason string) { p.refused[reason]++ }

// report prints what the run would do, or did. The breakdown by source is the tripwire
// the design asks for: a batch dominated by one board is the signature of a broken
// board title, not a real cluster, and it is only visible next to the other sources.
func (p *plan) report(w io.Writer, applied bool) error {
	verb := "would delete"
	if applied {
		verb = "deleted"
	}
	if _, err := fmt.Fprintf(w, "%s %d of %d matching rows\n", verb, len(p.targets), p.matched); err != nil {
		return err
	}
	if err := section(w, "BY RULE", p.byRule); err != nil {
		return err
	}
	if err := section(w, "BY SOURCE", p.bySource); err != nil {
		return err
	}
	if len(p.refused) > 0 {
		if err := section(w, "REFUSED (guard)", p.refused); err != nil {
			return err
		}
	}
	if len(p.samples) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nSAMPLE"); err != nil {
		return err
	}
	for _, s := range p.samples {
		if _, err := fmt.Fprintf(w, "  %s\n", s); err != nil {
			return err
		}
	}
	return nil
}

func section(w io.Writer, title string, counts map[string]int) error {
	if len(counts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if _, err := fmt.Fprintf(w, "\n%s\n", title); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range keys {
		if _, err := fmt.Fprintf(tw, "  %s\t%d\n", k, counts[k]); err != nil {
			return err
		}
	}
	return tw.Flush()
}
