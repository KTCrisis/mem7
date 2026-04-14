package memory

import (
	"regexp"
	"strings"
)

// naturalizeFTSQuery converts a natural-language question into an FTS5
// query that an agent can sensibly hand to memory_search without knowing
// FTS5 quirks (default AND, no stemming, literal stop words).
//
// Pipeline :
//  1. Lowercase + extract alphanumeric tokens
//  2. Drop English stop words and tokens shorter than 3 chars
//  3. Deduplicate preserving order
//  4. Append a trailing "*" for tokens >= 4 chars (cheap stemming)
//  5. Join with " OR " so FTS5 returns anything that matches at least one
//     meaningful keyword ; BM25 then ranks by the number of hits and
//     proximity
//
// If the pipeline produces no keywords (pathological short input), the
// original query is returned unchanged so the caller still gets a meaningful
// error from FTS5 rather than silent emptiness.
//
// Agents that already know how to write FTS5 queries can opt out by passing
// mode="raw" (the default). This function is only invoked when the caller
// asks for mode="natural".
func naturalizeFTSQuery(q string) string {
	tokens := tokenRe.FindAllString(strings.ToLower(q), -1)
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, ok := stopWords[t]; ok {
			continue
		}
		if len(t) < 3 {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		// Wildcard stemming : paint* matches paint, painted, painting.
		// Skip for very short tokens to avoid matching anything.
		if len(t) >= 4 {
			t = t + "*"
		}
		out = append(out, t)
		// Hard cap to avoid over-broadening on long questions.
		if len(out) >= 12 {
			break
		}
	}
	if len(out) == 0 {
		return q
	}
	return strings.Join(out, " OR ")
}

var tokenRe = regexp.MustCompile(`[a-zA-Z0-9]+`)

// stopWords is a minimal English stop-word list. Conservative by design :
// it strips only words that almost never carry retrieval signal. Domain
// vocabulary, proper nouns, verbs with content are kept.
var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"do": {}, "does": {}, "did": {}, "doing": {}, "done": {},
	"have": {}, "has": {}, "had": {}, "having": {},
	"what": {}, "when": {}, "where": {}, "who": {}, "whom": {}, "whose": {},
	"why": {}, "how": {}, "which": {},
	"this": {}, "that": {}, "these": {}, "those": {},
	"i": {}, "you": {}, "he": {}, "she": {}, "we": {}, "they": {}, "it": {},
	"me": {}, "him": {}, "her": {}, "us": {}, "them": {},
	"my": {}, "your": {}, "his": {}, "our": {}, "their": {}, "its": {},
	"to": {}, "from": {}, "of": {}, "in": {}, "on": {}, "at": {}, "by": {},
	"for": {}, "with": {}, "about": {}, "as": {}, "into": {}, "through": {},
	"and": {}, "or": {}, "but": {}, "not": {}, "no": {}, "nor": {}, "so": {}, "yet": {},
	"if": {}, "then": {}, "than": {}, "because": {},
	"will": {}, "would": {}, "could": {}, "should": {}, "may": {}, "might": {}, "can": {},
	"get": {}, "got": {}, "go": {}, "went": {}, "gone": {},
	"come": {}, "came": {}, "make": {}, "made": {}, "take": {}, "took": {},
	"say": {}, "said": {}, "says": {},
}
