package notification

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type NotificationsService struct {
	client *Client
}

func sendForChannel(
	ch Channel,
	userID, idempotencyKey, notificationType, recipient string,
	opts *NotifyOptions,
) *SendRequest {
	req := &SendRequest{
		IdempotencyKey: idempotencyKey,
		UserID:         userID,
		Channels:       []Channel{ch},
		Type:           notificationType,
		Recipient:      recipient,
	}
	if opts == nil {
		return req
	}
	req.Body = opts.Body
	req.TemplateID = opts.TemplateID
	req.TemplateVariables = opts.TemplateVariables
	req.ScheduledAt = opts.ScheduledAt
	return req
}

// NotifyByEmail sends to channel email. recipient is typically the destination email address.
func (s *NotificationsService) NotifyByEmail(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelEmail, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyBySMS sends to channel sms. recipient is typically an E.164 phone number.
func (s *NotificationsService) NotifyBySMS(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelSMS, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyByPush sends to channel push. recipient is the provider-specific device target when required.
func (s *NotificationsService) NotifyByPush(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelPush, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyByWebSocket sends to channel websocket.
func (s *NotificationsService) NotifyByWebSocket(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelWebSocket, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyByWebhook sends to channel webhook. recipient is typically the callback URL.
func (s *NotificationsService) NotifyByWebhook(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelWebhook, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyBySlack sends to channel slack. recipient is typically the Incoming Webhook URL; message text belongs in opts.Body or a template.
func (s *NotificationsService) NotifyBySlack(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelSlack, userID, idempotencyKey, notificationType, recipient, opts))
}

func (s *NotificationsService) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	var out SendResponse
	if err := s.client.do(ctx, "POST", "/notifications", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *NotificationsService) List(ctx context.Context, params *ListNotificationsParams) (*ListNotificationsResponse, error) {
	q := url.Values{}
	if params != nil {
		if params.Page > 0 {
			q.Set("page", strconv.Itoa(params.Page))
		}
		if params.PageSize > 0 {
			q.Set("page_size", strconv.Itoa(params.PageSize))
		}
		if params.UserID != "" {
			q.Set("user_id", params.UserID)
		}
		if params.Channel != "" {
			q.Set("channel", string(params.Channel))
		}
		if params.Status != "" {
			q.Set("status", string(params.Status))
		}
	}

	path := "/notifications"
	if len(q) > 0 {
		path = fmt.Sprintf("%s?%s", path, q.Encode())
	}

	var out ListNotificationsResponse
	if err := s.client.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *NotificationsService) Get(ctx context.Context, id string) (*NotificationDetailResponse, error) {
	var out NotificationDetailResponse
	if err := s.client.do(ctx, "GET", "/notifications/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
