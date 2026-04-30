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
	req.Subject = opts.Subject
	req.Body = opts.Body
	req.HTML = opts.HTML
	req.TemplateID = opts.TemplateID
	req.TemplateVariables = opts.TemplateVariables
	req.Priority = opts.Priority
	req.ScheduledAt = opts.ScheduledAt
	return req
}

// NotifyByEmail sends to channel email. recipient is typically the destination email address.
func (s *NotificationsService) NotifyByEmail(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelEmail, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyBySMS sends to channel sms. recipient is an E.164 phone number.
func (s *NotificationsService) NotifyBySMS(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelSMS, userID, idempotencyKey, notificationType, recipient, opts))
}

// NotifyByPush sends to channel push.
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

// NotifyBySlack sends to channel slack. recipient is typically the Incoming Webhook URL;
// message text belongs in opts.Body or a template.
func (s *NotificationsService) NotifyBySlack(ctx context.Context, userID, idempotencyKey, notificationType, recipient string, opts *NotifyOptions) (*SendResponse, error) {
	return s.Send(ctx, sendForChannel(ChannelSlack, userID, idempotencyKey, notificationType, recipient, opts))
}

// Send dispatches a notification via the full SendRequest.
func (s *NotificationsService) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	var out SendResponse
	if err := s.client.do(ctx, "POST", "/notifications", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns a paginated list of notifications matching the given filters.
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
		if params.Recipient != "" {
			q.Set("recipient", params.Recipient)
		}
		if params.Search != "" {
			q.Set("search", params.Search)
		}
		if params.DateFrom != "" {
			q.Set("date_from", params.DateFrom)
		}
		if params.DateTo != "" {
			q.Set("date_to", params.DateTo)
		}
		if params.APIKeyID != "" {
			q.Set("api_key_id", params.APIKeyID)
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

// Get returns full notification details including delivery attempts and events.
func (s *NotificationsService) Get(ctx context.Context, id string) (*Notification, error) {
	var out Notification
	if err := s.client.do(ctx, "GET", "/notifications/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Sync polls the originating provider for the latest delivery status and updates the record.
func (s *NotificationsService) Sync(ctx context.Context, id string) (*SyncResponse, error) {
	var out SyncResponse
	if err := s.client.do(ctx, "POST", fmt.Sprintf("/notifications/%s/sync", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrigger starts a new delivery workflow for an existing failed or stuck notification.
func (s *NotificationsService) Retrigger(ctx context.Context, id string) (*RetriggerResponse, error) {
	var out RetriggerResponse
	if err := s.client.do(ctx, "POST", fmt.Sprintf("/notifications/%s/retrigger", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListScheduled returns a paginated list of pending scheduled notifications.
func (s *NotificationsService) ListScheduled(ctx context.Context, params *ListScheduledParams) (*ListScheduledResponse, error) {
	q := url.Values{}
	if params != nil {
		if params.Page > 0 {
			q.Set("page", strconv.Itoa(params.Page))
		}
		if params.PageSize > 0 {
			q.Set("page_size", strconv.Itoa(params.PageSize))
		}
		if params.Status != "" {
			q.Set("status", string(params.Status))
		}
	}

	path := "/notifications/scheduled"
	if len(q) > 0 {
		path = fmt.Sprintf("%s?%s", path, q.Encode())
	}

	var out ListScheduledResponse
	if err := s.client.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Reschedule updates the dispatch time of a scheduled notification.
func (s *NotificationsService) Reschedule(ctx context.Context, id string, req RescheduleRequest) (*Notification, error) {
	var out Notification
	if err := s.client.do(ctx, "PATCH", fmt.Sprintf("/notifications/%s/schedule", id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelScheduled cancels a pending scheduled notification.
func (s *NotificationsService) CancelScheduled(ctx context.Context, id string) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/notifications/%s/schedule", id), nil, nil)
}
