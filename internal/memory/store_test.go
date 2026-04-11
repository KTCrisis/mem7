package memory

import (
	"fmt"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir(), 10000)
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
		t.Fatalf("old value v1 should be gone, got: %s", text)
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

func TestForgetMissingParams(t *testing.T) {
	s := newStore(t)
	res := call(t, s, "memory_forget", map[string]any{})
	if !res["isError"].(bool) {
		t.Fatal("expected error when no key or tags")
	}
}

func TestTTLExpiry(t *testing.T) {
	s := newStore(t)
	call(t, s, "memory_store", map[string]any{"key": "ephemeral", "value": "gone soon", "ttl": float64(1)})

	res := call(t, s, "memory_recall", map[string]any{"key": "ephemeral"})
	if strings.Contains(getText(t, res), "No memories") {
		t.Fatal("should exist immediately after store")
	}

	// Manually backdate the updated timestamp to force expiry.
	entries, _ := s.load()
	for i, e := range entries {
		if e.Key == "ephemeral" {
			entries[i].Updated = "2020-01-01T00:00:00Z"
		}
	}
	_ = s.save(entries)

	res = call(t, s, "memory_recall", map[string]any{"key": "ephemeral"})
	assertText(t, res, "No memories")
}
