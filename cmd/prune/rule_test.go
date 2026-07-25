package main

import "testing"

func techPtr(b bool) *bool { return &b }

// The rule decides what a permanent deletion targets, so every branch is pinned:
// which signal wins, which company state each company-scoped rule needs, and — most
// importantly — everything it must refuse to touch.
func TestMatchRule(t *testing.T) {
	crawled := map[string]bool{"greenhouse": true, "workday": true}

	cases := []struct {
		name string
		c    candidate
		ev   evidence
		want string // "" = keep
	}{
		{
			name: "blue-collar title is removed regardless of the company",
			c:    candidate{Source: "greenhouse", Title: "Registered Nurse"},
			ev:   evidence{anyTech: true, anySkills: true},
			want: ruleTitle,
		},
		{
			name: "technical evidence vetoes the title dictionary",
			c:    candidate{Source: "greenhouse", Title: "DevOps Engineer (HVAC IoT Platform)", IsTech: techPtr(true)},
			ev:   evidence{},
			want: "",
		},
		{
			name: "business role at a company with no technical evidence is removed",
			c:    candidate{Source: "greenhouse", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:   evidence{},
			want: ruleBusiness,
		},
		{
			name: "the same business role is kept where the company has posted technical work",
			c:    candidate{Source: "greenhouse", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:   evidence{anyTech: true},
			want: "",
		},
		{
			name: "unclassified job at a company showing nothing at all is removed",
			c:    candidate{Source: "greenhouse", Title: "Team Member"},
			ev:   evidence{},
			want: ruleUnknown,
		},
		{
			name: "tagged skills alone keep an unclassified job",
			c:    candidate{Source: "greenhouse", Title: "Team Member"},
			ev:   evidence{anySkills: true},
			want: "",
		},
		{
			name: "an unclassified job is kept wherever the company has any evidence",
			c:    candidate{Source: "workday", Title: "Team Member"},
			ev:   evidence{anyTech: true},
			want: "",
		},
		{
			name: "a technical job is never a target",
			c:    candidate{Source: "greenhouse", Title: "Backend Engineer", Category: "backend", IsTech: techPtr(true)},
			ev:   evidence{},
			want: "",
		},
		{
			name: "a source no crawl re-admits is never touched, even by the title rule",
			c:    candidate{Source: "telegram", Title: "Registered Nurse"},
			ev:   evidence{},
			want: "",
		},
		{
			name: "a non-crawled source is exempt from the company rules too",
			c:    candidate{Source: "manual", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:   evidence{},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchRule(tc.c, tc.ev, crawled)
			if tc.want == "" {
				if ok {
					t.Errorf("matched %q, want kept", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("matched %q (ok=%v), want %q", got, ok, tc.want)
			}
		})
	}
}

// The title rule is self-sufficient because ingest applies the same dictionary, but
// the company-scoped rules have no ingest counterpart: the bucket does not exist at
// crawl time. Deleting under them without retiring the board undoes itself within the
// hour, so the worker has to be able to tell the two classes apart.
func TestCompanyScopedRulesAreIdentifiable(t *testing.T) {
	if companyScoped(ruleTitle) {
		t.Error("the title rule is enforced at ingest, so it needs no board retirement")
	}
	for _, rule := range []string{ruleBusiness, ruleUnknown} {
		if !companyScoped(rule) {
			t.Errorf("%q depends on the company bucket and has no ingest counterpart, so it requires board retirement", rule)
		}
	}
}
