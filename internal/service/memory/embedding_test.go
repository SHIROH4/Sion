package memory

import (
	"testing"
)

func TestEmbeddingService_Vectorize(t *testing.T) {
	svc := NewEmbeddingService("http://localhost:11434", "hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf")
	vec, err := svc.Vectorize("主人喜欢玩王者荣耀")
	if err != nil {
		t.Skip("Ollama not running: " + err.Error())
	}
	if len(vec) == 0 {
		t.Error("expected non-empty vector")
	}
	if len(vec) != 512 {
		t.Errorf("expected 512 dimensions, got %d", len(vec))
	}
}

func TestEmbeddingService_EmptyText(t *testing.T) {
	svc := NewEmbeddingService("http://localhost:11434", "hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf")
	vec, err := svc.Vectorize("")
	if err != nil {
		t.Log("Ollama returned error for empty text (acceptable):", err)
		return
	}
	// If Ollama returns a vector for empty text, it should not panic.
	t.Log("empty text vector length:", len(vec))
}

func TestEmbeddingService_InvalidURL(t *testing.T) {
	svc := NewEmbeddingService("http://127.0.0.1:19999", "hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf")
	_, err := svc.Vectorize("test")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}
