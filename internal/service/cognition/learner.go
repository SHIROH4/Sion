package cognition

import (
	"log/slog"
	"strconv"
	"time"

	"desktop-pet/internal/domain"
)

// Learner performs offline learning: Judge scoring, DPO weight updates,
// experience distillation, and System3 meta-cognition audits.
type Learner struct {
	motivator     *Motivator
	outcomeRepo   domain.ActionOutcomeRepository
	lastLearnAt   time.Time
	minLearnInterval time.Duration
	storedDrives  []driveRecord // <Ctx drives, Act, R> for batch learning
}

// driveRecord stores the computed drives at decision time along with the outcome.
type driveRecord struct {
	Action  string
	Social  float64
	Care    float64
	Curious float64
	Quiet   float64
	Explore float64
	Reward  float64 // +1 positive, 0 neutral, -1 negative, or Judge score
	At      time.Time
}

// NewLearner creates a learner backed by the motivator.
func NewLearner(motivator *Motivator) *Learner {
	return &Learner{
		motivator:        motivator,
		minLearnInterval: 6 * time.Hour,
	}
}

// SetOutcomeRepo injects the outcome repository for historical analysis.
func (l *Learner) SetOutcomeRepo(repo domain.ActionOutcomeRepository) {
	l.outcomeRepo = repo
}

// RecordDrive stores a drive snapshot and returns its index for later reward attribution.
func (l *Learner) RecordDrive(action string, social, care, curious, quiet, explore float64, reward float64) int {
	l.storedDrives = append(l.storedDrives, driveRecord{
		Action:  action,
		Social:  social,
		Care:    care,
		Curious: curious,
		Quiet:   quiet,
		Explore: explore,
		Reward:  reward,
		At:      time.Now(),
	})
	if len(l.storedDrives) > 500 {
		l.storedDrives = l.storedDrives[len(l.storedDrives)-500:]
	}
	return len(l.storedDrives) - 1
}

// UpdateLastReward sets the reward on the drive entry at the given index.
func (l *Learner) UpdateLastReward(driveID int, reward float64) {
	if driveID < 0 || driveID >= len(l.storedDrives) {
		return
	}
	l.storedDrives[driveID].Reward = reward
}

// ShouldLearn returns true if enough time has passed since the last batch.
func (l *Learner) ShouldLearn() bool {
	return time.Since(l.lastLearnAt) > l.minLearnInterval && len(l.storedDrives) >= 5
}

// BatchLearn performs one epoch of offline weight updates using stored drives.
// Returns the number of records processed.
func (l *Learner) BatchLearn() int {
	if l.motivator == nil || len(l.storedDrives) == 0 {
		return 0
	}

	n := 0
	for _, d := range l.storedDrives {
		if d.Reward == 0 {
			continue // neutral samples don't contribute
		}
		// Scale reward: +1 → step +0.003, -1 → step -0.003
		l.motivator.UpdateWeightsFromOutcome(d.Action, d.Reward, d.Social, d.Care, d.Curious, d.Quiet, d.Explore)
		n++
	}

	// Keep only last 50 for next cycle (old data less relevant).
	if len(l.storedDrives) > 50 {
		l.storedDrives = l.storedDrives[len(l.storedDrives)-50:]
	}

	l.motivator.Save()
	l.lastLearnAt = time.Now()

	slog.Info("learner: batch complete", "processed", n, "remaining", len(l.storedDrives))
	return n
}

// DistillStrategies scans recent outcomes and generates strategy principles.
// Returns the number of new principles distilled.
func (l *Learner) DistillStrategies(principleRepo domain.StrategyPrincipleRepository) int {
	if principleRepo == nil || len(l.storedDrives) < 10 {
		return 0
	}

	// Simple heuristic: find patterns in recent 50 drives.
	recent := l.storedDrives
	if len(recent) > 50 {
		recent = recent[len(recent)-50:]
	}

	// Count action outcomes to find high-frequency patterns.
	type pattern struct {
		action      string
		hourBucket  int // 0, 6, 12, 18
		positive    int
		total       int
	}
	patterns := make(map[string]*pattern)

	for _, d := range recent {
		hb := (d.At.Hour() / 6) * 6
		key := d.Action + "_h" + strconv.Itoa(hb)
		p, ok := patterns[key]
		if !ok {
			p = &pattern{action: d.Action, hourBucket: hb}
			patterns[key] = p
		}
		p.total++
		if d.Reward > 0 {
			p.positive++
		}
	}

	added := 0
	now := time.Now().Unix()
	for _, p := range patterns {
		if p.total < 3 {
			continue
		}
		rate := float64(p.positive) / float64(p.total)
		slot := ""
		switch p.hourBucket {
		case 0:
			slot = "深夜"
		case 6:
			slot = "早晨"
		case 12:
			slot = "下午"
		case 18:
			slot = "晚间"
		}

		var principle domain.StrategyPrinciple
		if rate >= 0.7 {
			principle = domain.StrategyPrinciple{
				Situation:    slot + "时段",
				GoodStrategy: p.action + " — 成功率" + strconv.Itoa(int(rate*100)) + "%",
				Confidence:   rate,
				Source:       "auto-distill",
				Active:       true,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
		} else if rate <= 0.2 && p.total >= 5 {
			principle = domain.StrategyPrinciple{
				Situation:    slot + "时段",
				BadStrategy:  p.action + " — 成功率仅" + strconv.Itoa(int(rate*100)) + "%",
				GoodStrategy: "考虑换一种行为类型",
				Confidence:   1 - rate,
				Source:       "auto-distill",
				Active:       true,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
		}
		if principle.Situation != "" {
			if _, err := principleRepo.SavePrinciple(principle); err == nil {
				added++
			}
		}
	}
	return added
}

// Audit runs System3 meta-cognition: checks for stuck loops and drift.
func (l *Learner) Audit() (stuckActions []string, driftWarning bool) {
	if len(l.storedDrives) < 10 {
		return nil, false
	}

	recent := l.storedDrives
	if len(recent) > 30 {
		recent = recent[len(recent)-30:]
	}

	// Check for repeated same action.
	if len(recent) == 0 {
		return nil, false
	}
	lastAction := recent[len(recent)-1].Action
	streak := 1
	for i := len(recent) - 2; i >= 0; i-- {
		if recent[i].Action == lastAction {
			streak++
		} else {
			break
		}
	}
	if streak >= 5 && lastAction != "none" {
		stuckActions = append(stuckActions, lastAction)
	}

	// Check for drift: if recent rewards are mostly negative.
	negative := 0
	for _, d := range recent {
		if d.Reward < 0 {
			negative++
		}
	}
	if float64(negative)/float64(len(recent)) > 0.5 {
		driftWarning = true
	}

	return
}

