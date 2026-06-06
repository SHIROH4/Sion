package care

import (
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

func NewObservation(source domain.ObservationSource, content string) domain.Observation {
	return domain.Observation{
		Source:    source,
		Content:   strings.TrimSpace(content),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}
