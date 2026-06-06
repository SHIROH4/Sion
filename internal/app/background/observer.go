package background

import (
	"fmt"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/native"
)

// ScreenObserver handles L1 screen observation callbacks, updating
// care state and feeding observations to the proactive learner.
type ScreenObserver struct {
	Care    domain.CareProvider
	Learner interface{ Ingest(domain.Observation) }
	Summary *string // points to lastScreenSummary string
}

// NewScreenObserver creates a ScreenObserver.
func NewScreenObserver(care domain.CareProvider, learner interface{ Ingest(domain.Observation) }, summary *string) *ScreenObserver {
	return &ScreenObserver{Care: care, Learner: learner, Summary: summary}
}

// OnObserve handles a screen observation event.
func (o *ScreenObserver) OnObserve(obs native.ScreenObservation) {
	if native.IsSelfApp(obs.AppName) {
		return
	}
	app := native.FriendlyAppName(obs.AppName)
	summary := fmt.Sprintf("软件: %s", app)
	if obs.WindowTitle != "" {
		summary += fmt.Sprintf(" | 窗口: %s", obs.WindowTitle)
	}
	if obs.IsWorking {
		summary += " | 正在工作"
	} else {
		summary += " | 休闲中"
	}
	*o.Summary = summary

	if o.Care == nil {
		return
	}
	if obs.IsWorking {
		o.Care.IncrementWork(60)
	} else {
		o.Care.ResetWork()
	}
	content := fmt.Sprintf("%s %s %s", obs.AppName, obs.WindowTitle, obs.OCRText)
	observation := domain.Observation{
		Source:  domain.ObsScreen,
		Content: content,
	}
	o.Care.UpdateState(observation)
	if o.Learner != nil {
		o.Learner.Ingest(observation)
	}
}

// EagerObserve does an initial screen observation after a short delay.
func EagerObserve(summary *string) {
	// Called as goroutine from plugin Start.
	obs, err := native.OCRActiveScreen()
	if err != nil || obs.AppName == "" || native.IsSelfApp(obs.AppName) {
		return
	}
	app := native.FriendlyAppName(obs.AppName)
	s := fmt.Sprintf("软件: %s", app)
	if obs.WindowTitle != "" {
		s += fmt.Sprintf(" | 窗口: %s", obs.WindowTitle)
	}
	if obs.IsWorking {
		s += " | 正在工作"
	} else {
		s += " | 休闲中"
	}
	*summary = s
}
