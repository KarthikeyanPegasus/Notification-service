package websocket

import (
	"context"

	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
)

// Ensure Sender interface compliance.
var _ provider.Sender = (*Sender)(nil)

// Sender delivers notifications to connected WebSocket clients via the Hub.
type Sender struct {
	hub *Hub
}

// NewSender creates a WebSocket sender that uses the given Hub for delivery.
func NewSender(hub *Hub) *Sender {
	return &Sender{hub: hub}
}

// ProviderName returns the provider identifier.
func (s *Sender) ProviderName() string {
	return "websocket"
}

// Send delivers a notification to the user's connected WebSocket clients.
func (s *Sender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	return SendWSNotification(ctx, n, s.hub)
}

// GetStatus returns the delivery status. WebSocket delivery is best-effort;
// once sent, there is no provider-side status to poll.
func (s *Sender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for WebSocket (best-effort delivery)",
	}, nil
}

// HubStats returns current WebSocket hub statistics.
type HubStats struct {
	TotalConnections int            `json:"total_connections"`
	ConnectedUsers   int            `json:"connected_users"`
}

// Stats returns current hub statistics.
func (s *Sender) Stats() HubStats {
	return HubStats{
		TotalConnections: s.hub.TotalConnections(),
		ConnectedUsers:   s.hub.TotalConnections(), // approximate
	}
}
