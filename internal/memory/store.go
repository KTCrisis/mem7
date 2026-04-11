package memory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry is a single memory record persisted in the flat JSON store.
// The v0.2.0 schema still mirrors v0.1 — the SQLite + markdown migration
// lands in Phase 1.1 (see docs/internal/ROADMAP.md).
type Entry struct {
	ID      string   `json:"id"`
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Tags    []string `json:"tags"`
	Agent   string   `json:"agent"`
	Created string   `json:"created"`
	Updated string   `json:"updated"`
	TTL     int      `json:"ttl"`
}

// Store owns the backing file and serialises access to it. All tool
// methods hang off Store ; the Dispatcher routes MCP calls into them.
type Store struct {
	dir        string
	maxEntries int
	mu         sync.Mutex
}

// NewStore constructs a Store rooted at dir. maxEntries <= 0 falls back
// to a 10_000-entry default.
func NewStore(dir string, maxEntries int) *Store {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &Store{dir: dir, maxEntries: maxEntries}
}

func (s *Store) filePath() string {
	return filepath.Join(s.dir, "memories.json")
}

func (s *Store) load() ([]Entry, error) {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) save(entries []Entry) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath())
}

func (s *Store) purgeExpired(entries []Entry) []Entry {
	now := time.Now()
	result := make([]Entry, 0, len(entries))
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

func hasTags(entry Entry, tags []string) bool {
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

// Result is the MCP tool-call content envelope returned by every tool
// method. Tool-level errors are carried inside this envelope with
// isError=true ; transport-level errors travel through the JSON-RPC
// error object instead.
type Result = map[string]any

// TextResult wraps plain text in the MCP content envelope.
func TextResult(text string) Result {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

// ErrResult wraps an error message in the MCP content envelope with
// isError set. It represents a tool-level failure, not a transport error.
func ErrResult(msg string) Result {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

// --- Tool methods ---

// ToolStore upserts a memory entry. If a key already exists it is
// updated in place ; otherwise a new entry is appended, subject to
// the maxEntries ceiling.
func (s *Store) ToolStore(args map[string]any) Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if key == "" || value == "" {
		return ErrResult("key and value are required")
	}

	tags := parseTags(args["tags"])
	agent, _ := args["agent"].(string)
	ttl := 0
	if v, ok := args["ttl"].(float64); ok {
		ttl = int(v)
	}

	entries, err := s.load()
	if err != nil {
		return ErrResult(fmt.Sprintf("failed to load memories: %v", err))
	}

	entries = s.purgeExpired(entries)
	now := time.Now().UTC().Format(time.RFC3339)

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
			return ErrResult(fmt.Sprintf("memory full: %d entries (max %d)", len(entries), s.maxEntries))
		}
		entries = append(entries, Entry{
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
		return ErrResult(fmt.Sprintf("failed to save: %v", err))
	}

	action := "created"
	if found {
		action = "updated"
	}
	return TextResult(fmt.Sprintf("Memory '%s' %s (%d total entries)", key, action, len(entries)))
}

// ToolRecall returns matching entries as a formatted text result.
func (s *Store) ToolRecall(args map[string]any) Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return ErrResult(fmt.Sprintf("failed to load memories: %v", err))
	}

	purged := s.purgeExpired(entries)
	if len(purged) < len(entries) {
		_ = s.save(purged)
	}
	entries = purged

	key, _ := args["key"].(string)
	tags := parseTags(args["tags"])
	agent, _ := args["agent"].(string)
	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var results []Entry
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
		return TextResult("No memories found.")
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

	return TextResult(sb.String())
}

// ToolList returns the list of keys (no values) optionally filtered by
// tags and agent.
func (s *Store) ToolList(args map[string]any) Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return ErrResult(fmt.Sprintf("failed to load memories: %v", err))
	}

	purged := s.purgeExpired(entries)
	if len(purged) < len(entries) {
		_ = s.save(purged)
	}
	entries = purged

	tags := parseTags(args["tags"])
	agent, _ := args["agent"].(string)

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
		return TextResult("No memories found.")
	}

	return TextResult(fmt.Sprintf("%d memories:\n%s", count, sb.String()))
}

// ToolForget deletes entries matching key and/or tags. At least one of
// the two filters must be provided.
func (s *Store) ToolForget(args map[string]any) Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, _ := args["key"].(string)
	tags := parseTags(args["tags"])

	if key == "" && len(tags) == 0 {
		return ErrResult("key or tags required")
	}

	entries, err := s.load()
	if err != nil {
		return ErrResult(fmt.Sprintf("failed to load memories: %v", err))
	}

	kept := make([]Entry, 0, len(entries))
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
		return TextResult("No matching memories found.")
	}

	if err := s.save(kept); err != nil {
		return ErrResult(fmt.Sprintf("failed to save: %v", err))
	}

	return TextResult(fmt.Sprintf("Removed %d memory(ies). %d remaining.", removed, len(kept)))
}
