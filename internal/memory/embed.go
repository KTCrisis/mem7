package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type embedder struct {
	url      string
	model    string
	provider string
	key      string
	client   *http.Client
}

func newEmbedder(url, model, provider, key string) *embedder {
	if provider == "" {
		provider = "ollama"
	}
	return &embedder{
		url:      url,
		model:    model,
		provider: provider,
		key:      key,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *embedder) Embed(text string) ([]float32, error) {
	if e.provider == "openai" {
		return e.embedOpenAI(text)
	}
	return e.embedOllama(text)
}

func (e *embedder) embedOllama(text string) ([]float32, error) {
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

func (e *embedder) embedOpenAI(text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": e.model,
		"input": text,
	})
	req, err := http.NewRequest("POST", e.url+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.key != "" {
		req.Header.Set("Authorization", "Bearer "+e.key)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embed decode: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embed: no embeddings returned")
	}
	return result.Data[0].Embedding, nil
}
