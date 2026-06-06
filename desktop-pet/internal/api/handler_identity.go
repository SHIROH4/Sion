package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"desktop-pet/internal/domain"
)

// IdentityHandlers holds dependencies for identity API handlers.
type IdentityHandlers struct {
	ListAll    func() []domain.IdentityNode
	Upsert     func(node *domain.IdentityNode) error
	SelfUpdate func() error
}

type identityNodeView struct {
	ID        int64   `json:"id"`
	Category  string  `json:"category"`
	Content   string  `json:"content"`
	Weight    float64 `json:"weight"`
	UpdatedAt int64   `json:"updated_at"`
}

func nodeToView(n domain.IdentityNode) identityNodeView {
	return identityNodeView{
		ID:        n.ID,
		Category:  string(n.Type),
		Content:   n.Content,
		Weight:    n.Confidence,
		UpdatedAt: n.UpdatedAt,
	}
}

func (h *Handlers) handleIdentity(w http.ResponseWriter, r *http.Request) {
	if h.Identity == nil {
		writeError(w, http.StatusNotImplemented, "identity not wired")
		return
	}

	switch r.Method {
	case http.MethodGet:
		nodes := h.Identity.ListAll()
		views := make([]identityNodeView, 0, len(nodes))
		for _, n := range nodes {
			views = append(views, nodeToView(n))
		}
		writeJSON(w, http.StatusOK, views)

	case http.MethodPut:
		// PUT /api/identity/{id}
		path := strings.TrimPrefix(r.URL.Path, "/api/identity/")
		if path == "" || path == r.URL.Path {
			writeError(w, http.StatusBadRequest, "node id required")
			return
		}
		id, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		var body struct {
			Content  string `json:"content"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		node := &domain.IdentityNode{
			ID:      id,
			Type:    domain.IdentityNodeType(body.Category),
			Content: body.Content,
			Active:  true,
		}
		if err := h.Identity.Upsert(node); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case http.MethodPost:
		// POST /api/identity → create new node
		var body struct {
			Content  string `json:"content"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		node := &domain.IdentityNode{
			Type:    domain.IdentityNodeType(body.Category),
			Content: body.Content,
			Active:  true,
		}
		if err := h.Identity.Upsert(node); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handlers) handleIdentitySelfUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.Identity == nil || h.Identity.SelfUpdate == nil {
		writeError(w, http.StatusNotImplemented, "self-update not wired")
		return
	}
	if err := h.Identity.SelfUpdate(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "self-update triggered"})
}
