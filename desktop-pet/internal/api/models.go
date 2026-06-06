package api

import (
	"net/http"
	"os"
	"path/filepath"
)

// ModelInfo describes an available Live2D model.
type ModelInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// scanModels discovers Live2D models under frontend/public/model/.
func scanModels() []ModelInfo {
	modelDir := filepath.Join("frontend", "public", "model")
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		// Fallback for production build (assets embedded).
		entries, err = os.ReadDir(filepath.Join("..", "frontend", "public", "model"))
		if err != nil {
			return nil
		}
	}
	var models []ModelInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Check for .model3.json inside the directory.
		modelFile := filepath.Join(modelDir, name, name+".model3.json")
		if _, err := os.Stat(modelFile); err != nil {
			// Try alternate path.
			modelFile = filepath.Join("..", "frontend", "public", "model", name, name+".model3.json")
			if _, err := os.Stat(modelFile); err != nil {
				continue
			}
		}
		models = append(models, ModelInfo{
			Name: name,
			Path: "/model/" + name + "/" + name + ".model3.json",
		})
	}
	return models
}

func (h *Handlers) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	models := scanModels()
	if models == nil {
		models = []ModelInfo{}
	}
	writeJSON(w, http.StatusOK, models)
}

func (h *Handlers) handleProactivePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	msg := ""
	if h.ProactivePoll != nil {
		msg = h.ProactivePoll()
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}
