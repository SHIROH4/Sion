package memory

import (
	"desktop-pet/internal/domain"
	"fmt"
	"time"
)

// MemoryMeta describes a compressed memory segment and is stored in domain.Message.Meta.
type MemoryMeta struct {
	Level     int
	StartTime time.Time
	EndTime   time.Time
}

// Name returns the archive marker label, e.g. "1-20240101120000-20240101130000".
func (m MemoryMeta) Name() string {
	return fmt.Sprintf("%d-%s-%s", m.Level,
		m.StartTime.Format("20060102150405"),
		m.EndTime.Format("20060102150405"))
}

// MetaLevel returns the compression level, satisfying the metaLeveler interface
// so Store.SaveHistory can extract the level automatically.
func (m MemoryMeta) MetaLevel() int { return m.Level }

// CompressConfig controls compression thresholds and limits.
type CompressConfig struct {
	Level0Threshold    int // original messages to trigger compression, default 8
	HighLevelThreshold int // L1+ summaries to trigger compression, default 3
	MaxLevel           int // highest compression level, default 3
}

// Compressor manages multi-level inline-marker compression.
type Compressor struct {
	llmSync   func(messages []domain.Message) (string, error)
	config    CompressConfig
	OnArchive func(name string, level int, original []domain.Message, summary string)
}

// SetLLMSync replaces the summarisation function. Passing nil disables compression.
func (c *Compressor) SetLLMSync(fn func([]domain.Message) (string, error)) {
	c.llmSync = fn
}

// NewCompressor returns a Compressor with defaults filled in.
func NewCompressor(llmSync func([]domain.Message) (string, error), cfg CompressConfig) *Compressor {
	if cfg.Level0Threshold <= 0 {
		cfg.Level0Threshold = 8
	}
	if cfg.HighLevelThreshold <= 0 {
		cfg.HighLevelThreshold = 3
	}
	if cfg.MaxLevel <= 0 {
		cfg.MaxLevel = 3
	}
	return &Compressor{llmSync: llmSync, config: cfg}
}

// ShouldCompress returns true when the number of Level-0 (original) messages
// reaches or exceeds the configured threshold.
func (c *Compressor) ShouldCompress(messages []domain.Message) bool {
	return c.countLevel(messages, 0) >= c.threshold(0)
}

// Compress performs one round of compression starting at the given level.
// It scans for the first continuous segment of messages at `level`, and if
// the segment is large enough, replaces the leading messages with a summary.
// It then recurses to handle cascading compressions.
func (c *Compressor) Compress(messages []domain.Message, level int) []domain.Message {
	if level > c.config.MaxLevel {
		return messages
	}
	if c.llmSync == nil {
		return messages
	}

	thresh := c.threshold(level)
	if c.countLevel(messages, level) < thresh {
		return messages
	}

	// Find the range [start, end) of the first continuous Level=level segment.
	start, end := c.findSegment(messages, level)
	segLen := end - start
	if segLen < thresh {
		return messages
	}

	// Summarise the first compressCount messages of the segment.
	n := c.compressCount(level)
	if n > segLen {
		n = segLen
	}
	toCompress := messages[start : start+n]

	summary, err := c.llmSync(toCompress)
	if err != nil {
		return messages
	}

	// Determine time bounds for the new MemoryMeta.
	meta := c.buildMeta(level+1, toCompress)

	archived := domain.Message{
		Role:    "system",
		Content: fmt.Sprintf("[记忆存档 L%s] %s", meta.Name(), summary),
		Meta:    meta,
	}

	// Persist the original messages so they can be recalled later.
	if c.OnArchive != nil {
		original := make([]domain.Message, n)
		copy(original, toCompress)
		c.OnArchive(meta.Name(), meta.Level, original, summary)
	}

	// Replace the compressed slice with the single summary message.
	result := make([]domain.Message, 0, len(messages)-n+1)
	result = append(result, messages[:start]...)
	result = append(result, archived)
	result = append(result, messages[start+n:]...)

	// Recurse — ascend to the next level.
	return c.Compress(result, level+1)
}

// threshold returns the minimum segment size to trigger compression.
func (c *Compressor) threshold(level int) int {
	if level == 0 {
		return c.config.Level0Threshold
	}
	return c.config.HighLevelThreshold
}

// compressCount returns how many messages to fold into one summary.
func (c *Compressor) compressCount(level int) int {
	if level == 0 {
		return 5
	}
	return 3
}

// countLevel counts messages currently at the given compression level.
// Level 0 means messages whose Meta is nil or whose MemoryMeta.Level == 0.
func (c *Compressor) countLevel(messages []domain.Message, level int) int {
	count := 0
	for _, m := range messages {
		if c.msgLevel(m) == level {
			count++
		}
	}
	return count
}

// findSegment returns [start, end) of the first maximal consecutive run of
// messages at `level`.
func (c *Compressor) findSegment(messages []domain.Message, level int) (int, int) {
	start := -1
	for i, m := range messages {
		if c.msgLevel(m) == level {
			if start == -1 {
				start = i
			}
		} else if start != -1 {
			return start, i
		}
	}
	if start != -1 {
		return start, len(messages)
	}
	return 0, 0
}

// msgLevel extracts the compression level from a message's Meta.
func (c *Compressor) msgLevel(m domain.Message) int {
	if m.Meta == nil {
		return 0
	}
	if mm, ok := m.Meta.(MemoryMeta); ok {
		return mm.Level
	}
	if ml, ok := m.Meta.(metaLeveler); ok {
		return ml.MetaLevel()
	}
	return 0
}

// buildMeta creates a MemoryMeta for the new summary, spanning the time
// range covered by the compressed messages.
func (c *Compressor) buildMeta(level int, msgs []domain.Message) MemoryMeta {
	now := time.Now()
	var start, end time.Time

	for _, m := range msgs {
		msgStart, msgEnd := c.msgTimeBounds(m)
		if msgStart.IsZero() {
			continue
		}
		if start.IsZero() || msgStart.Before(start) {
			start = msgStart
		}
		if end.IsZero() || msgEnd.After(end) {
			end = msgEnd
		}
	}
	if start.IsZero() {
		start = now
	}
	if end.IsZero() {
		end = now
	}
	return MemoryMeta{Level: level, StartTime: start, EndTime: end}
}

// msgTimeBounds extracts time bounds from a message's Meta.
func (c *Compressor) msgTimeBounds(m domain.Message) (time.Time, time.Time) {
	if m.Meta == nil {
		return time.Time{}, time.Time{}
	}
	if mm, ok := m.Meta.(MemoryMeta); ok {
		return mm.StartTime, mm.EndTime
	}
	return time.Time{}, time.Time{}
}

type metaLeveler interface{ MetaLevel() int }
