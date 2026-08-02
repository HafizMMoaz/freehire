package main

import "testing"

func TestWriteTallySummary(t *testing.T) {
	cases := map[string]struct {
		record func(*writeTally)
		want   string
	}{
		// The healthy shape: a settled board re-crawls almost entirely through the cheap write.
		"mostly unchanged": {
			record: func(w *writeTally) {
				for range 97 {
					w.record("greenhouse", true)
				}
				for range 3 {
					w.record("greenhouse", false)
				}
			},
			want: "greenhouse cheap=97/100 (97%)",
		},
		// The finding this line exists for. A provider that varies a hashed field between crawls
		// — a session token in the url, a re-serialized posted time — takes the cheap write for
		// nothing, and must SAY so. Reporting nothing would read the same as a healthy run.
		"nothing matched": {
			record: func(w *writeTally) {
				for range 40 {
					w.record("workday", false)
				}
			},
			want: "workday cheap=0/40 (0%)",
		},
		// Providers are sorted so two runs of the same file produce comparable lines.
		"several providers": {
			record: func(w *writeTally) {
				w.record("lever", true)
				w.record("ashby", false)
				w.record("ashby", true)
			},
			want: "ashby cheap=1/2 (50%) lever cheap=1/1 (100%)",
		},
		// A run that persisted nothing has nothing to report, and must not print a bare label
		// that reads as a result.
		"no writes": {record: func(*writeTally) {}, want: ""},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWriteTally()
			c.record(w)
			if got := w.summary(); got != c.want {
				t.Errorf("summary() = %q, want %q", got, c.want)
			}
		})
	}
}

// A nil tally is what a Store built without one holds — every test that does not care about
// the share. Recording into it must be a no-op rather than a panic, so no call site needs a
// guard of its own.
func TestWriteTallyNilIsInert(t *testing.T) {
	var w *writeTally
	w.record("greenhouse", true)
	if got := w.summary(); got != "" {
		t.Errorf("summary() = %q, want empty for a nil tally", got)
	}
}
