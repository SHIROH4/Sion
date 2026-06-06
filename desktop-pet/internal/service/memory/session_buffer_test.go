package memory

import (
	"testing"

	"desktop-pet/internal/app/plugin"
)

func TestSessionBuffer_Append(t *testing.T) {
	buf := NewSessionBuffer(20)

	buf.Append(plugin.Message{Role: "user", Content: "你好"})
	buf.Append(plugin.Message{Role: "assistant", Content: "你好喵"})
	buf.Append(plugin.Message{Role: "user", Content: "今天天气如何"})

	if buf.Len() != 3 {
		t.Fatalf("expected 3 messages, got %d", buf.Len())
	}

	all := buf.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 in All(), got %d", len(all))
	}
	if all[0].Content != "你好" {
		t.Errorf("expected first message 你好, got %q", all[0].Content)
	}
	if all[1].Content != "你好喵" {
		t.Errorf("expected second message 你好喵, got %q", all[1].Content)
	}
	if all[2].Content != "今天天气如何" {
		t.Errorf("expected third message 今天天气如何, got %q", all[2].Content)
	}
}

func TestSessionBuffer_RingOverflow(t *testing.T) {
	buf := NewSessionBuffer(3)

	for i := 0; i < 5; i++ {
		buf.Append(plugin.Message{Role: "user", Content: "msg"})
	}

	if buf.Len() != 3 {
		t.Fatalf("expected 3 messages after overflow, got %d", buf.Len())
	}

	// Clear and verify that only the last 3 are retained.
	buf.Clear()
	if buf.Len() != 0 {
		t.Errorf("expected 0 after Clear, got %d", buf.Len())
	}

	// Refill to verify Clear doesn't break subsequent use.
	buf.Append(plugin.Message{Role: "user", Content: "a"})
	buf.Append(plugin.Message{Role: "user", Content: "b"})
	buf.Append(plugin.Message{Role: "user", Content: "c"})
	buf.Append(plugin.Message{Role: "user", Content: "d"})

	all := buf.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 after refill overflow, got %d", len(all))
	}
	if all[0].Content != "b" {
		t.Errorf("expected oldest retained 'b', got %q", all[0].Content)
	}
	if all[2].Content != "d" {
		t.Errorf("expected newest retained 'd', got %q", all[2].Content)
	}
}

func TestSessionBuffer_Recent(t *testing.T) {
	buf := NewSessionBuffer(20)

	buf.Append(plugin.Message{Role: "user", Content: "1"})
	buf.Append(plugin.Message{Role: "assistant", Content: "2"})
	buf.Append(plugin.Message{Role: "user", Content: "3"})
	buf.Append(plugin.Message{Role: "assistant", Content: "4"})
	buf.Append(plugin.Message{Role: "user", Content: "5"})

	recent := buf.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent, got %d", len(recent))
	}
	if recent[0].Content != "3" {
		t.Errorf("expected '3', got %q", recent[0].Content)
	}
	if recent[2].Content != "5" {
		t.Errorf("expected '5', got %q", recent[2].Content)
	}

	// Request more than available.
	all := buf.Recent(100)
	if len(all) != 5 {
		t.Fatalf("expected 5 (capped), got %d", len(all))
	}

	// Request zero returns nil.
	if got := buf.Recent(0); got != nil {
		t.Errorf("expected nil for n=0, got %v", got)
	}

	// Empty buffer returns nil.
	empty := NewSessionBuffer(10)
	if got := empty.Recent(5); got != nil {
		t.Errorf("expected nil from empty buffer, got %v", got)
	}
}
