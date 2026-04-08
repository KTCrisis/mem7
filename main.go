package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- JSON-RPC 2.0 types ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      any      `json:"id"`
	Result  any      `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP tool schema types ---

type mcpTool struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	InputSchema mcpSchema `json:"inputSchema"`
}

type mcpSchema struct {
	Type       string             `json:"type"`
	Properties map[string]mcpProp `json:"properties,omitempty"`
	Required   []string           `json:"required,omitempty"`
}

type mcpProp struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Items       *mcpItems `json:"items,omitempty"`
	Default     any       `json:"default,omitempty"`
}

type mcpItems struct {
	Type string `json:"type"`
}

// --- Memory types ---

type memoryEntry struct {
	ID      string   `json:"id"`
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Tags    []string `json:"tags"`
	Agent   string   `json:"agent"`
	Created string   `json:"created"`
	Updated string   `json:"updated"`
	TTL     int      `json:"ttl"`
}

// --- Tool definitions ---

var tools = []mcpTool{
	{
		Name:        "memory_store",
		Description: "Store or update a memory entry. If a key already exists, it is updated (upsert).",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"key":   {Type: "string", Description: "Unique key for this memory"},
				"value": {Type: "string", Description: "The content to remember"},
				"tags":  {Type: "array", Description: "Tags for filtering and grouping", Items: &mcpItems{Type: "string"}},
				"agent": {Type: "string", Description: "Identifier of the agent storing this memory"},
				"ttl":   {Type: "number", Description: "Time-to-live in seconds (0 = permanent)", Default: 0},
			},
			Required: []string{"key", "value"},
		},
	},
	{
		Name:        "memory_recall",
		Description: "Recall memories by key, tags, or agent. Returns matching entries sorted by most recent.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"key":   {Type: "string", Description: "Exact key to recall"},
				"tags":  {Type: "array", Description: "Filter by tags (AND logic: all tags must match)", Items: &mcpItems{Type: "string"}},
				"agent": {Type: "string", Description: "Filter by agent identifier"},
				"limit": {Type: "number", Description: "Max number of results (default 10)", Default: 10},
			},
		},
	},
	{
		Name:        "memory_list",
		Description: "List memory keys with metadata (without values). Useful for browsing what is stored.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"tags":  {Type: "array", Description: "Filter by tags (AND logic)", Items: &mcpItems{Type: "string"}},
				"agent": {Type: "string", Description: "Filter by agent identifier"},
			},
		},
	},
	{
		Name:        "memory_forget",
		Description: "Delete memories by key or tags.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"key":  {Type: "string", Description: "Exact key to delete"},
				"tags": {Type: "array", Description: "Delete all entries matching these tags (AND logic)", Items: &mcpItems{Type: "string"}},
			},
		},
	},
}

// --- Server ---

type server struct {
	memDir     string
	maxEntries int
	mu         sync.Mutex
}

func newServer() *server {
	dir := os.Getenv("MEMORY_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".memory-mcp")
	}

	maxEntries := 10000
	if v := os.Getenv("MEMORY_MAX_ENTRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxEntries = n
		}
	}

	return &server{
		memDir:     dir,
		maxEntries: maxEntries,
	}
}

func (s *server) filePath() string {
	return filepath.Join(s.memDir, "memories.json")
}

func (s *server) load() ([]memoryEntry, error) {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []memoryEntry{}, nil
		}
		return nil, err
	}
	var entries []memoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *server) save(entries []memoryEntry) error {
	if err := os.MkdirAll(s.memDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath())
}

func (s *server) purgeExpired(entries []memoryEntry) []memoryEntry {
	now := time.Now()
	result := make([]memoryEntry, 0, len(entries))
	for _, e := range entries {
		if e.TTL > 0 {
			updated, err := time.Parse(time.RFC3339, e.Updated)
			if err == nil && now.Sub(updated) > time.Duration(e.TTL)*time.Second {
				continue
			}
		}
		result = append(result, e)
	}
	return result
}

func generateID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func hasTags(entry memoryEntry, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	tagSet := make(map[string]bool, len(entry.Tags))
	for _, t := range entry.Tags {
		tagSet[t] = true
	}
	for _, t := range tags {
		if !tagSet[t] {
			return false
		}
	}
	return true
}

func parseTags(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			tags = append(tags, s)
		}
	}
	return tags
}

// --- Tool implementations ---

func (s *server) toolStore(params map[string]any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, _ := params["key"].(string)
	value, _ := params["value"].(string)
	if key == "" || value == "" {
		return errResult("key and value are required"), nil
	}

	tags := parseTags(params["tags"])
	agent, _ := params["agent"].(string)
	ttl := 0
	if v, ok := params["ttl"].(float64); ok {
		ttl = int(v)
	}

	entries, err := s.load()
	if err != nil {
		return errResult(fmt.Sprintf("failed to load memories: %v", err)), nil
	}

	entries = s.purgeExpired(entries)
	now := time.Now().UTC().Format(time.RFC3339)

	// Upsert
	found := false
	for i, e := range entries {
		if e.Key == key {
			entries[i].Value = value
			entries[i].Tags = tags
			if agent != "" {
				entries[i].Agent = agent
			}
			entries[i].Updated = now
			entries[i].TTL = ttl
			found = true
			break
		}
	}

	if !found {
		if len(entries) >= s.maxEntries {
			return errResult(fmt.Sprintf("memory full: %d entries (max %d)", len(entries), s.maxEntries)), nil
		}
		entries = append(entries, memoryEntry{
			ID:      generateID(),
			Key:     key,
			Value:   value,
			Tags:    tags,
			Agent:   agent,
			Created: now,
			Updated: now,
			TTL:     ttl,
		})
	}

	if err := s.save(entries); err != nil {
		return errResult(fmt.Sprintf("failed to save: %v", err)), nil
	}

	action := "created"
	if found {
		action = "updated"
	}
	return textResult(fmt.Sprintf("Memory '%s' %s (%d total entries)", key, action, len(entries))), nil
}

func (s *server) toolRecall(params map[string]any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return errResult(fmt.Sprintf("failed to load memories: %v", err)), nil
	}

	purged := s.purgeExpired(entries)
	if len(purged) < len(entries) {
		_ = s.save(purged)
	}
	entries = purged

	key, _ := params["key"].(string)
	tags := parseTags(params["tags"])
	agent, _ := params["agent"].(string)
	limit := 10
	if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var results []memoryEntry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if key != "" && e.Key != key {
			continue
		}
		if !hasTags(e, tags) {
			continue
		}
		if agent != "" && e.Agent != agent {
			continue
		}
		results = append(results, e)
		if len(results) >= limit {
			break
		}
	}

	if len(results) == 0 {
		return textResult("No memories found."), nil
	}

	var sb strings.Builder
	for _, e := range results {
		sb.WriteString(fmt.Sprintf("## %s\n", e.Key))
		sb.WriteString(e.Value)
		sb.WriteString("\n")
		if len(e.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(e.Tags, ", ")))
		}
		if e.Agent != "" {
			sb.WriteString(fmt.Sprintf("Agent: %s\n", e.Agent))
		}
		sb.WriteString(fmt.Sprintf("Updated: %s\n\n", e.Updated))
	}

	return textResult(sb.String()), nil
}

func (s *server) toolList(params map[string]any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return errResult(fmt.Sprintf("failed to load memories: %v", err)), nil
	}

	purged := s.purgeExpired(entries)
	if len(purged) < len(entries) {
		_ = s.save(purged)
	}
	entries = purged

	tags := parseTags(params["tags"])
	agent, _ := params["agent"].(string)

	var sb strings.Builder
	count := 0
	for _, e := range entries {
		if !hasTags(e, tags) {
			continue
		}
		if agent != "" && e.Agent != agent {
			continue
		}
		tagStr := ""
		if len(e.Tags) > 0 {
			tagStr = fmt.Sprintf(" [%s]", strings.Join(e.Tags, ", "))
		}
		agentStr := ""
		if e.Agent != "" {
			agentStr = fmt.Sprintf(" (by %s)", e.Agent)
		}
		sb.WriteString(fmt.Sprintf("- %s%s%s — %s\n", e.Key, tagStr, agentStr, e.Updated))
		count++
	}

	if count == 0 {
		return textResult("No memories found."), nil
	}

	return textResult(fmt.Sprintf("%d memories:\n%s", count, sb.String())), nil
}

func (s *server) toolForget(params map[string]any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, _ := params["key"].(string)
	tags := parseTags(params["tags"])

	if key == "" && len(tags) == 0 {
		return errResult("key or tags required"), nil
	}

	entries, err := s.load()
	if err != nil {
		return errResult(fmt.Sprintf("failed to load memories: %v", err)), nil
	}

	kept := make([]memoryEntry, 0, len(entries))
	removed := 0
	for _, e := range entries {
		shouldRemove := false
		if key != "" && e.Key == key {
			shouldRemove = true
		}
		if len(tags) > 0 && hasTags(e, tags) {
			shouldRemove = true
		}
		if shouldRemove {
			removed++
		} else {
			kept = append(kept, e)
		}
	}

	if removed == 0 {
		return textResult("No matching memories found."), nil
	}

	if err := s.save(kept); err != nil {
		return errResult(fmt.Sprintf("failed to save: %v", err)), nil
	}

	return textResult(fmt.Sprintf("Removed %d memory(ies). %d remaining.", removed, len(kept))), nil
}

// --- MCP protocol handlers ---

func (s *server) handleInitialize() any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "memory-mcp-go",
			"version": "0.1.0",
		},
	}
}

func (s *server) handleToolsList() any {
	return map[string]any{
		"tools": tools,
	}
}

func (s *server) handleToolsCall(params json.RawMessage) any {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return errResult(fmt.Sprintf("invalid params: %v", err))
	}

	args := call.Arguments
	if args == nil {
		args = map[string]any{}
	}

	var result any
	var err error

	switch call.Name {
	case "memory_store":
		result, err = s.toolStore(args)
	case "memory_recall":
		result, err = s.toolRecall(args)
	case "memory_list":
		result, err = s.toolList(args)
	case "memory_forget":
		result, err = s.toolForget(args)
	default:
		return errResult(fmt.Sprintf("unknown tool: %s", call.Name))
	}

	if err != nil {
		return errResult(err.Error())
	}
	return result
}

// --- Helpers ---

func textResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func errResult(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": msg},
		},
		"isError": true,
	}
}

func writeResponse(w *bufio.Writer, resp rpcResponse) {
	data, _ := json.Marshal(resp)
	w.Write(data)
	w.WriteByte('\n')
	w.Flush()
}

// --- Main ---

func main() {
	srv := newServer()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		// Skip notifications (no ID)
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
		default:
			writeResponse(writer, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32601, Message: "method not found"},
			})
			continue
		}

		writeResponse(writer, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		})
	}
}
