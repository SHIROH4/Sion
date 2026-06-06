package memory

import (
	"testing"
	"time"
)

func TestDecayWeight_Recent(t *testing.T) {
	w := DecayWeight(0.8, time.Now().Add(-24*time.Hour).Unix(), 2, 30, 0.15)
	if w < 0.5 {
		t.Errorf("recent fact should have high weight, got %f", w)
	}
}

func TestDecayWeight_Old(t *testing.T) {
	w := DecayWeight(0.5, time.Now().Add(-60*24*time.Hour).Unix(), 0, 30, 0.15)
	if w > 0.2 {
		t.Errorf("old fact should have low weight, got %f", w)
	}
}

func TestDecayWeight_RecallBoost(t *testing.T) {
	w1 := DecayWeight(0.5, time.Now().Add(-30*24*time.Hour).Unix(), 0, 30, 0.15)
	w2 := DecayWeight(0.5, time.Now().Add(-30*24*time.Hour).Unix(), 5, 30, 0.15)
	if w2 <= w1 {
		t.Errorf("recalled fact (%.4f) should have higher weight than non-recalled (%.4f)", w2, w1)
	}
}
