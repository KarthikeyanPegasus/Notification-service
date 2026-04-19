package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/circuit"
	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// WebSocketWorker maintains a registry of active connections and delivers real-time notifications.
type WebSocketWorker struct {
	base        *BaseWorker
	cache       *cache.Client
	connections sync.Map // userID -> []*websocket.Conn
}

func NewWebSocketWorker(
	subscriber pubsub.Subscriber,
	cacheClient *cache.Client,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *WebSocketWorker {
	return &WebSocketWorker{
		base: newBaseWorker(
			domain.ChannelWebSocket, "websocket-worker-sub",
			subscriber, notifRepo, attemptRepo, eventRepo, registry, log,
		),
		cache: cacheClient,
	}
}

func (w *WebSocketWorker) Channel() domain.Channel { return domain.ChannelWebSocket }

// RegisterConnection adds a WebSocket connection for a user.
func (w *WebSocketWorker) RegisterConnection(userID string, conn *websocket.Conn) {
	existing, _ := w.connections.LoadOrStore(userID, []*websocket.Conn{})
	conns := existing.([]*websocket.Conn)
	w.connections.Store(userID, append(conns, conn))
	_ = w.cache.SAdd(context.Background(), fmt.Sprintf("ws:presence:%s", userID), conn.RemoteAddr().String())
}

// RemoveConnection removes a closed connection.
func (w *WebSocketWorker) RemoveConnection(userID string, conn *websocket.Conn) {
	existing, ok := w.connections.Load(userID)
	if !ok {
		return
	}
	conns := existing.([]*websocket.Conn)
	updated := conns[:0]
	for _, c := range conns {
		if c != conn {
			updated = append(updated, c)
		}
	}
	w.connections.Store(userID, updated)
	_ = w.cache.SRem(context.Background(), fmt.Sprintf("ws:presence:%s", userID), conn.RemoteAddr().String())
}

func (w *WebSocketWorker) Start(ctx context.Context) error {
	w.base.log.Info("websocket worker started")
	return w.base.subscriber.Subscribe(ctx, "websocket", func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			return w.sendToUser(ctx, n)
		}, "websocket-gateway")
	})
}

func (w *WebSocketWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	// No-op for websocket channel.
}

func (w *WebSocketWorker) sendToUser(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()
	userID := n.UserID.String()

	existing, ok := w.connections.Load(userID)
	if !ok {
		// User offline — persist for inbox fetch
		w.base.log.Debug("no active WS connection — notification stored for inbox",
			zap.String("user_id", userID),
		)
		return domain.DeliveryResult{
			Success:       true,
			Provider:      "websocket-gateway",
			ProviderMsgID: "inbox-" + uuid.New().String(),
			LatencyMs:     int(time.Since(start).Milliseconds()),
		}, nil
	}

	conns := existing.([]*websocket.Conn)
	payload, _ := json.Marshal(map[string]any{
		"notification_id": n.ID.String(),
		"type":            n.Type,
		"content":         n.RenderedContent,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})

	var sent bool
	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			w.base.log.Warn("websocket write error",
				zap.String("user_id", userID),
				zap.Error(err),
			)
			continue
		}
		sent = true
	}

	latencyMs := int(time.Since(start).Milliseconds())
	if !sent && len(conns) > 0 {
		return domain.DeliveryResult{
			Provider:     "websocket-gateway",
			LatencyMs:    latencyMs,
			ErrorMessage: "all websocket connections failed",
		}, fmt.Errorf("websocket: all connections failed for user %s", userID)
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      "websocket-gateway",
		ProviderMsgID: "ws-" + uuid.New().String(),
		LatencyMs:     latencyMs,
	}, nil
}
