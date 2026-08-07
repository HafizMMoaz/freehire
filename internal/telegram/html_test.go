package telegram

import "testing"

func TestTextToHTML(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			// The extraction prompt asks the model to preserve bullet points, so this is the
			// norm, not an edge case: a run of "- "-prefixed lines becomes a <ul>, matching
			// what an ATS adapter's plain-text description already renders for the same shape.
			name: "bullet lines become a list, not literal hyphens",
			in:   "Требования:\n- Go\n- Postgres\n\nПишите @hr",
			want: "<p>Требования:</p><ul><li>Go</li><li>Postgres</li></ul><p>Пишите @hr</p>",
		},
		{
			name: "markup is escaped, not interpreted",
			in:   "Stack: C++ & <script>alert(1)</script>",
			want: "<p>Stack: C++ &amp; &lt;script&gt;alert(1)&lt;/script&gt;</p>",
		},
		{
			name: "surrounding whitespace trimmed",
			in:   "\n\n  hello  \n\n",
			want: "<p>hello</p>",
		},
		{
			name: "non-bullet lines within a paragraph join with a line break",
			in:   "Office in Berlin.\nRemote within the EU.",
			want: "<p>Office in Berlin.<br>Remote within the EU.</p>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TextToHTML(tc.in); got != tc.want {
				t.Errorf("TextToHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
