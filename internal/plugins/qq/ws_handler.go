package qq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSPayload is a generic QQ Bot WebSocket message.
type WSPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  int             `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// WSHandler manages the QQ Bot WebSocket connection lifecycle.
type WSHandler struct {
	appID     string
	appSecret string

	conn   *websocket.Conn
	connMu sync.Mutex
	seq    int

	stopCh chan struct{}
	onMsg  func(userID, content string)

	client  *http.Client
	token   string
	tokenMu sync.Mutex
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(appID, appSecret string) *WSHandler {
	return &WSHandler{
		appID:     appID,
		appSecret: appSecret,
		stopCh:    make(chan struct{}),
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// SetMessageHandler sets the callback for incoming private messages.
func (h *WSHandler) SetMessageHandler(fn func(userID, content string)) {
	h.onMsg = fn
}

// getAccessToken fetches a QQ Bot access token using AppID + Secret.
func (h *WSHandler) getAccessToken() (string, error) {
	h.tokenMu.Lock()
	defer h.tokenMu.Unlock()

	if h.token != "" {
		return h.token, nil
	}

	reqBody := fmt.Sprintf(`{"appId":"%s","clientSecret":"%s"}`, h.appID, h.appSecret)
	resp, err := h.client.Post(
		"https://bots.qq.com/app/getAppAccessToken",
		"application/json",
		bytes.NewReader([]byte(reqBody)),
	)
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty access token")
	}

	h.token = result.AccessToken
	slog.Info("qq: got access token", "expires_in", result.ExpiresIn)
	return h.token, nil
}

// Connect establishes the WebSocket connection and starts the message loop.
func (h *WSHandler) Connect() error {
	token, err := h.getAccessToken()
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	url := fmt.Sprintf("wss://api.sgroup.qq.com/websocket?access_token=%s", token)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	h.connMu.Lock()
	h.conn = conn
	h.connMu.Unlock()

	slog.Info("qq: websocket connected")

	// Store token for identify and reconnect.
	h.token = token

	go h.readLoop()
	go h.heartbeatLoop()
	return nil
}

func (h *WSHandler) readLoop() {
	defer func() {
		h.connMu.Lock()
		if h.conn != nil {
			h.conn.Close()
		}
		h.connMu.Unlock()
	}()

	backoff := 2 * time.Second
	const maxBackoff = 5 * time.Minute

	for {
		select {
		case <-h.stopCh:
			return
		default:
		}

		h.connMu.Lock()
		conn := h.conn
		h.connMu.Unlock()
		if conn == nil {
			return
		}

		var payload WSPayload
		if err := conn.ReadJSON(&payload); err != nil {
			slog.Warn("qq: ws read error, reconnecting", "err", err, "backoff", backoff)
			h.connMu.Lock()
			if h.conn != nil {
				h.conn.Close()
				h.conn = nil
			}
			h.connMu.Unlock()

			select {
			case <-h.stopCh:
				return
			case <-time.After(backoff):
			}

			if err := h.Connect(); err != nil {
				slog.Error("qq: reconnect failed", "err", err)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			backoff = 2 * time.Second // reset on success
			continue
		}

		backoff = 2 * time.Second // reset on successful read
		h.seq = payload.S

		switch payload.Op {
		case 10: // Hello — send Identify with token.
			slog.Info("qq: hello received, sending identify")
			h.sendIdentify()
		case 11: // Heartbeat ACK
		case 0: // Dispatch
			h.handleDispatch(payload.T, payload.D)
		}
	}
}

func (h *WSHandler) sendIdentify() {
	h.connMu.Lock()
	conn := h.conn
	h.connMu.Unlock()
	if conn == nil {
		return
	}
	// QQ Bot WebSocket: Identify (op=2) with token and intents.
	// Intent 1<<25 = C2C (private chat) events.
	payload := map[string]interface{}{
		"op": 2,
		"d": map[string]interface{}{
			"token":   fmt.Sprintf("QQBot %s", h.token),
			"intents": 1 << 25,
		},
	}
	b, err := json.Marshal(payload); if err != nil { slog.Error("qq: marshal identify", "err", err); return }
	conn.WriteMessage(websocket.TextMessage, b)
	slog.Info("qq: identify sent")
}

func (h *WSHandler) heartbeatLoop() {
	ticker := time.NewTicker(40 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.connMu.Lock()
			conn := h.conn
			h.connMu.Unlock()
			if conn == nil {
				return
			}
			hb := WSPayload{Op: 1, D: json.RawMessage(fmt.Sprintf(`%d`, h.seq))}
			if err := conn.WriteJSON(hb); err != nil {
				slog.Warn("qq: heartbeat failed", "err", err)
			}
		}
	}
}

func (h *WSHandler) handleDispatch(t string, d json.RawMessage) {
	if t != "C2C_MESSAGE_CREATE" {
		return
	}
	var msg struct {
		Author struct {
			ID string `json:"id"`
		} `json:"author"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(d, &msg); err != nil {
		slog.Warn("qq: parse c2c message", "err", err)
		return
	}
	if h.onMsg != nil && msg.Content != "" {
		h.onMsg(msg.Author.ID, msg.Content)
	}
}

// SendPrivateMessage sends a text message to a QQ user.
func (h *WSHandler) SendPrivateMessage(userID, content string) error {
	token, err := h.getAccessToken()
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"msg_type": 0,
		"content":  content,
	}
	b, err := json.Marshal(body); if err != nil { return fmt.Errorf("marshal qq message: %w", err) }

	url := fmt.Sprintf("https://api.sgroup.qq.com/v2/users/%s/messages", userID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("QQBot %s", token))

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("send qq message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("qq api returned %d", resp.StatusCode)
	}
	return nil
}

// Stop shuts down the WebSocket connection.
func (h *WSHandler) Stop() {
	close(h.stopCh)
	h.connMu.Lock()
	defer h.connMu.Unlock()
	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
}
