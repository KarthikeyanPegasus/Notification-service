package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/spidey/notification-service/internal/provider/websocket"
	"go.uber.org/zap"
)

// WebSocketHandler handles WebSocket upgrade requests.
type WebSocketHandler struct {
	hub *ws.Hub
	log *zap.Logger
}

// NewWebSocketHandler creates a new WebSocket upgrade handler.
func NewWebSocketHandler(hub *ws.Hub, log *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{hub: hub, log: log}
}

// Upgrade upgrades an HTTP connection to WebSocket and registers the client.
// Client must provide a valid JWT/API key for authentication; user_id is extracted
// from the claims and used for routing notifications.
//
// GET /v1/ws?token=<jwt_or_api_key>
func (h *WebSocketHandler) Upgrade(c *gin.Context) {
	// Extract user_id from the authenticated context (set by AnyAuth middleware).
	_, sub := getRoleAndSubject(c)
	if sub == "" {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "valid authentication required for WebSocket")
		return
	}

	// Use gin's ResponseWriter but wrap with WebSocket upgrader
	upgrader := DefaultWebSocketUpgrader()
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Warn("websocket upgrade failed",
			zap.String("user_id", sub),
			zap.Error(err),
		)
		return
	}

	client := &ws.Client{
		ID:     uuid.New().String(),
		UserID: sub,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		Hub:    h.hub,
		Log:    h.log,
	}

	h.hub.Register <- client

	// Start read and write pumps in goroutines
	go client.WritePump()
	go client.ReadPump()
}

// DefaultWebSocketUpgrader returns the standard WebSocket upgrader used by the service.
func DefaultWebSocketUpgrader() ws.Upgrader {
	return ws.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for WebSocket connections
		},
	}
}
