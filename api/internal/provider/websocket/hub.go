package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spidey/notification-service/internal/domain"
	"go.uber.org/zap"
)

// Upgrader re-exports gorilla/websocket.Upgrader for use by HTTP handlers.
type Upgrader = websocket.Upgrader

const (
	// writeWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// pongWait is the time allowed to read the next pong from the peer.
	pongWait = 60 * time.Second
	// pingPeriod is the interval at which pings are sent (must be less than pongWait).
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize is the maximum message size allowed from the peer.
	maxMessageSize = 4096
)

// Client represents a single WebSocket connection.
type Client struct {
	ID     string
	UserID string
	Conn   *websocket.Conn
	Send   chan []byte
	Hub    *Hub
	Log    *zap.Logger
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.Log.Debug("websocket read error", zap.Error(err))
			}
			break
		}
		// We don't process incoming messages from clients for now —
		// this is a notification push channel only.
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Hub maintains the set of active WebSocket clients grouped by user ID.
type Hub struct {
	mu         sync.RWMutex
	clients    map[string]map[*Client]bool // userID -> clients
	Register   chan *Client
	Unregister chan *Client
	log        *zap.Logger
}

// NewHub creates a new WebSocket hub.
func NewHub(log *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		log:        log,
	}
}

// Run starts the hub's event loop. Must be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			if h.clients[client.UserID] == nil {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.UserID][client] = true
			count := len(h.clients[client.UserID])
			h.mu.Unlock()
			h.log.Info("websocket client connected",
				zap.String("user_id", client.UserID),
				zap.String("client_id", client.ID),
				zap.Int("connections_for_user", count),
			)

		case client := <-h.Unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.UserID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(h.clients, client.UserID)
					}
				}
			}
			h.mu.Unlock()
			h.log.Info("websocket client disconnected",
				zap.String("user_id", client.UserID),
				zap.String("client_id", client.ID),
			)
		}
	}
}

// SendToUser sends a notification payload to all connected clients for a user.
func (h *Hub) SendToUser(userID string, notification *domain.Notification) error {
	payload, err := json.Marshal(map[string]any{
		"type":    "notification",
		"id":      notification.ID.String(),
		"channel": notification.Channel,
		"type_name": notification.Type,
		"subject": notification.RenderedContent.Subject,
		"body":    notification.RenderedContent.Body,
		"data":    notification.RenderedContent.Data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshalling websocket payload: %w", err)
	}

	h.mu.RLock()
	clients, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok || len(clients) == 0 {
		return fmt.Errorf("no websocket clients connected for user %s", userID)
	}

	sent := 0
	var lastErr error
	for client := range clients {
		select {
		case client.Send <- payload:
			sent++
		default:
			// Client's send buffer is full — drop message for this connection
			lastErr = fmt.Errorf("websocket client %s buffer full, dropping message", client.ID)
			h.log.Warn("websocket send buffer full, dropping message",
				zap.String("user_id", userID),
				zap.String("client_id", client.ID),
			)
		}
	}
	if sent == 0 {
		return fmt.Errorf("failed to send to any websocket client for user %s: %w", userID, lastErr)
	}
	return nil
}

// ConnectedCount returns the number of connected clients for a user.
func (h *Hub) ConnectedCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if clients, ok := h.clients[userID]; ok {
		return len(clients)
	}
	return 0
}

// TotalConnections returns the total number of connected clients across all users.
func (h *Hub) TotalConnections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, clients := range h.clients {
		total += len(clients)
	}
	return total
}

// SendWSNotification sends a notification via WebSocket to the notification's user.
// Returns an error if the delivery fails or no clients are connected.
func SendWSNotification(ctx context.Context, n *domain.Notification, hub *Hub) (domain.DeliveryResult, error) {
	start := time.Now()

	if n.UserID == nil {
		return domain.DeliveryResult{
			Provider:     "websocket",
			ErrorMessage: "user_id is nil, cannot route websocket notification",
		}, fmt.Errorf("websocket: user_id is nil")
	}

	userID := n.UserID.String()
	if err := hub.SendToUser(userID, n); err != nil {
		return domain.DeliveryResult{
			Provider:     "websocket",
			LatencyMs:    int(time.Since(start).Milliseconds()),
			ErrorMessage: err.Error(),
		}, err
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      "websocket",
		ProviderMsgID: fmt.Sprintf("ws-%s-%d", userID, time.Now().UnixNano()),
		LatencyMs:     int(time.Since(start).Milliseconds()),
	}, nil
}
