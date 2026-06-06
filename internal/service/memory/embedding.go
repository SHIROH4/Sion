package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Vectorizer is the interface for text-to-vector embedding.
type Vectorizer interface {
	Vectorize(text string) ([]float32, error)
}

// EmbeddingService wraps a local Ollama embedding model.
type EmbeddingService struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewEmbeddingService creates an embedding client for the given Ollama endpoint.
func NewEmbeddingService(baseURL, model string) *EmbeddingService {
	return &EmbeddingService{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Vectorize returns the embedding vector for the given text, with one retry.
func (e *EmbeddingService) Vectorize(text string) ([]float32, error) {
	vec, err := e.vectorizeOnce(text)
	if err == nil {
		return vec, nil
	}
	// One retry after a short delay.
	time.Sleep(500 * time.Millisecond)
	return e.vectorizeOnce(text)
}

func (e *EmbeddingService) vectorizeOnce(text string) ([]float32, error) {
	body := map[string]any{
		"model":  e.model,
		"prompt": text,
	}
	b, _ := json.Marshal(body)
	resp, err := e.client.Post(
		e.baseURL+"/api/embeddings",
		"application/json",
		bytes.NewReader(b),
	)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding decode: %w", err)
	}
	return result.Embedding, nil
}

// Health checks whether the Ollama embedding endpoint is reachable.
func (e *EmbeddingService) Health() error {
	_, err := e.vectorizeOnce("ping")
	return err
}
