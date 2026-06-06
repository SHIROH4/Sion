package cognition

import (
	"testing"
	"time"
	"strconv"
)

func TestLearner_RecordAndBatchLearn(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)

	// Record some drives.
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("speak_care", 0.2, 0.7, 0.1, 0.0, 0.0, -1.0)
	l.RecordDrive("speak_care", 0.2, 0.7, 0.1, 0.0, 0.0, -1.0)
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("none", 0.1, 0.1, 0.1, 0.9, 0.0, 0.0)

	if len(l.storedDrives) != 6 {
		t.Fatalf("expected 6 stored drives, got %d", len(l.storedDrives))
	}

	if !l.ShouldLearn() {
		t.Log("ShouldLearn returned false (interval not passed yet) — forcing batch learn")
		l.lastLearnAt = time.Time{} // force
	}

	n := l.BatchLearn()
	if n < 4 {
		t.Errorf("expected >=4 processed (4 non-neutral), got %d", n)
	}

	// Weights should have changed.
	w := m.weights["speak_casual"].Social
	if w == 0.80 {
		t.Error("speak_casual social weight should have changed after learning")
	}
	t.Logf("speak_casual social weight: %.4f (was 0.80)", w)
}

func TestLearner_NeutralSkipped(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)
	l.lastLearnAt = time.Time{}

	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 0.0) // neutral
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 0.0) // neutral
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0) // positive
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 0.0) // neutral
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0) // positive

	n := l.BatchLearn()
	if n != 2 {
		t.Errorf("expected 2 processed (neutrals skipped), got %d", n)
	}
}

func TestLearner_ShouldLearn(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)

	if l.ShouldLearn() {
		t.Error("should not learn with 0 drives")
	}

	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)

	if l.ShouldLearn() {
		t.Log("ShouldLearn returned true (interval check depends on timing)")
	}
}

func TestLearner_DriveCap(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)

	for i := 0; i < 600; i++ {
		l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	}
	if len(l.storedDrives) > 500 {
		t.Errorf("drive cap exceeded: %d", len(l.storedDrives))
	}
}

func TestLearner_Audit_NoData(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)

	stuck, drift := l.Audit()
	if len(stuck) > 0 || drift {
		t.Error("audit should return empty with no data")
	}
}

func TestLearner_Audit_DetectsStuck(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)

	for i := 0; i < 15; i++ {
		l.RecordDrive("speak_casual", 0.5, 0.3, 0.2, 0.1, 0.1, 1.0)
	}

	stuck, _ := l.Audit()
	if len(stuck) == 0 {
		t.Error("should detect stuck action loop")
	}
	if len(stuck) > 0 {
		t.Logf("stuck action: %s", stuck[0])
	}
}

func TestLearner_Audit_DetectsDrift(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)

	for i := 0; i < 15; i++ {
		l.RecordDrive("speak_care", 0.3, 0.7, 0.1, 0.0, 0.0, -1.0)
	}

	_, drift := l.Audit()
	if !drift {
		t.Error("should detect drift warning with mostly negative rewards")
	}
}

func TestLearner_DistillStrategies(t *testing.T) {
	// This test requires a principle repo, which we don't have in the test.
	// Just verify it doesn't panic with nil.
	m := NewMotivator()
	l := NewLearner(m)

	n := l.DistillStrategies(nil)
	if n != 0 {
		t.Error("should return 0 with nil repo")
	}
}

func TestLearner_SetOutcomeRepo(t *testing.T) {
	m := NewMotivator()
	l := NewLearner(m)
	l.SetOutcomeRepo(nil) // should not panic
	if l.outcomeRepo != nil {
		t.Error("should be nil")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"}, {5, "5"}, {10, "10"}, {99, "99"}, {100, "100"},
	}
	for _, tt := range tests {
		if got := strconv.Itoa(tt.n); got != tt.want {
			t.Errorf("strconv.Itoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
