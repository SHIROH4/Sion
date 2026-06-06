package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"desktop-pet/internal/domain"
)

// MemoryHandlers holds dependencies for memory/diary API handlers.
type MemoryHandlers struct {
	ListFacts        func() []domain.FactEntry
	ListDiaries      func() []domain.DiaryEntry
	DeleteFact       func(id int64) error
	SelfModel        func() string
	ListSelfProfiles  func() []string // all self-profile entries
	ListL0           func() []domain.Message // session buffer recent messages
}

// ---- GET/POST /api/memories ----

type memoryItem struct {
	ID        string  `json:"id"`
	Layer     string  `json:"layer"`
	Content   string  `json:"content"`
	Weight    float64 `json:"weight"`
	CreatedAt string  `json:"created_at,omitempty"`
}

type memoryListResponse struct {
	Memories []memoryItem `json:"memories"`
	Total    int          `json:"total"`
}

func (h *Handlers) handleMemories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.Memory == nil {
		writeError(w, http.StatusNotImplemented, "memory not wired")
		return
	}

	layer := r.URL.Query().Get("layer") // L0|L1|L2|L3 or empty=all
	query := r.URL.Query().Get("query")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize := 20
	if ps := r.URL.Query().Get("pageSize"); ps != "" {
		if n, _ := strconv.Atoi(ps); n > 0 && n <= 100 {
			pageSize = n
		}
	}

	var items []memoryItem

	// L0: Session buffer (working memory)
	if (layer == "" || layer == "L0") && h.Memory.ListL0 != nil {
		msgs := h.Memory.ListL0()
		for _, m := range msgs {
			items = append(items, memoryItem{
				ID:      "l0-" + strconv.Itoa(len(items)),
				Layer:   "L0",
				Content: m.Role + ": " + m.Content,
				Weight:  0.5,
			})
		}
	}

	// L2: Facts
	if layer == "" || layer == "L2" {
		facts := h.Memory.ListFacts()
		for _, f := range facts {
			items = append(items, memoryItem{
				ID:      "fact-" + strconv.FormatInt(f.ID, 10),
				Layer:   "L2",
				Content: f.Content,
				Weight:  f.Importance,
			})
		}
	}

	// L1: Diaries
	if layer == "" || layer == "L1" {
		diaries := h.Memory.ListDiaries()
		for _, d := range diaries {
			items = append(items, memoryItem{
				ID:      "diary-" + strconv.FormatInt(d.ID, 10),
				Layer:   "L1",
				Content: d.Summary,
				Weight:  (d.EmotionValence + 1) / 2, // normalize to 0~1
			})
		}
	}

	// L3: Self model
	if layer == "" || layer == "L3" {
		profiles := h.Memory.ListSelfProfiles()
		for i, p := range profiles {
			items = append(items, memoryItem{
				ID:      fmt.Sprintf("self-%d", i),
				Layer:   "L3",
				Content: p,
				Weight:  1.0 - float64(i)*0.1,
			})
		}
	}

	// Filter by query
	if query != "" {
		filtered := make([]memoryItem, 0)
		for _, it := range items {
			if strings.Contains(it.Content, query) {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	total := len(items)

	// Paginate
	start := page * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	pageItems := items[start:end]
	if pageItems == nil {
		pageItems = []memoryItem{}
	}

	writeJSON(w, http.StatusOK, memoryListResponse{
		Memories: pageItems,
		Total:    total,
	})
}

// ---- GET /api/memories/{id} ----

func (h *Handlers) handleMemoryDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract ID from path: /api/memories/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/memories/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusBadRequest, "memory id required")
		return
	}

	// Look up by ID
	if h.Memory == nil {
		writeError(w, http.StatusNotImplemented, "memory not wired")
		return
	}

	if strings.HasPrefix(path, "fact-") {
		idStr := strings.TrimPrefix(path, "fact-")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		for _, f := range h.Memory.ListFacts() {
			if f.ID == id {
				writeJSON(w, http.StatusOK, memoryItem{
					ID:      "fact-" + strconv.FormatInt(f.ID, 10),
					Layer:   "L2",
					Content: f.Content,
					Weight:  f.Importance,
				})
				return
			}
		}
	}

	if strings.HasPrefix(path, "diary-") {
		idStr := strings.TrimPrefix(path, "diary-")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		for _, d := range h.Memory.ListDiaries() {
			if d.ID == id {
				writeJSON(w, http.StatusOK, memoryItem{
					ID:      "diary-" + strconv.FormatInt(d.ID, 10),
					Layer:   "L1",
					Content: d.Summary,
					Weight:  (d.EmotionValence + 1) / 2,
				})
				return
			}
		}
	}

	if path == "self-model" {
		writeJSON(w, http.StatusOK, memoryItem{
			ID:      "self-model",
			Layer:   "L3",
			Content: h.Memory.SelfModel(),
			Weight:  1.0,
		})
		return
	}

	writeError(w, http.StatusNotFound, "memory not found")
}

// ---- DELETE /api/memories/{id} ----

func (h *Handlers) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/memories/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusBadRequest, "memory id required")
		return
	}

	if h.Memory == nil || h.Memory.DeleteFact == nil {
		writeError(w, http.StatusNotImplemented, "memory delete not wired")
		return
	}

	if strings.HasPrefix(path, "fact-") {
		idStr := strings.TrimPrefix(path, "fact-")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if err := h.Memory.DeleteFact(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	writeError(w, http.StatusBadRequest, "only fact memories can be deleted")
}

// ---- GET /api/diaries ----

type diaryItem struct {
	ID           int64   `json:"id"`
	Content      string  `json:"content"`
	EmotionLabel string  `json:"emotion_label"`
	EmotionScore float64 `json:"emotion_score"`
	CreatedAt    string  `json:"created_at"`
}

type diaryListResponse struct {
	Diaries []diaryItem `json:"diaries"`
	Total   int         `json:"total"`
}

func (h *Handlers) handleDiaries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.Memory == nil {
		writeError(w, http.StatusNotImplemented, "diaries not wired")
		return
	}

	diaries := h.Memory.ListDiaries()

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize := 20
	if ps := r.URL.Query().Get("pageSize"); ps != "" {
		if n, _ := strconv.Atoi(ps); n > 0 && n <= 100 {
			pageSize = n
		}
	}

	total := len(diaries)
	start := page * pageSize
	if start > len(diaries) {
		start = len(diaries)
	}
	end := start + pageSize
	if end > len(diaries) {
		end = len(diaries)
	}
	pageDiaries := diaries[start:end]

	result := make([]diaryItem, 0, len(pageDiaries))
	for _, d := range pageDiaries {
		content := d.Summary
		if content == "" {
			content = d.Title
		}
		label := "neutral"
		if d.EmotionValence > 0.3 {
			label = "happy"
		} else if d.EmotionValence < -0.3 {
			label = "sad"
		}
		result = append(result, diaryItem{
			ID:           d.ID,
			Content:      content,
			EmotionLabel: label,
			EmotionScore: d.EmotionValence,
		})
	}

	writeJSON(w, http.StatusOK, diaryListResponse{
		Diaries: result,
		Total:   total,
	})
}
