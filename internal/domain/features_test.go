package domain

import (
	"testing"
)

func TestFeatureRegistry_Count(t *testing.T) {
	reg := FeatureRegistry()
	if len(reg) < 46 {
		t.Errorf("expected >= 46 features, got %d", len(reg))
	}
}

func TestFeatureRegistry_UniqueIDs(t *testing.T) {
	reg := FeatureRegistry()
	seen := make(map[string]bool, len(reg))
	for _, f := range reg {
		if seen[f.ID] {
			t.Errorf("duplicate feature ID: %q", f.ID)
		}
		seen[f.ID] = true
	}
}

func TestFeatureRegistry_ValidDimensions(t *testing.T) {
	reg := FeatureRegistry()
	valid := map[string]bool{
		"user": true, "agent": true, "environment": true,
		"relationship": true, "task": true,
	}
	for _, f := range reg {
		if !valid[f.Dimension] {
			t.Errorf("feature %q has invalid dimension %q", f.ID, f.Dimension)
		}
	}
}

func TestFeatureRegistry_ValidTiers(t *testing.T) {
	reg := FeatureRegistry()
	for _, f := range reg {
		if f.Tier < 1 || f.Tier > 3 {
			t.Errorf("feature %q has invalid tier %d", f.ID, f.Tier)
		}
	}
}

func TestFeatureRegistry_AllHaveLabels(t *testing.T) {
	reg := FeatureRegistry()
	for _, f := range reg {
		if f.Label == "" {
			t.Errorf("feature %q has empty label", f.ID)
		}
		if f.Description == "" {
			t.Errorf("feature %q has empty description", f.ID)
		}
	}
}

func TestQuantifiedFeatures_Defaults(t *testing.T) {
	f := &QuantifiedFeatures{}
	// All normalized values should be 0 for a zero-valued struct.
	if f.U3_IsWorking != 0 || f.U11_MealTime != 0 || f.U12_NightTime != 0 {
		t.Error("zero-valued features should have zero defaults")
	}
}
