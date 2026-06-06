package identity

import (
	"desktop-pet/internal/infra"
	"desktop-pet/internal/infra/storage"
)

func clamp01(v float64) float64 { return storage.Clamp01(v) }
func cleanJSON(raw string) string { return infra.CleanJSON(raw) }
func cosineSimilarity(a, b []float32) float64 { return storage.CosineSimilarity(a, b) }
