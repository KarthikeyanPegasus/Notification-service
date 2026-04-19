package notification

import "context"

type ReportsService struct {
	client *Client
}

func (s *ReportsService) Summary(ctx context.Context) ([]*ReportSummaryItem, error) {
	var out []*ReportSummaryItem
	if err := s.client.do(ctx, "GET", "/reports/summary", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
