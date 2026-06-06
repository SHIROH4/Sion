package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/infra/config"
)

func TestGateway_ChatStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"世界\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test-model"}
	gw := NewGateway(cfg)

	var result string
	err := gw.ChatStream(context.Background(), []plugin.Message{{Role: "user", Content: "hi"}},
		func(chunk string) error { result += chunk; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result != "你好世界" {
		t.Errorf("expected '你好世界', got '%s'", result)
	}
}

func TestGateway_ChatStream_DONE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	err := gw.ChatStream(context.Background(), []plugin.Message{{Role: "user", Content: "hi"}},
		func(chunk string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestGateway_ChatStream_EmptyLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	var result string
	err := gw.ChatStream(context.Background(), []plugin.Message{{Role: "user", Content: "hi"}},
		func(chunk string) error { result += chunk; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got '%s'", result)
	}
}

func TestGateway_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	err := gw.ChatStream(context.Background(), []plugin.Message{{Role: "user", Content: "hi"}},
		func(chunk string) error { return nil })
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500, got: %v", err)
	}
}

func TestGateway_ChatSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"完整回答"}}]}`)
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	result, err := gw.ChatSync(context.Background(), []plugin.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if result != "完整回答" {
		t.Errorf("expected '完整回答', got '%s'", result)
	}
}

func TestGateway_ChatSync_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "unauthorized")
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "bad", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	_, err := gw.ChatSync(context.Background(), []plugin.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestGateway_GetEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"embedding":[0.1, 0.2, 0.3]}]}`)
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	emb, err := gw.GetEmbedding(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 floats, got %d", len(emb))
	}
	if emb[0] != 0.1 || emb[1] != 0.2 || emb[2] != 0.3 {
		t.Errorf("unexpected embedding values: %v", emb)
	}
}

func TestGateway_GetEmbedding_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: server.URL, LLMModel: "test"}
	gw := NewGateway(cfg)

	_, err := gw.GetEmbedding(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

func TestGateway_BaseURLTrimSlash(t *testing.T) {
	cfg := &config.GlobalConfig{LLMAPIKey: "test", LLMBaseURL: "https://api.deepseek.com/", LLMModel: "test"}
	gw := NewGateway(cfg)
	if gw.baseURL != "https://api.deepseek.com" {
		t.Errorf("expected trimmed baseURL, got '%s'", gw.baseURL)
	}
}

func TestNewGateway_Defaults(t *testing.T) {
	cfg := &config.GlobalConfig{LLMAPIKey: "sk-key", LLMBaseURL: "https://api.test.com", LLMModel: "test-model"}
	gw := NewGateway(cfg)

	if gw.apiKey != "sk-key" {
		t.Errorf("apiKey = %q", gw.apiKey)
	}
	if gw.baseURL != "https://api.test.com" {
		t.Errorf("baseURL = %q", gw.baseURL)
	}
	if gw.model != "test-model" {
		t.Errorf("model = %q", gw.model)
	}
	if gw.httpClient.Timeout == 0 {
		t.Error("httpClient timeout not set")
	}
}

// ---- toChatMessages tests ----

func TestToChatMessages_WithImages(t *testing.T) {
	msgs := []plugin.Message{
		{
			Role:    "user",
			Content: "这段报错是什么意思",
			Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
		},
	}

	result := toChatMessages(msgs)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	parts, ok := result[0].Content.([]contentPart)
	if !ok {
		t.Fatalf("Content must be []contentPart for messages with images, got %T", result[0].Content)
	}

	if len(parts) < 1 {
		t.Fatal("expected at least 1 content part")
	}

	if parts[0].Type != "text" {
		t.Errorf("first part type = %q, want text", parts[0].Type)
	}
	if parts[0].Text != "这段报错是什么意思" {
		t.Errorf("first part text = %q", parts[0].Text)
	}

	if len(parts) < 2 {
		t.Fatal("expected image part")
	}
	if parts[1].Type != "image_url" {
		t.Errorf("second part type = %q, want image_url", parts[1].Type)
	}
	if parts[1].ImageURL == nil {
		t.Fatal("ImageURL must not be nil")
	}
	if parts[1].ImageURL.Detail != "auto" {
		t.Errorf("ImageURL.Detail = %q, want auto", parts[1].ImageURL.Detail)
	}
}

func TestToChatMessages_PlainTextBackwardCompat(t *testing.T) {
	msgs := []plugin.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	result := toChatMessages(msgs)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	for i, cm := range result {
		content, ok := cm.Content.(string)
		if !ok {
			t.Errorf("message %d: Content must be string for plain text, got %T", i, cm.Content)
		}
		if content != msgs[i].Content {
			t.Errorf("message %d: content = %q, want %q", i, content, msgs[i].Content)
		}
	}
}

func TestToChatMessages_MixedContent(t *testing.T) {
	msgs := []plugin.Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "user",
			Content: "检查这个截图",
			Images:  []plugin.Image{{Base64: "xyz", Format: "jpeg"}},
		},
	}

	result := toChatMessages(msgs)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	if _, ok := result[0].Content.(string); !ok {
		t.Errorf("message 0: Content must be string, got %T", result[0].Content)
	}

	parts, ok := result[1].Content.([]contentPart)
	if !ok {
		t.Fatalf("message 1: Content must be []contentPart, got %T", result[1].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("message 1: expected 2 parts, got %d", len(parts))
	}
}

func TestContentPart_Format(t *testing.T) {
	msgs := []plugin.Message{
		{
			Role:    "user",
			Content: "",
			Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
		},
	}

	result := toChatMessages(msgs)
	parts := result[0].Content.([]contentPart)

	if len(parts) != 1 {
		t.Fatalf("expected 1 part (image only), got %d", len(parts))
	}

	expectedPrefix := "data:image/png;base64,"
	url := parts[0].ImageURL.URL
	if !strings.HasPrefix(url, expectedPrefix) {
		t.Errorf("URL = %q, must start with %q", url, expectedPrefix)
	}
	if !strings.Contains(url, "abc123") {
		t.Errorf("URL must contain base64 data, got %q", url)
	}
}

func TestToChatMessages_ImageOnlyNoText(t *testing.T) {
	msgs := []plugin.Message{
		{
			Role:   "user",
			Images: []plugin.Image{{Base64: "imgdata", Format: "png"}},
		},
	}

	result := toChatMessages(msgs)
	parts := result[0].Content.([]contentPart)

	if len(parts) != 1 {
		t.Fatalf("expected 1 part (image only), got %d", len(parts))
	}
	if parts[0].Type != "image_url" {
		t.Errorf("part type = %q, want image_url", parts[0].Type)
	}
}
