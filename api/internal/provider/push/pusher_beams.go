package push

import (
	"context"
	"fmt"
	"strings"
	"time"

	beams "github.com/pusher/push-notifications-go"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

type PusherBeamsSender struct {
	client beams.PushNotifications
}

func NewPusherBeamsSender(cfg config.PusherConfig) *PusherBeamsSender {
	if strings.TrimSpace(cfg.InstanceID) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil
	}
	c, err := beams.New(strings.TrimSpace(cfg.InstanceID), strings.TrimSpace(cfg.SecretKey))
	if err != nil {
		return nil
	}
	return &PusherBeamsSender{client: c}
}

func (s *PusherBeamsSender) ProviderName() string { return "pusher" }

func (s *PusherBeamsSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	interest := strings.TrimSpace(n.Recipient)
	if interest == "" {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: "missing recipient (interest)"}, fmt.Errorf("missing recipient")
	}

	text := ""
	if n.RenderedContent != nil {
		text = strings.TrimSpace(n.RenderedContent.Body)
	}
	if text == "" {
		text = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}

	// Generic publish payload that should render on supported platforms.
	req := map[string]any{
		"fcm": map[string]any{
			"notification": map[string]any{
				"title": "NotifyHub",
				"body":  text,
			},
		},
		"apns": map[string]any{
			"aps": map[string]any{
				"alert": map[string]any{
					"title": "NotifyHub",
					"body":  text,
				},
			},
		},
		"web": map[string]any{
			"notification": map[string]any{
				"title": "NotifyHub",
				"body":  text,
			},
		},
	}

	// SDK doesn't take context; run with ctx select.
	type result struct {
		id  string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := s.client.PublishToInterests([]string{interest}, req)
		if err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{id: resp, err: nil}
	}()

	select {
	case <-ctx.Done():
		latencyMs := int(time.Since(start).Milliseconds())
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: ctx.Err().Error()}, ctx.Err()
	case r := <-ch:
		latencyMs := int(time.Since(start).Milliseconds())
		if r.err != nil {
			return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: r.err.Error()}, r.err
		}
		msgID := strings.TrimSpace(r.id)
		if msgID == "" {
			msgID = fmt.Sprintf("pusher-%d", time.Now().UnixMilli())
		}
		return domain.DeliveryResult{
			Success:       true,
			Provider:      s.ProviderName(),
			ProviderMsgID: msgID,
			LatencyMs:     latencyMs,
		}, nil
	}
}

func (s *PusherBeamsSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for pusher beams",
	}, nil
}

