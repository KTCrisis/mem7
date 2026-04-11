package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func newDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	return NewDispatcher(NewStore(t.TempDir(), 10000))
}

func TestDispatcherInitialize(t *testing.T) {
	d := newDispatcher(t)
	raw, err := d.Call(context.Background(), "initialize", nil)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["protocolVersion"] != ProtocolVersion {
		t.Fatalf("expected protocol %s, got %v", ProtocolVersion, m["protocolVersion"])
	}
	info := m["serverInfo"].(map[string]any)
	if info["name"] != "mem7" {
		t.Fatalf("expected server name mem7, got %v", info["name"])
	}
}

func TestDispatcherToolsList(t *testing.T) {
	d := newDispatcher(t)
	raw, err := d.Call(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var m struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(m.Tools))
	}
	names := map[string]bool{}
	for _, tool := range m.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"memory_store", "memory_recall", "memory_list", "memory_forget"} {
		if !names[name] {
			t.Fatalf("missing tool: %s", name)
		}
	}
}

func TestDispatcherUnknownMethod(t *testing.T) {
	d := newDispatcher(t)
	_, err := d.Call(context.Background(), "does/not/exist", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	var rerr *RPCError
	if !errorAs(err, &rerr) {
		t.Fatalf("expected RPCError, got %T", err)
	}
	if rerr.Code != -32601 {
		t.Fatalf("expected code -32601, got %d", rerr.Code)
	}
}

func TestDispatcherUnknownTool(t *testing.T) {
	d := newDispatcher(t)
	params, _ := json.Marshal(map[string]any{"name": "nope", "arguments": map[string]any{}})
	raw, err := d.Call(context.Background(), "tools/call", params)
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !m["isError"].(bool) {
		t.Fatal("expected tool-level error envelope")
	}
	content := m["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "unknown tool") {
		t.Fatalf("expected 'unknown tool' message, got %q", text)
	}
}

// errorAs is a small wrapper so the test file does not need to import
// the errors package just for one call.
func errorAs(err error, target any) bool {
	rerr, ok := err.(*RPCError)
	if !ok {
		return false
	}
	ptr := target.(**RPCError)
	*ptr = rerr
	return true
}
