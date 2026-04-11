package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Protocol tests ---

func TestInitialize(t *testing.T) {
	srv := newTestServer(t)
	result := srv.handleInitialize()
	m := result.(map[string]any)
	if m["protocolVersion"] != "2024-11-05" {
		t.Fatalf("expected protocol 2024-11-05, got %v", m["protocolVersion"])
	}
	info := m["serverInfo"].(map[string]any)
	if info["name"] != "mem7" {
		t.Fatalf("expected server name mem7, got %v", info["name"])
	}
}

func TestToolsList(t *testing.T) {
	srv := newTestServer(t)
	result := srv.handleToolsList()
	m := result.(map[string]any)
	tools := m["tools"].([]mcpTool)
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"memory_store", "memory_recall", "memory_list", "memory_forget"} {
		if !names[name] {
			t.Fatalf("missing tool: %s", name)
		}
	}
}

func TestToolsCallUnknown(t *testing.T) {
	srv := newTestServer(t)
	params, _ := json.Marshal(map[string]any{"name": "nope", "arguments": map[string]any{}})
	result := srv.handleToolsCall(params)
	m := result.(map[string]any)
	if !m["isError"].(bool) {
		t.Fatal("expected error for unknown tool")
	}
}

// --- Store tests ---

func TestStoreAndRecall(t *testing.T) {
	srv := newTestServer(t)

	// Store
	res := callTool(t, srv, "memory_store", map[string]any{
		"key":   "test_key",
		"value": "test_value",
		"tags":  []any{"tag1", "tag2"},
		"agent": "test-agent",
	})
	assertText(t, res, "created")

	// Recall by key
	res = callTool(t, srv, "memory_recall", map[string]any{"key": "test_key"})
	text := getText(t, res)
	if !strings.Contains(text, "test_value") {
		t.Fatalf("expected value in recall, got: %s", text)
	}
	if !strings.Contains(text, "tag1") {
		t.Fatalf("expected tags in recall, got: %s", text)
	}
}

func TestStoreUpsert(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "k1", "value": "v1"})
	callTool(t, srv, "memory_store", map[string]any{"key": "k1", "value": "v2"})

	res := callTool(t, srv, "memory_recall", map[string]any{"key": "k1"})
	text := getText(t, res)
	if !strings.Contains(text, "v2") {
		t.Fatalf("expected updated value v2, got: %s", text)
	}
	if strings.Contains(text, "v1") {
		t.Fatalf("old value v1 should be gone, got: %s", text)
	}
}

func TestStoreMissingParams(t *testing.T) {
	srv := newTestServer(t)
	res := callTool(t, srv, "memory_store", map[string]any{"key": "k1"})
	m := res.(map[string]any)
	if !m["isError"].(bool) {
		t.Fatal("expected error for missing value")
	}
}

// --- Recall tests ---

func TestRecallByTags(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"x", "y"}})
	callTool(t, srv, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"x"}})
	callTool(t, srv, "memory_store", map[string]any{"key": "c", "value": "vc", "tags": []any{"y"}})

	// Tag x AND y -> only "a"
	res := callTool(t, srv, "memory_recall", map[string]any{"tags": []any{"x", "y"}})
	text := getText(t, res)
	if !strings.Contains(text, "va") {
		t.Fatalf("expected va, got: %s", text)
	}
	if strings.Contains(text, "vb") || strings.Contains(text, "vc") {
		t.Fatalf("should only match a, got: %s", text)
	}
}

func TestRecallByAgent(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "a", "value": "va", "agent": "llama3"})
	callTool(t, srv, "memory_store", map[string]any{"key": "b", "value": "vb", "agent": "claude"})

	res := callTool(t, srv, "memory_recall", map[string]any{"agent": "llama3"})
	text := getText(t, res)
	if !strings.Contains(text, "va") {
		t.Fatalf("expected va, got: %s", text)
	}
	if strings.Contains(text, "vb") {
		t.Fatalf("should not contain claude's memory, got: %s", text)
	}
}

func TestRecallLimit(t *testing.T) {
	srv := newTestServer(t)

	for i := 0; i < 20; i++ {
		callTool(t, srv, "memory_store", map[string]any{
			"key":   fmt.Sprintf("k%d", i),
			"value": fmt.Sprintf("v%d", i),
		})
	}

	res := callTool(t, srv, "memory_recall", map[string]any{"limit": float64(3)})
	text := getText(t, res)
	// Should have at most 3 "##" headers
	count := strings.Count(text, "## ")
	if count != 3 {
		t.Fatalf("expected 3 results, got %d", count)
	}
}

func TestRecallEmpty(t *testing.T) {
	srv := newTestServer(t)
	res := callTool(t, srv, "memory_recall", map[string]any{"key": "nonexistent"})
	text := getText(t, res)
	if !strings.Contains(text, "No memories") {
		t.Fatalf("expected no memories message, got: %s", text)
	}
}

// --- List tests ---

func TestList(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"t1"}})
	callTool(t, srv, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"t2"}})

	res := callTool(t, srv, "memory_list", map[string]any{})
	text := getText(t, res)
	if !strings.Contains(text, "2 memories") {
		t.Fatalf("expected 2 memories, got: %s", text)
	}
	// Values should NOT be in list output
	if strings.Contains(text, "va") || strings.Contains(text, "vb") {
		t.Fatalf("list should not contain values, got: %s", text)
	}
}

func TestListFilterByTags(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"staffd"}})
	callTool(t, srv, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"event7"}})

	res := callTool(t, srv, "memory_list", map[string]any{"tags": []any{"staffd"}})
	text := getText(t, res)
	if !strings.Contains(text, "1 memories") {
		t.Fatalf("expected 1 memory, got: %s", text)
	}
	if !strings.Contains(text, "a") {
		t.Fatalf("expected key a, got: %s", text)
	}
}

// --- Forget tests ---

func TestForgetByKey(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "a", "value": "va"})
	callTool(t, srv, "memory_store", map[string]any{"key": "b", "value": "vb"})

	res := callTool(t, srv, "memory_forget", map[string]any{"key": "a"})
	assertText(t, res, "Removed 1")

	res = callTool(t, srv, "memory_recall", map[string]any{"key": "a"})
	text := getText(t, res)
	if !strings.Contains(text, "No memories") {
		t.Fatalf("expected a to be forgotten, got: %s", text)
	}

	// b should still exist
	res = callTool(t, srv, "memory_recall", map[string]any{"key": "b"})
	text = getText(t, res)
	if !strings.Contains(text, "vb") {
		t.Fatalf("b should still exist, got: %s", text)
	}
}

func TestForgetByTags(t *testing.T) {
	srv := newTestServer(t)

	callTool(t, srv, "memory_store", map[string]any{"key": "a", "value": "va", "tags": []any{"temp"}})
	callTool(t, srv, "memory_store", map[string]any{"key": "b", "value": "vb", "tags": []any{"temp"}})
	callTool(t, srv, "memory_store", map[string]any{"key": "c", "value": "vc", "tags": []any{"keep"}})

	res := callTool(t, srv, "memory_forget", map[string]any{"tags": []any{"temp"}})
	assertText(t, res, "Removed 2")

	res = callTool(t, srv, "memory_list", map[string]any{})
	text := getText(t, res)
	if !strings.Contains(text, "1 memories") {
		t.Fatalf("expected 1 remaining, got: %s", text)
	}
}

func TestForgetMissingParams(t *testing.T) {
	srv := newTestServer(t)
	res := callTool(t, srv, "memory_forget", map[string]any{})
	m := res.(map[string]any)
	if !m["isError"].(bool) {
		t.Fatal("expected error when no key or tags")
	}
}

// --- TTL tests ---

func TestTTLExpiry(t *testing.T) {
	srv := newTestServer(t)

	// Store with TTL=1 second
	callTool(t, srv, "memory_store", map[string]any{"key": "ephemeral", "value": "gone soon", "ttl": float64(1)})

	// Should exist immediately
	res := callTool(t, srv, "memory_recall", map[string]any{"key": "ephemeral"})
	text := getText(t, res)
	if strings.Contains(text, "No memories") {
		t.Fatal("should exist immediately after store")
	}

	// Manually hack the updated time to be in the past
	entries, _ := srv.load()
	for i, e := range entries {
		if e.Key == "ephemeral" {
			entries[i].Updated = "2020-01-01T00:00:00Z"
		}
	}
	srv.save(entries)

	// Should be expired now
	res = callTool(t, srv, "memory_recall", map[string]any{"key": "ephemeral"})
	text = getText(t, res)
	if !strings.Contains(text, "No memories") {
		t.Fatalf("should be expired, got: %s", text)
	}
}

// --- Stdio round-trip test ---

func TestStdioRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MEMORY_DIR", dir)

	messages := []rpcRequest{
		{JSONRPC: "2.0", ID: 1, Method: "initialize"},
		{JSONRPC: "2.0", ID: 2, Method: "tools/list"},
	}

	// Build store call
	storeParams, _ := json.Marshal(map[string]any{
		"name":      "memory_store",
		"arguments": map[string]any{"key": "hello", "value": "world"},
	})
	messages = append(messages, rpcRequest{JSONRPC: "2.0", ID: 3, Method: "tools/call", Params: storeParams})

	// Build recall call
	recallParams, _ := json.Marshal(map[string]any{
		"name":      "memory_recall",
		"arguments": map[string]any{"key": "hello"},
	})
	messages = append(messages, rpcRequest{JSONRPC: "2.0", ID: 4, Method: "tools/call", Params: recallParams})

	var input bytes.Buffer
	for _, msg := range messages {
		data, _ := json.Marshal(msg)
		input.Write(data)
		input.WriteByte('\n')
	}

	srv := newServer()
	scanner := bufio.NewScanner(&input)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var output bytes.Buffer
	writer := bufio.NewWriter(&output)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if req.ID == nil {
			continue
		}

		var result any
		switch req.Method {
		case "initialize":
			result = srv.handleInitialize()
		case "tools/list":
			result = srv.handleToolsList()
		case "tools/call":
			result = srv.handleToolsCall(req.Params)
		}

		writeResponse(writer, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	}

	// Parse responses
	respScanner := bufio.NewScanner(&output)
	var responses []rpcResponse
	for respScanner.Scan() {
		var resp rpcResponse
		if err := json.Unmarshal(respScanner.Bytes(), &resp); err == nil {
			responses = append(responses, resp)
		}
	}

	if len(responses) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(responses))
	}

	// Check recall result contains "world"
	lastResult, _ := json.Marshal(responses[3].Result)
	if !strings.Contains(string(lastResult), "world") {
		t.Fatalf("expected recall to contain 'world', got: %s", string(lastResult))
	}
}

// --- Test helpers ---

func newTestServer(t *testing.T) *server {
	t.Helper()
	dir := t.TempDir()
	return &server{
		memDir:     dir,
		maxEntries: 10000,
	}
}

func callTool(t *testing.T, srv *server, name string, args map[string]any) any {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	return srv.handleToolsCall(params)
}

func getText(t *testing.T, result any) string {
	t.Helper()
	m := result.(map[string]any)
	content := m["content"].([]map[string]any)
	return content[0]["text"].(string)
}

func assertText(t *testing.T, result any, substr string) {
	t.Helper()
	text := getText(t, result)
	if !strings.Contains(text, substr) {
		t.Fatalf("expected %q in text, got: %s", substr, text)
	}
}

// Need fmt for Sprintf in tests
func init() {
	_ = filepath.Join
	_ = os.TempDir
}
