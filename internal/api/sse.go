package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SSEBroker broadcasts events to all connected SSE clients.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

// SSEvent is a single server-sent event.
type SSEvent struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

// NewSSEBroker creates a new broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[chan string]struct{}),
	}
}

// Subscribe adds a client channel.
func (b *SSEBroker) Subscribe(ch chan string) {
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
}

// Unsubscribe removes a client channel.
func (b *SSEBroker) Unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

// Publish sends an event to all connected clients.
func (b *SSEBroker) Publish(event, data string) {
	msg, err := json.Marshal(SSEvent{Event: event, Data: data})
	if err != nil {
		return
	}
	s := string(msg)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- s:
		default:
			// skip slow clients
		}
	}
}

// Count returns the number of connected clients.
func (b *SSEBroker) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// SSEHandler returns an http.HandlerFunc that streams events to the client.
func (b *SSEBroker) SSEHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ch := make(chan string, 64)
		b.Subscribe(ch)
		defer b.Unsubscribe(ch)

		// Send initial connected event.
		fmt.Fprintf(w, "event: connected\ndata: {\"clients\":%d}\n\n", b.Count())
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var evt SSEvent
				if err := json.Unmarshal([]byte(msg), &evt); err != nil {
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, evt.Data)
				flusher.Flush()
			}
		}
	}
}
