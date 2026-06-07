package cognition

import (
	"strings"
	"testing"
)

// ---- Structural integrity ----

func TestAllActions_NoDuplicateNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range AllActions() {
		if seen[a.Name] {
			t.Errorf("duplicate action name: %q", a.Name)
		}
		seen[a.Name] = true
	}
}

func TestAllActions_NotEmpty(t *testing.T) {
	if len(AllActions()) == 0 {
		t.Fatal("AllActions must return at least one action")
	}
}

func TestAllActions_AllHaveRequiredFields(t *testing.T) {
	for _, a := range AllActions() {
		if a.Name == "" {
			t.Error("action has empty Name")
		}
		if a.DisplayName == "" {
			t.Errorf("%s: DisplayName is empty", a.Name)
		}
		if a.Category == "" {
			t.Errorf("%s: Category is empty", a.Name)
		}
		if a.Description == "" {
			t.Errorf("%s: Description is empty", a.Name)
		}
		if a.SkillCard == "" {
			t.Errorf("%s: SkillCard is empty", a.Name)
		}
		if a.OutcomeType == "" && a.Name != "none" {
			t.Errorf("%s: OutcomeType is empty", a.Name)
		}
	}
}

func TestAllActions_ValidCategories(t *testing.T) {
	valid := map[string]bool{"social": true, "care": true, "learning": true, "none": true}
	for _, a := range AllActions() {
		if !valid[a.Category] {
			t.Errorf("%s: invalid Category %q", a.Name, a.Category)
		}
	}
}

// ---- NeedsTool → ToolName consistency ----

func TestAllActions_ToolActionsHaveToolName(t *testing.T) {
	for _, a := range AllActions() {
		if a.NeedsTool && a.ToolName == "" {
			t.Errorf("%s: NeedsTool=true but ToolName is empty", a.Name)
		}
	}
}

func TestAllActions_ToolNameOnlyWhenNeedsTool(t *testing.T) {
	for _, a := range AllActions() {
		if !a.NeedsTool && a.ToolName != "" {
			t.Errorf("%s: NeedsTool=false but ToolName=%q (should be empty)", a.Name, a.ToolName)
		}
	}
}

// ---- NightSafe consistency ----

func TestAllActions_NightSafeActionsAreLearningOrCare(t *testing.T) {
	for _, a := range AllActions() {
		if a.NightSafe && a.Category != "learning" && a.Category != "care" && a.Category != "none" {
			t.Errorf("%s: NightSafe=true but Category=%q (expected learning/care/none)", a.Name, a.Category)
		}
	}
}

func TestBuildNightActions_AllNightSafe(t *testing.T) {
	night := BuildNightActions()
	for _, a := range AllActions() {
		if a.NightSafe && !night[a.Name] {
			t.Errorf("%s: NightSafe=true but not in BuildNightActions()", a.Name)
		}
		if !a.NightSafe && night[a.Name] {
			t.Errorf("%s: NightSafe=false but present in BuildNightActions()", a.Name)
		}
	}
}

// ---- Weights ----

func TestBuildWeightsMap_AllActionsPresent(t *testing.T) {
	wm := BuildWeightsMap()
	for _, a := range AllActions() {
		if _, ok := wm[a.Name]; !ok {
			t.Errorf("%s: missing from BuildWeightsMap()", a.Name)
		}
	}
	if len(wm) != len(AllActions()) {
		t.Errorf("BuildWeightsMap has %d entries, but AllActions has %d", len(wm), len(AllActions()))
	}
}

func TestAllActions_WeightsInRange(t *testing.T) {
	for _, a := range AllActions() {
		w := a.Weights
		for _, pair := range []struct {
			name  string
			value float64
		}{
			{"Social", w.Social},
			{"Care", w.Care},
			{"Curious", w.Curious},
			{"Quiet", w.Quiet},
			{"Explore", w.Explore},
		} {
			if pair.value < -1.0 || pair.value > 1.0 {
				t.Errorf("%s.%s = %.2f, out of [-1, 1]", a.Name, pair.name, pair.value)
			}
		}
	}
}

// ---- ActionByName ----

func TestActionByName_Found(t *testing.T) {
	for _, a := range AllActions() {
		found := ActionByName(a.Name)
		if found == nil {
			t.Errorf("ActionByName(%q) returned nil", a.Name)
			continue
		}
		if found.Name != a.Name {
			t.Errorf("ActionByName(%q).Name = %q", a.Name, found.Name)
		}
	}
}

func TestActionByName_NotFound(t *testing.T) {
	if found := ActionByName("nonexistent_action"); found != nil {
		t.Errorf("ActionByName for unknown action should return nil, got %v", found)
	}
}

// ---- Outcome type and source ----

func TestAllActions_OutcomeTypeIsValid(t *testing.T) {
	// Every OutcomeType should be one of the values expected by A7_ActionSuccessRate.
	seen := make(map[string]int)
	for _, a := range AllActions() {
		seen[a.OutcomeType]++
	}
	// At minimum we need "social", "rest", "meal", "encourage", etc.
	expected := []string{"social", "rest", "meal", "hydration", "health", "encourage",
		"search", "observe", "reflect", "analyze"}
	for _, e := range expected {
		if seen[e] == 0 {
			t.Errorf("OutcomeType %q not used by any action", e)
		}
	}
}

func TestAllActions_SourceIsValid(t *testing.T) {
	valid := map[string]bool{"care": true, "casual": true, "knowledge_gap": true, "": true}
	for _, a := range AllActions() {
		if !valid[a.Source] {
			t.Errorf("%s: invalid Source %q", a.Name, a.Source)
		}
	}
}

// ---- Skill cards ----

func TestBuildDecisionSkills_ContainsAllActions(t *testing.T) {
	skills := BuildDecisionSkills()
	for _, a := range AllActions() {
		if !strings.Contains(skills, a.Name) {
			t.Errorf("BuildDecisionSkills missing action: %s", a.Name)
		}
	}
}

func TestBuildDecisionSkills_HasOutputFormat(t *testing.T) {
	skills := BuildDecisionSkills()
	if !strings.Contains(skills, "输出JSON") {
		t.Error("BuildDecisionSkills should include JSON output format instruction")
	}
	if !strings.Contains(skills, "should_act") {
		t.Error("BuildDecisionSkills should mention should_act")
	}
	if !strings.Contains(skills, "tool_input") {
		t.Error("BuildDecisionSkills should mention tool_input")
	}
}

// ---- isSpeakAction ----

func TestIsSpeakAction_TrueForSocialAndCare(t *testing.T) {
	for _, a := range AllActions() {
		expected := a.Category == "social" || a.Category == "care"
		if got := isSpeakAction(a.Name); got != expected {
			t.Errorf("isSpeakAction(%q) = %v, want %v (Category=%s)", a.Name, got, expected, a.Category)
		}
	}
}

func TestIsSpeakAction_UnknownReturnsFalse(t *testing.T) {
	if isSpeakAction("made_up_action") {
		t.Error("isSpeakAction for unknown action should return false")
	}
}

// ---- Caching ----

func TestAllActions_Cached(t *testing.T) {
	a1 := AllActions()
	a2 := AllActions()
	// Same underlying slice pointer after caching.
	if len(a1) > 0 && &a1[0] != &a2[0] {
		t.Error("AllActions should return the same cached slice on subsequent calls")
	}
}

// ---- DisplayName uniqueness ----

func TestAllActions_UniqueDisplayNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range AllActions() {
		if seen[a.DisplayName] {
			t.Errorf("duplicate DisplayName: %q", a.DisplayName)
		}
		seen[a.DisplayName] = true
	}
}

// ---- Action count ----

func TestAllActions_Count(t *testing.T) {
	n := len(AllActions())
	if n < 10 {
		t.Errorf("expected at least 10 actions, got %d", n)
	}
	if n > 30 {
		t.Errorf("expected at most 30 actions, got %d — review if all are needed", n)
	}
}

// ---- Category grouping ----

func TestAllActions_AtLeastOnePerCategory(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range AllActions() {
		seen[a.Category] = true
	}
	for _, cat := range []string{"social", "care", "learning", "none"} {
		if !seen[cat] {
			t.Errorf("no actions in Category %q", cat)
		}
	}
}

// ---- ToolHint only when NeedsTool ----

func TestAllActions_ToolHintOnlyWhenNeedsTool(t *testing.T) {
	for _, a := range AllActions() {
		if a.ToolHint != "" && !a.NeedsTool {
			t.Errorf("%s: ToolHint set but NeedsTool=false", a.Name)
		}
	}
}

// ---- none action is special ----

func TestActionNone_HasQuietWeightHigh(t *testing.T) {
	a := ActionByName("none")
	if a == nil {
		t.Fatal("none action not found")
	}
	if a.Weights.Quiet < 0.9 {
		t.Errorf("none.Quiet = %.2f, should be near 1.0", a.Weights.Quiet)
	}
	if a.Category != "none" {
		t.Errorf("none.Category = %q", a.Category)
	}
	if a.NeedsTool {
		t.Error("none should not need tool")
	}
}

// ---- search action has tool fields ----

func TestActionSearch_ConfiguredForTool(t *testing.T) {
	a := ActionByName("search")
	if a == nil {
		t.Fatal("search action not found")
	}
	if !a.NeedsTool {
		t.Error("search should need tool")
	}
	if a.ToolName != "search" {
		t.Errorf("search.ToolName = %q", a.ToolName)
	}
	if a.ToolHint == "" {
		t.Error("search should have ToolHint for LLM guidance")
	}
	if a.Source != "knowledge_gap" {
		t.Errorf("search.Source = %q, want knowledge_gap", a.Source)
	}
}

// ---- analyze_patterns is special-case ----

func TestActionAnalyzePatterns_NotNeedsTool(t *testing.T) {
	a := ActionByName("analyze_patterns")
	if a == nil {
		t.Fatal("analyze_patterns action not found")
	}
	if a.NeedsTool {
		t.Error("analyze_patterns.NeedsTool should be false (handled by special case in lifecycle.go)")
	}
}
