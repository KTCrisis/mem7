package memory

import (
	"encoding/binary"
	"math"
	"sort"
)

type scoredID struct {
	ID    int64
	Score float64
}

func cosineSearch(query []float32, embeddings map[int64][]float32, limit int) []scoredID {
	qNorm := vecNorm(query)
	if qNorm == 0 {
		return nil
	}
	results := make([]scoredID, 0, len(embeddings))
	for id, vec := range embeddings {
		vNorm := vecNorm(vec)
		if vNorm == 0 {
			continue
		}
		sim := dotProduct(query, vec) / (qNorm * vNorm)
		results = append(results, scoredID{ID: id, Score: sim})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func mergeRRF(bm25Results []fact, cosineResults []scoredID, factMap map[int64]fact, limit int) []fact {
	const k = 60
	scores := make(map[int64]float64)
	for rank, f := range bm25Results {
		scores[f.ID] += 1.0 / float64(k+rank+1)
	}
	for rank, s := range cosineResults {
		scores[s.ID] += 1.0 / float64(k+rank+1)
	}

	type entry struct {
		id    int64
		score float64
	}
	entries := make([]entry, 0, len(scores))
	for id, score := range scores {
		entries = append(entries, entry{id, score})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	out := make([]fact, 0, len(entries))
	for _, e := range entries {
		if f, ok := factMap[e.id]; ok {
			out = append(out, f)
		}
	}
	return out
}

func dotProduct(a, b []float32) float64 {
	sum := 0.0
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func vecNorm(v []float32) float64 {
	sum := 0.0
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	return math.Sqrt(sum)
}

func float32ToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return buf
}

func bytesToFloat32(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		v[i] = math.Float32frombits(bits)
	}
	return v
}
