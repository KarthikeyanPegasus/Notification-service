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
