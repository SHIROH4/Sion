package api

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

const maxStatusLog = 30

// StatusEvent is a lightweight execution status update.
type StatusEvent struct {
	Time    string `json:"time"`
	Phase   string `json:"phase"`
	Status  string `json:"status"` // "start" | "ok" | "fail" | "info"
	Message string `json:"message"`
}

// StatusBus publishes execution status events via SSE.
type StatusBus struct {
	mu          sync.Mutex
	sse         *SSEBroker
	log         []StatusEvent
	storagePath string
}

var globalBus *StatusBus
var globalMu sync.Mutex

func InitStatusBus(sse *SSEBroker) *StatusBus {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalBus == nil {
		globalBus = &StatusBus{sse: sse}
	}
	return globalBus
}

func StatusBusInstance() *StatusBus {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalBus
}

// SetStoragePath enables persistence and loads any existing log.
func (b *StatusBus) SetStoragePath(path string) {
	b.storagePath = path
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var log []StatusEvent
	if json.Unmarshal(data, &log) == nil {
		b.mu.Lock()
		b.log = log
		b.mu.Unlock()
	}
}

// Emit publishes a status event to SSE and persists it.
func (b *StatusBus) Emit(phase, status, message string) {
	if b == nil || b.sse == nil {
		return
	}
	evt := StatusEvent{
		Time:    time.Now().Format("15:04:05"),
		Phase:   phase,
		Status:  status,
		Message: message,
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	b.sse.Publish("status", string(data))

	b.mu.Lock()
	b.log = append(b.log, evt)
	if len(b.log) > maxStatusLog {
		b.log = b.log[len(b.log)-maxStatusLog:]
	}
	b.mu.Unlock()

	b.persist()
}

func (b *StatusBus) persist() {
	if b.storagePath == "" {
		return
	}
	b.mu.Lock()
	data, err := json.Marshal(b.log)
	b.mu.Unlock()
	if err != nil {
		slog.Warn("status_bus: failed to marshal log", "err", err)
		return
	}
	_ = os.WriteFile(b.storagePath, data, 0644)
}

func (b *StatusBus) EmitStart(phase, message string) { b.Emit(phase, "start", message) }
func (b *StatusBus) EmitOK(phase, message string)    { b.Emit(phase, "ok", message) }
func (b *StatusBus) EmitFail(phase, message string)  { b.Emit(phase, "fail", message) }
func (b *StatusBus) EmitInfo(phase, message string)  { b.Emit(phase, "info", message) }

func (b *StatusBus) Recent(n int) []StatusEvent {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.log) <= n {
		result := make([]StatusEvent, len(b.log))
		copy(result, b.log)
		return result
	}
	start := len(b.log) - n
	result := make([]StatusEvent, n)
	copy(result, b.log[start:])
	return result
}
