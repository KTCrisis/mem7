package memory

import "testing"

func TestNaturalizeFTSQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "natural question with stop words",
			in:   "When did Caroline go to the LGBTQ support group?",
			want: "caroline* lgbtq* support* group*",
		},
		{
			name: "stemming via trailing wildcard",
			in:   "When did Melanie paint a sunrise?",
			want: "melanie* paint* sunrise*",
		},
		{
			name: "short tokens kept without wildcard",
			in:   "Caroline LGBTQ",
			want: "caroline* lgbtq*",
		},
		{
			name: "all-stopwords input falls back to original",
			in:   "what is the",
			want: "what is the",
		},
		{
			name: "deduplication preserves first occurrence",
			in:   "Melanie Melanie paint paint",
			want: "melanie* paint*",
		},
		{
			name: "punctuation stripped, short tokens keep no wildcard",
			in:   "Caroline's painting, yes!",
			want: "caroline* painting* yes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := naturalizeFTSQuery(tc.in)
			// Re-join with OR to match our spec since the function does it.
			want := tc.want
			if tc.name != "all-stopwords input falls back to original" {
				// Transform expected space-joined form into OR-joined.
				// (Keeping test cases readable by not writing OR inline.)
				want = orJoin(tc.want)
			}
			if got != want {
				t.Errorf("naturalizeFTSQuery(%q) = %q ; want %q", tc.in, got, want)
			}
		})
	}
}

func orJoin(spaceJoined string) string {
	// Convert "a b c" into "a OR b OR c".
	// Test helper only — keeps the expected strings concise above.
	out := ""
	for i, tok := range split(spaceJoined) {
		if i > 0 {
			out += " OR "
		}
		out += tok
	}
	return out
}

func split(s string) []string {
	out := make([]string, 0)
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
