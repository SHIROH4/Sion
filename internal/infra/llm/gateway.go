package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/infra/config"
)

// Gateway is an HTTP client for OpenAI-compatible chat APIs.
type Gateway struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	model      string
	TokenCount *int // shared token counter across gateways
}

// NewGateway returns an initialized Gateway from the global config.
func cleanBaseURL(raw string) string {
	s := strings.TrimSuffix(raw, "/")
	s = strings.TrimSuffix(s, "/v1")
	return s
}

func NewGateway(cfg *config.GlobalConfig) *Gateway {
	return &Gateway{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		apiKey:     cfg.LLMAPIKey,
		baseURL:    cleanBaseURL(cfg.LLMBaseURL),
		model:      cfg.LLMModel,
	}
}

// NewVisionGateway returns a Gateway for vision/multimodal analysis.
// Falls back to the main LLM config when vision-specific settings are empty.
func NewVisionGateway(cfg *config.GlobalConfig) *Gateway {
	model := cfg.VisionModel
	apiKey := cfg.VisionAPIKey
	baseURL := cfg.VisionBaseURL
	if model == "" {
		model = cfg.LLMModel
	}
	if apiKey == "" {
		apiKey = cfg.LLMAPIKey
	}
	if baseURL == "" {
		baseURL = cfg.LLMBaseURL
	}
	return &Gateway{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		apiKey:     apiKey,
		baseURL:    cleanBaseURL(baseURL),
		model:      model,
	}
}

// ---- request/response types ----

type chatMessage struct {
	Role       string            `json:"role"`
	Content    any               `json:"content"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolCalls  []plugin.ToolCall `json:"tool_calls,omitempty"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Tool represents an OpenAI-compatible function tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction holds the JSON Schema for a single callable function.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []Tool        `json:"tools,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string            `json:"content"`
			ToolCalls []plugin.ToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type syncResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// ---- helpers ----

func toChatMessages(msgs []plugin.Message) []chatMessage {
	out := make([]chatMessage, len(msgs))
	for i, m := range msgs {
		if len(m.Images) > 0 {
			parts := []contentPart{}
			if m.Content != "" {
				parts = append(parts, contentPart{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, img := range m.Images {
				url := fmt.Sprintf("data:image/%s;base64,%s", img.Format, img.Base64)
				parts = append(parts, contentPart{
					Type:     "image_url",
					ImageURL: &imageURL{URL: url, Detail: "auto"},
				})
			}
			out[i] = chatMessage{
				Role:       m.Role,
				Content:    parts,
				ToolCallID: m.ToolCallID,
				ToolCalls:  m.ToolCalls,
			}
		} else {
			out[i] = chatMessage{
				Role:       m.Role,
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
				ToolCalls:  m.ToolCalls,
			}
		}
	}
	return out
}

// BuildTools converts registered functions into OpenAI-compatible Tool definitions.
func BuildTools(entries []plugin.FunctionEntry) []Tool {
	tools := make([]Tool, 0, len(entries))
	for _, e := range entries {
		params := e.Parameters
		if params == nil {
			params = defaultToolParams(e.Description)
		}
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        e.Name,
				Description: e.Description,
				Parameters:  params,
			},
		})
	}
	return tools
}

func defaultToolParams(description string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": description,
			},
		},
		"required": []string{"query"},
	}
}

func (g *Gateway) doRequest(ctx context.Context, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
					io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("llm api returned %d", resp.StatusCode)
	}
	return resp, nil
}

// ChatStream performs a streaming chat completion, calling callback with each content delta.
func (g *Gateway) ChatStream(
	ctx context.Context,
	messages []plugin.Message,
	callback func(chunk string) error,
) error {
	reqBody := chatRequest{
		Model:    g.model,
		Messages: toChatMessages(messages),
		Stream:   true,
	}

	resp, err := g.doRequest(ctx, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 1*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if err := callback(c.Delta.Content); err != nil {
					return err
				}
			}
		}
	}

	return scanner.Err()
}

// ChatStreamWithTools performs a streaming chat completion with function tool definitions.
func (g *Gateway) ChatStreamWithTools(
	ctx context.Context,
	messages []plugin.Message,
	tools []Tool,
	onContent func(chunk string) error,
	onToolCalls func(calls []plugin.ToolCall) error,
) error {
	reqBody := chatRequest{
		Model:    g.model,
		Messages: toChatMessages(messages),
		Stream:   true,
		Tools:    tools,
	}

	resp, err := g.doRequest(ctx, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 1*1024*1024)

	acc := make(map[int]*plugin.ToolCall)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if err := onContent(c.Delta.Content); err != nil {
					return err
				}
			}

			for _, tc := range c.Delta.ToolCalls {
				if existing, ok := acc[tc.Index]; ok {
					existing.Function.Arguments += tc.Function.Arguments
					if tc.Function.Name != "" {
						existing.Function.Name = tc.Function.Name
					}
					if tc.ID != "" {
						existing.ID = tc.ID
					}
				} else {
					cp := tc
					acc[tc.Index] = &cp
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if len(acc) > 0 && onToolCalls != nil {
		calls := make([]plugin.ToolCall, 0, len(acc))
		for i := 0; i < len(acc); i++ {
			if tc, ok := acc[i]; ok {
				calls = append(calls, *tc)
			}
		}
		if len(calls) > 0 {
			return onToolCalls(calls)
		}
	}

	return nil
}

// ChatSync performs a non-streaming chat completion and returns the full response.
func (g *Gateway) ChatSync(ctx context.Context, messages []plugin.Message) (string, error) {
	reqBody := chatRequest{
		Model:    g.model,
		Messages: toChatMessages(messages),
		Stream:   false,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
					io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("llm api returned %d", resp.StatusCode)
	}

	var result syncResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// GetEmbedding returns the embedding vector for the given text.
func (g *Gateway) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody := embeddingRequest{
		Model: "text-embedding-3-small",
		Input: text,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
					io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("llm api returned %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding data in response")
	}

	return result.Data[0].Embedding, nil
}
