package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"desktop-pet/internal/app/chat"
	"desktop-pet/internal/domain"
)

// ChatHandlers holds dependencies for chat-related API handlers.
type ChatHandlers struct {
	SendMessage func(text string) (string, error)
	LoadHistory func(limit int) []domain.Message
}

// ---- POST /api/chat/send ----

type chatSendRequest struct {
	Text string `json:"text"`
}

type chatSendResponse struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

func (h *Handlers) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.Chat == nil || h.Chat.SendMessage == nil {
		writeError(w, http.StatusNotImplemented, "chat not wired")
		return
	}

	// Limit request body to 64KB.
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

	var req chatSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large (max 64KB)")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	// Trim overly long messages (defense in depth).
	if len([]rune(req.Text)) > 8000 {
		req.Text = string([]rune(req.Text)[:8000])
	}

	content, err := h.Chat.SendMessage(req.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chatSendResponse{Content: content})
}

// ---- GET /api/chat/history ----

type chatHistoryResponse struct {
	Messages []chatMessageView `json:"messages"`
	Total    int               `json:"total"`
}

type chatMessageView struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Source    string `json:"source,omitempty"`
	Observed  bool   `json:"observed,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

func (h *Handlers) handleChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.Chat == nil || h.Chat.LoadHistory == nil {
		writeError(w, http.StatusNotImplemented, "chat history not wired")
		return
	}

	query := r.URL.Query()
	pageSize := 50
	if ps := query.Get("pageSize"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 && n <= 200 {
			pageSize = n
		}
	}
	page := 0
	if p := query.Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 {
			page = n
		}
	}
	searchQuery := query.Get("query")

	limit := pageSize * (page + 1)
	msgs := h.Chat.LoadHistory(limit)

	// Apply search filter
	if searchQuery != "" {
		filtered := make([]domain.Message, 0)
		for _, m := range msgs {
			if strings.Contains(m.Content, searchQuery) {
				filtered = append(filtered, m)
			}
		}
		msgs = filtered
	}

	total := len(msgs)

	// Apply pagination
	start := page * pageSize
	if start > len(msgs) {
		start = len(msgs)
	}
	end := start + pageSize
	if end > len(msgs) {
		end = len(msgs)
	}
	pageMsgs := msgs[start:end]

	result := make([]chatMessageView, 0, len(pageMsgs))
	for _, m := range pageMsgs {
		content := chat.FilterTimestamp(m.Content)
		if content == "" {
			continue
		}
		result = append(result, chatMessageView{
			Role:    m.Role,
			Content: content,
		})
	}

	writeJSON(w, http.StatusOK, chatHistoryResponse{
		Messages: result,
		Total:    total,
	})
}
