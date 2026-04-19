package notification

import "context"

type GovernanceService struct {
	client *Client
}

type Suppression struct {
	ID        string `json:"id"`
	Recipient string `json:"recipient"`
	Channel   Channel `json:"channel"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

type AddSuppressionRequest struct {
	Recipient string  `json:"recipient"`
	Channel   Channel `json:"channel"`
	Reason    string  `json:"reason,omitempty"`
}

func (s *GovernanceService) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	var out []*Suppression
	if err := s.client.do(ctx, "GET", "/governance/suppressions", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GovernanceService) AddSuppression(ctx context.Context, req *AddSuppressionRequest) (*Suppression, error) {
	var out Suppression
	if err := s.client.do(ctx, "POST", "/governance/suppressions", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
