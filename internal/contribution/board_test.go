package contribution

import "testing"

// The partial unique index that backs the review queue's dedup (RecordReview) keys on this
// function's exact output, so a trailing-slash variant of the same page must canonicalize to
// the same string or it queues as a second row for a human to triage twice.
func TestStripQueryFragmentTrimsTrailingSlash(t *testing.T) {
	with := stripQueryFragment("https://dropbox.jobs/en/jobs/12345/title/")
	without := stripQueryFragment("https://dropbox.jobs/en/jobs/12345/title")
	if with != without {
		t.Errorf("stripQueryFragment diverged on a trailing slash: %q vs %q", with, without)
	}
	if with != "https://dropbox.jobs/en/jobs/12345/title" {
		t.Errorf("stripQueryFragment = %q, want the trailing slash trimmed", with)
	}
}

func TestStripQueryFragmentDropsQueryAndFragment(t *testing.T) {
	got := stripQueryFragment("https://example.com/careers/123?utm=x&ref=y#apply")
	if want := "https://example.com/careers/123"; got != want {
		t.Errorf("stripQueryFragment = %q, want %q", got, want)
	}
}

// A bare apex has no path to trim — it must not gain a trailing slash it never had, since
// that would itself be a canonicalization mismatch against a link that omitted it.
func TestStripQueryFragmentLeavesABareApexAlone(t *testing.T) {
	got := stripQueryFragment("https://example.com")
	if want := "https://example.com"; got != want {
		t.Errorf("stripQueryFragment = %q, want %q", got, want)
	}
}
