package sources_test

import (
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

func TestRefFromURL_Greenhouse(t *testing.T) {
	tests := []struct {
		name, url, wantExternalID string
	}{
		{
			// What an application form on job-boards.greenhouse.io looks like. The
			// job id rides `token`; `jr_id` is Greenhouse's internal requisition id,
			// which the catalog does not carry.
			name:           "embedded application form",
			url:            "https://job-boards.greenhouse.io/embed/job_app?for=stripe&jr_id=6a2444ad757ade085b6affd5&token=7826765",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "embedded form on the legacy boards host",
			url:            "https://boards.greenhouse.io/embed/job_app?for=stripe&token=7826765",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "job detail page",
			url:            "https://job-boards.greenhouse.io/stripe/jobs/7826765",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "job detail page on the legacy host, with tracking query",
			url:            "https://boards.greenhouse.io/stripe/jobs/7826765?gh_src=abc123",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "trailing slash",
			url:            "https://job-boards.greenhouse.io/stripe/jobs/7826765/",
			wantExternalID: "stripe:7826765",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := sources.RefFromURL(tt.url)
			if !ok {
				t.Fatalf("RefFromURL(%q) did not resolve", tt.url)
			}
			if ref.Source != "greenhouse" {
				t.Errorf("source = %q, want greenhouse", ref.Source)
			}
			if ref.ExternalID != tt.wantExternalID {
				t.Errorf("external id = %q, want %q", ref.ExternalID, tt.wantExternalID)
			}
		})
	}
}

func TestRefFromURL_Unresolvable(t *testing.T) {
	tests := []struct{ name, url string }{
		{"empty", ""},
		{"not a url", "reCAPTCHA"},
		{"a board we do not recognise", "https://jobs.lever.co/acme/1234"},
		{"greenhouse board listing, no job", "https://job-boards.greenhouse.io/stripe"},
		{"embedded form without the job id", "https://job-boards.greenhouse.io/embed/job_app?for=stripe"},
		{"embedded form without the board", "https://job-boards.greenhouse.io/embed/job_app?token=7826765"},
		{"a non-numeric job id", "https://job-boards.greenhouse.io/stripe/jobs/not-an-id"},
		// The company's own careers page carries the job id but not the board
		// token, and the board is not derivable from the host — so this stays
		// unresolved rather than guessing a board from the domain.
		{"company careers page with gh_jid", "https://stripe.com/jobs/search?gh_jid=7954688"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ref, ok := sources.RefFromURL(tt.url); ok {
				t.Fatalf("RefFromURL(%q) resolved to %+v, want no match", tt.url, ref)
			}
		})
	}
}

func TestRefFromURL_IsCaseInsensitiveAboutTheBoard(t *testing.T) {
	ref, ok := sources.RefFromURL("https://job-boards.greenhouse.io/embed/job_app?for=Stripe&token=7826765")
	if !ok {
		t.Fatal("did not resolve")
	}
	// Board tokens are lowercase in the catalog; a link that shouts still matches.
	if ref.ExternalID != "stripe:7826765" {
		t.Fatalf("external id = %q, want the lowercased board", ref.ExternalID)
	}
}
