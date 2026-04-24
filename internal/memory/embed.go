package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type embedder struct {
	url    string
	model  string
	client *http.Client
}

func newEmbedder(url, model string) *embedder {
	return &embedder{
		url:    url,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *embedder) Embed(text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": e.model,
		"input": text,
	})
	resp, err := e.client.Post(e.url+"/api/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: status %d", resp.StatusCode)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embed decode: %w", err)
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("embed: no embeddings returned")
	}
	return result.Embeddings[0], nil
}
