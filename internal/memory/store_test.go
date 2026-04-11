package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir(), 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func call(t *testing.T, s *Store, name string, args map[string]any) Result {
	t.Helper()
	switch name {
	case "memory_store":
		return s.ToolStore(args)
	case "memory_recall":
		return s.ToolRecall(args)
	case "memory_list":
		return s.ToolList(args)
	case "memory_forget":
		return s.ToolForget(args)
	}
	t.Fatalf("unknown tool: %s", name)
	return nil
}

func getText(t *testing.T, r Result) string {
	t.Helper()
	content := r["content"].([]map[string]any)
	return content[0]["text"].(string)
}

func assertText(t *testing.T, r Result, substr string) {
	t.Helper()
	text := getText(t, r)
	if !strings.Contains(text, substr) {
		t.Fatalf("expected %q in text, got: %s", substr, text)
	}
}

func TestStoreAndRecall(t *testing.T) {
	s := newStore(t)
	res := call(t, s, "memory_store", map[string]any{
		"key":   "test_key",
		"value": "test_value",
		"tags":  []any{"tag1", "tag2"},
		"agent": "test-agent",
	})
	assertText(t, res, "created")

	res = call(t, s, "memory_recall", map[string]any{"key": "test_key"})
	text := getText(t, res)
	if !strings.Contains(text, "test_value") {
		t.Fatalf("expected value in recall, got: %s", text)
	}
	if !strings.Contains(text, "tag1") {
		t.Fatalf("expected tags in recall, got: %s", text)
	}
}

func TestStoreUpsert(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "k1", "value": "v1"})
	call(t, s, "memory_store", map[string]any{"key": "k1", "value": "v2"})

	res := call(t, s, "memory_recall", map[string]any{"key": "k1"})
	text := getText(t, res)
	if !strings.Contains(text, "v2") {
		t.Fatalf("expected updated value v2, got: %s", text)
	}
	if strings.Contains(text, "v1") {
		t.Fatalf("old value v1 should not leak into latest recall, got: %s", text)
	}
}

func TestStoreMissingParams(t *testing.T) {
	s := newStore(t)
	res := call(t, s, "memory_store", map[string]any{"key": "k1"})
	if !res["isError"].(bool) {
		t.Fatal("expected error for missing value")
	}
}

func TestRecallByTags(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"x", "y"}})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"x"}})
	call(t, s, "memory_store", map[string]any{"key": "c", "value": "vc", "tags": []any{"y"}})

	res := call(t, s, "memory_recall", map[string]any{"tags": []any{"x", "y"}})
	text := getText(t, res)
	if !strings.Contains(text, "va") {
		t.Fatalf("expected va, got: %s", text)
	}
	if strings.Contains(text, "vb") || strings.Contains(text, "vc") {
		t.Fatalf("should only match a, got: %s", text)
	}
}

func TestRecallByAgent(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va", "agent": "llama3"})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb", "agent": "claude"})

	res := call(t, s, "memory_recall", map[string]any{"agent": "llama3"})
	text := getText(t, res)
	if !strings.Contains(text, "va") {
		t.Fatalf("expected va, got: %s", text)
	}
	if strings.Contains(text, "vb") {
		t.Fatalf("should not contain claude's memory, got: %s", text)
	}
}

func TestRecallLimit(t *testing.T) {
	s := newStore(t)
	for i := 0; i < 20; i++ {
		call(t, s, "memory_store", map[string]any{
			"key":   fmt.Sprintf("k%d", i),
			"value": fmt.Sprintf("v%d", i),
		})
	}

	res := call(t, s, "memory_recall", map[string]any{"limit": float64(3)})
	text := getText(t, res)
	count := strings.Count(text, "## ")
	if count != 3 {
		t.Fatalf("expected 3 results, got %d", count)
	}
}

func TestRecallEmpty(t *testing.T) {
	s := newStore(t)
	res := call(t, s, "memory_recall", map[string]any{"key": "nonexistent"})
	assertText(t, res, "No memories")
}

func TestList(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"t1"}})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"t2"}})

	res := call(t, s, "memory_list", map[string]any{})
	text := getText(t, res)
	if !strings.Contains(text, "2 memories") {
		t.Fatalf("expected 2 memories, got: %s", text)
	}
	if strings.Contains(text, "va") || strings.Contains(text, "vb") {
		t.Fatalf("list should not contain values, got: %s", text)
	}
}

func TestListFilterByTags(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"staffd"}})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"event7"}})

	res := call(t, s, "memory_list", map[string]any{"tags": []any{"staffd"}})
	text := getText(t, res)
	if !strings.Contains(text, "1 memories") {
		t.Fatalf("expected 1 memory, got: %s", text)
	}
}

func TestForgetByKey(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va"})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb"})

	res := call(t, s, "memory_forget", map[string]any{"key": "a"})
	assertText(t, res, "Removed 1")

	res = call(t, s, "memory_recall", map[string]any{"key": "a"})
	assertText(t, res, "No memories")

	res = call(t, s, "memory_recall", map[string]any{"key": "b"})
	assertText(t, res, "vb")
}

func TestForgetByTags(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"temp"}})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"temp"}})
	call(t, s, "memory_store", map[string]any{"key": "c", "value": "vc", "tags": []any{"keep"}})

	res := call(t, s, "memory_forget", map[string]any{"tags": []any{"temp"}})
	assertText(t, res, "Removed 2")

	res = call(t, s, "memory_list", map[string]any{})
	assertText(t, res, "1 memories")
}

func TestSearchFTS(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{
		"key": "go_interfaces", "value": "Go interfaces are satisfied implicitly", "tags": []any{"go"},
	})
	call(t, s, "memory_store", map[string]any{
		"key": "python_classes", "value": "Python classes use explicit inheritance", "tags": []any{"python"},
	})
	call(t, s, "memory_store", map[string]any{
		"key": "go_channels", "value": "Channels are typed conduits for goroutines", "tags": []any{"go"},
	})

	// Basic search — "interfaces" should hit go_interfaces only.
	res := s.ToolSearch(map[string]any{"query": "interfaces"})
	text := getText(t, res)
	if !strings.Contains(text, "go_interfaces") {
		t.Fatalf("expected go_interfaces, got: %s", text)
	}
	if strings.Contains(text, "python_classes") || strings.Contains(text, "go_channels") {
		t.Fatalf("unexpected hits: %s", text)
	}

	// Multi-token search — "goroutines channels" should hit go_channels.
	res = s.ToolSearch(map[string]any{"query": "goroutines channels"})
	assertText(t, res, "go_channels")

	// Tag post-filter — search "typed" with tag python yields nothing.
	res = s.ToolSearch(map[string]any{"query": "typed", "tags": []any{"python"}})
	assertText(t, res, "No memories")

	// Missing query returns error envelope.
	res = s.ToolSearch(map[string]any{})
	if !res["isError"].(bool) {
		t.Fatal("expected error for missing query")
	}
}

func TestSearchHandlesSpecialChars(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{
		"key": "hyphen_doc", "value": "Test entry mentioning claude-code and mem7-check integration",
	})
	call(t, s, "memory_store", map[string]any{
		"key": "accents_doc", "value": "Entrée avec des caractères accentués",
	})

	// Hyphen used to crash FTS5 with "no such column" — must now match.
	res := s.ToolSearch(map[string]any{"query": "claude-code"})
	assertText(t, res, "hyphen_doc")

	// Unicode punctuation must not break the parser either.
	res = s.ToolSearch(map[string]any{"query": "accentués"})
	assertText(t, res, "accents_doc")

	// Power-user operators still pass through.
	res = s.ToolSearch(map[string]any{"query": "claude-code AND mem7-check"})
	assertText(t, res, "hyphen_doc")
}

func TestGetFileAndRange(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "k1", "value": "first content"})
	call(t, s, "memory_store", map[string]any{"key": "k2", "value": "second content"})

	// Pick the daily file that was created. Use list to discover it.
	files, err := listDailyFiles(s.dir)
	if err != nil || len(files) == 0 {
		t.Fatalf("expected at least one daily file, got %v %v", files, err)
	}
	// Path argument is relative to workspace/
	rel := strings.TrimPrefix(files[0], filepath.Join(s.dir, "workspace")+string(filepath.Separator))

	res := s.ToolGet(map[string]any{"path": rel})
	text := getText(t, res)
	if !strings.Contains(text, "first content") || !strings.Contains(text, "second content") {
		t.Fatalf("expected both entries in full read, got: %s", text)
	}

	// Range read — just the first few lines.
	res = s.ToolGet(map[string]any{"path": rel, "from_line": float64(1), "to_line": float64(3)})
	text = getText(t, res)
	if !strings.Contains(text, "## k1") {
		t.Fatalf("expected k1 heading in range, got: %s", text)
	}
	if strings.Contains(text, "second content") {
		t.Fatalf("range should not include second entry, got: %s", text)
	}
}

func TestGetRefusesTraversal(t *testing.T) {
	s := newStore(t)
	res := s.ToolGet(map[string]any{"path": "../../../etc/passwd"})
	if !res["isError"].(bool) {
		t.Fatal("expected error for path traversal")
	}
	assertText(t, res, "escapes workspace")
}

func TestGetMissingPath(t *testing.T) {
	s := newStore(t)
	res := s.ToolGet(map[string]any{})
	if !res["isError"].(bool) {
		t.Fatal("expected error for missing path")
	}
}

func TestSearchIgnoresDeleted(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "alive", "value": "phoenix"})
	call(t, s, "memory_store", map[string]any{"key": "dead", "value": "phoenix"})
	call(t, s, "memory_forget", map[string]any{"key": "dead"})

	res := s.ToolSearch(map[string]any{"query": "phoenix"})
	text := getText(t, res)
	if !strings.Contains(text, "alive") {
		t.Fatalf("expected alive, got: %s", text)
	}
	if strings.Contains(text, "## dead") {
		t.Fatalf("deleted entry leaked: %s", text)
	}
}

func TestForgetMissingParams(t *testing.T) {
	s := newStore(t)
	res := call(t, s, "memory_forget", map[string]any{})
	if !res["isError"].(bool) {
		t.Fatal("expected error when no key or tags")
	}
}

// TestRescanRebuildsFromMarkdown verifies that dropping the SQLite
// index and calling Rescan reconstructs an identical live state from
// the markdown workspace alone.
func TestRescanRebuildsFromMarkdown(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"x"}})
	call(t, s, "memory_store", map[string]any{"key": "b", "value": "vb", "agent": "claude"})
	call(t, s, "memory_store", map[string]any{"key": "a", "value": "va2"}) // upsert
	call(t, s, "memory_forget", map[string]any{"key": "b"})

	beforeList := getText(t, call(t, s, "memory_list", map[string]any{}))

	n, err := s.Rescan()
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 live entry after rescan, got %d", n)
	}

	afterList := getText(t, call(t, s, "memory_list", map[string]any{}))
	if beforeList != afterList {
		t.Fatalf("list mismatch after rescan:\nbefore: %s\nafter:  %s", beforeList, afterList)
	}

	res := call(t, s, "memory_recall", map[string]any{"key": "a"})
	assertText(t, res, "va2")
}

// TestMigrateV1 imports a legacy flat JSON file and verifies the
// entries become queryable via the new storage backend.
func TestMigrateV1(t *testing.T) {
	dir := t.TempDir()
	legacyJSON := `[
  {"id":"1","key":"k1","value":"v1","tags":["a"],"agent":"claude","created":"2026-04-01T10:00:00Z","updated":"2026-04-01T10:00:00Z","ttl":0},
  {"id":"2","key":"k2","value":"v2","tags":[],"agent":"","created":"2026-04-02T11:00:00Z","updated":"2026-04-02T11:00:00Z","ttl":0}
]`
	if err := os.WriteFile(filepath.Join(dir, "memories.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewStore(dir, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	n, err := MigrateV1(s)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 entries imported, got %d", n)
	}

	if _, err := os.Stat(filepath.Join(dir, "memories.json")); !os.IsNotExist(err) {
		t.Fatal("legacy file should have been renamed")
	}
	if _, err := os.Stat(filepath.Join(dir, "memories.json.v0.1.bak")); err != nil {
		t.Fatal("backup file missing")
	}

	res := call(t, s, "memory_recall", map[string]any{"key": "k1"})
	assertText(t, res, "v1")
	res = call(t, s, "memory_recall", map[string]any{"key": "k2"})
	assertText(t, res, "v2")

	// Second call is a no-op.
	n2, err := MigrateV1(s)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("expected 0 on second call, got %d", n2)
	}
}
