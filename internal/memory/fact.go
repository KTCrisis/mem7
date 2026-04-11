package memory

import (
	"time"
)

// Fact is the internal row shape shared by the markdown writer and the
// SQLite index. It is intentionally unexported : the Dispatcher talks
// to the outside world in terms of map[string]any (MCP arguments) and
// the Store converts between the two.
//
// The schema mirrors the roadmap's v0.2.0 facts table. For v0.2.0 the
// tool API is still key/value, so every fact has predicate="note" ;
// richer predicates land with the v2 temporal graph (Phase 4).
type fact struct {
	ID         int64
	Entity     string
	Predicate  string
	Object     string
	Tags       []string
	Agent      string
	TTL        int
	SourceFile string
	SourceLine int
	Created    time.Time
	Updated    time.Time
	Deleted    *time.Time
}

// filter is the query shape accepted by the storage layer. All fields
// are optional ; an empty filter matches every live row.
type filter struct {
	Entity string
	Tags   []string
	Agent  string
	Limit  int
}

// searchQuery carries the parameters of a full-text search. Query is
// passed through to FTS5 MATCH ; the rest act as post-filters.
type searchQuery struct {
	Query string
	Tags  []string
	Agent string
	Since time.Time
	Until time.Time
	Limit int
}

// defaultPredicate is what the v0.2.0 key/value tool API writes into
// the predicate column. Phase 4 will introduce richer predicates.
const defaultPredicate = "note"
