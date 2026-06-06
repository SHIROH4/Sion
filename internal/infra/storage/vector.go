package storage

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"
)

// ActiveThreshold is the minimum weight for a fact to be considered "active".
const ActiveThreshold = 0.15

// CoreThreshold is the minimum weight for a fact to be considered "core memory".
const CoreThreshold = 0.6

// DefaultHalfLifeDays is the default Ebbinghaus half-life for fact decay (30 days).
const DefaultHalfLifeDays = 30.0

// DefaultBoostPerRecall is the default boost per recall for fact weight calculation.
const DefaultBoostPerRecall = 0.15

// DecayWeight calculates the current weight of a fact using a simplified
// Ebbinghaus forgetting curve.
func DecayWeight(importance float64, lastRecalledAt int64, recallCount int, halfLifeDays float64, boostPerRecall float64) float64 {
	if halfLifeDays <= 0 {
		halfLifeDays = DefaultHalfLifeDays
	}
	if boostPerRecall <= 0 {
		boostPerRecall = DefaultBoostPerRecall
	}

	daysSinceRecall := float64(time.Now().Unix()-lastRecalledAt) / 86400.0
	if daysSinceRecall < 0 {
		daysSinceRecall = 0
	}

	timeDecay := math.Exp(-daysSinceRecall / halfLifeDays)
	recallBoost := 1.0 + float64(recallCount)*boostPerRecall
	if recallBoost > 5.0 {
		recallBoost = 5.0
	}
	return importance * timeDecay * recallBoost
}

// CosineSimilarity returns the cosine similarity between two float32 vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// cosSim is a convenience alias for callers within the storage package.
func cosSim(a, b []float32) float64 { return CosineSimilarity(a, b) }

// EncodeVector encodes a float32 slice as little-endian binary for SQLite BLOB storage.
func EncodeVector(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, v)
	return buf.Bytes()
}

// DecodeVector decodes a little-endian binary BLOB back to a float32 slice.
// Returns nil for empty input or non-multiple-of-4 byte slices.
func DecodeVector(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	buf := bytes.NewReader(b)
	vec := make([]float32, len(b)/4)
	binary.Read(buf, binary.LittleEndian, &vec)
	return vec
}

// Clamp01 clamps a float64 to [0, 1].
func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
