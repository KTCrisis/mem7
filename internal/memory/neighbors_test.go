package memory

import (
	"testing"
	"time"
)

func TestParseSequentialKey(t *testing.T) {
	cases := []struct {
		name         string
		key          string
		wantPrefix   string
		wantIdx      int
		wantWidth    int
		wantOk       bool
	}{
		{"locomo turn key", "conv-26.session_1.t005", "conv-26.session_1.t", 5, 3, true},
		{"simple turn key", "session.t10", "session.t", 10, 2, true},
		{"zero-padded large", "batch.item0042", "batch.item", 42, 4, true},
		{"no digits suffix", "user_marc_profile", "", 0, 0, false},
		{"no dot separator", "t005", "", 0, 0, false},
		{"trailing letters only", "conv-26.session_1.header", "", 0, 0, false},
		{"digits only after dot (no letters)", "conv.1234", "", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix, idx, width, ok := parseSequentialKey(tc.key)
			if ok != tc.wantOk || prefix != tc.wantPrefix || idx != tc.wantIdx || width != tc.wantWidth {
				t.Errorf("parseSequentialKey(%q) = (%q, %d, %d, %v), want (%q, %d, %d, %v)",
					tc.key, prefix, idx, width, ok, tc.wantPrefix, tc.wantIdx, tc.wantWidth, tc.wantOk)
			}
		})
	}
}

func TestBuildNeighborKey(t *testing.T) {
	cases := []struct {
		prefix string
		idx    int
		width  int
		want   string
	}{
		{"conv-26.session_1.t", 4, 3, "conv-26.session_1.t004"},
		{"batch.item", 42, 4, "batch.item0042"},
		{"session.t", 10, 2, "session.t10"},
		{"session.t", 9, 2, "session.t09"},
	}
	for _, tc := range cases {
		got := buildNeighborKey(tc.prefix, tc.idx, tc.width)
		if got != tc.want {
			t.Errorf("buildNeighborKey(%q, %d, %d) = %q, want %q", tc.prefix, tc.idx, tc.width, got, tc.want)
		}
	}
}

func TestNeighborCandidates(t *testing.T) {
	results := []fact{
		{Entity: "conv-26.session_1.t005"},
		{Entity: "conv-26.session_1.t006"}, // adjacent to t005
		{Entity: "user_marc_profile"},       // non-sequential, no neighbors
	}
	got := neighborCandidates(results, 1)
	// Expected : t004 (neighbor of t005), t007 (neighbor of t006).
	// t006 is already in results → not a candidate.
	// t005 would be a candidate of t006 but is already in results.
	want := map[string]bool{
		"conv-26.session_1.t004": true,
		"conv-26.session_1.t007": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates, want %d : %v", len(got), len(want), got)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected candidate : %q", k)
		}
	}
}

func TestNeighborCandidatesZeroRadius(t *testing.T) {
	results := []fact{{Entity: "conv-26.session_1.t005"}}
	got := neighborCandidates(results, 0)
	if len(got) != 0 {
		t.Errorf("radius 0 should yield no candidates, got %v", got)
	}
}

func TestWeaveNeighbors(t *testing.T) {
	// Simulate : original top-k has t005 and t010 (ranked by BM25 in that order).
	// Neighbors fetched : t004, t006 (around t005) and t009, t011 (around t010).
	// Expected woven order : t004, t005, t006, t009, t010, t011.
	ts := time.Now()
	results := []fact{
		{Entity: "c.s.t005", Updated: ts},
		{Entity: "c.s.t010", Updated: ts},
	}
	neighbors := []fact{
		{Entity: "c.s.t004", Updated: ts},
		{Entity: "c.s.t006", Updated: ts},
		{Entity: "c.s.t009", Updated: ts},
		{Entity: "c.s.t011", Updated: ts},
	}
	woven := weaveNeighbors(results, neighbors)
	wantOrder := []string{"c.s.t004", "c.s.t005", "c.s.t006", "c.s.t009", "c.s.t010", "c.s.t011"}
	if len(woven) != len(wantOrder) {
		t.Fatalf("woven len %d, want %d", len(woven), len(wantOrder))
	}
	for i, f := range woven {
		if f.Entity != wantOrder[i] {
			t.Errorf("position %d : got %q, want %q", i, f.Entity, wantOrder[i])
		}
	}
}

func TestWeaveNeighborsDedup(t *testing.T) {
	// When two results are adjacent (t005, t007) their neighbor lists overlap (t006).
	// The shared neighbor must appear exactly once, in the correct position.
	ts := time.Now()
	results := []fact{
		{Entity: "c.s.t005", Updated: ts},
		{Entity: "c.s.t007", Updated: ts},
	}
	neighbors := []fact{
		{Entity: "c.s.t004", Updated: ts},
		{Entity: "c.s.t006", Updated: ts},
		{Entity: "c.s.t008", Updated: ts},
	}
	woven := weaveNeighbors(results, neighbors)
	// Expected : t004, t005, t006 (from t005's group), then t007, t008 (t006 dedup'd).
	wantOrder := []string{"c.s.t004", "c.s.t005", "c.s.t006", "c.s.t007", "c.s.t008"}
	if len(woven) != len(wantOrder) {
		t.Fatalf("woven len %d, want %d : %v", len(woven), len(wantOrder), woven)
	}
	for i, f := range woven {
		if f.Entity != wantOrder[i] {
			t.Errorf("position %d : got %q, want %q", i, f.Entity, wantOrder[i])
		}
	}
}
