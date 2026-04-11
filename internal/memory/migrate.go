package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// v1LegacyEntry mirrors the v0.1 memories.json row shape so we can
// import old data into the v0.2 layout without keeping the old Store
// around for compatibility.
type v1LegacyEntry struct {
	ID      string   `json:"id"`
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Tags    []string `json:"tags"`
	Agent   string   `json:"agent"`
	Created string   `json:"created"`
	Updated string   `json:"updated"`
	TTL     int      `json:"ttl"`
}

// MigrateV1 imports a v0.1 flat JSON file into the v0.2 markdown +
// SQLite layout. It is idempotent in the sense that once the legacy
// file has been renamed to memories.json.v0.1.bak, further calls are
// no-ops. Returns the number of entries imported.
//
// The function is non-fatal : if the file does not exist, it returns
// (0, nil). On partial failure it returns the count imported so far
// along with the error, so the caller can log progress.
func MigrateV1(s *Store) (int, error) {
	legacy := filepath.Join(s.dir, "memories.json")
	data, err := os.ReadFile(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read legacy file: %w", err)
	}

	var entries []v1LegacyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, fmt.Errorf("parse legacy file: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	imported := 0
	for _, e := range entries {
		if e.Key == "" || e.Value == "" {
			continue
		}
		created := parseLegacyTime(e.Created)
		updated := parseLegacyTime(e.Updated)
		if updated.IsZero() {
			updated = created
		}

		f := fact{
			Entity:    e.Key,
			Predicate: defaultPredicate,
			Object:    e.Value,
			Tags:      e.Tags,
			Agent:     e.Agent,
			TTL:       e.TTL,
			Created:   created,
			Updated:   updated,
		}

		path, line, err := s.md.AppendStore(f)
		if err != nil {
			return imported, fmt.Errorf("write markdown for %q: %w", e.Key, err)
		}
		f.SourceFile = path
		f.SourceLine = line
		if _, err := s.index.Put(f); err != nil {
			return imported, fmt.Errorf("index put for %q: %w", e.Key, err)
		}
		imported++
	}

	bak := legacy + ".v0.1.bak"
	if err := os.Rename(legacy, bak); err != nil {
		return imported, fmt.Errorf("rename legacy file: %w", err)
	}
	return imported, nil
}

func parseLegacyTime(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}
