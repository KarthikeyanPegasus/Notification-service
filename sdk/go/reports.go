package notification

import (
	"context"
	"fmt"
	"net/url"
)

type ReportsService struct {
	client *Client
}

func (s *ReportsService) buildQuery(f ReportFilters) string {
	q := url.Values{}
	if f.DateFrom != "" {
		q.Set("date_from", f.DateFrom)
	}
	if f.DateTo != "" {
		q.Set("date_to", f.DateTo)
	}
	if f.APIKeyID != "" {
		q.Set("api_key_id", f.APIKeyID)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// Summary returns delivery stats grouped by channel and date for the given range.
func (s *ReportsService) Summary(ctx context.Context, f ReportFilters) ([]*ReportSummaryItem, error) {
	var out []*ReportSummaryItem
	if err := s.client.do(ctx, "GET", "/reports/summary"+s.buildQuery(f), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// IngressBreakdown returns notification counts grouped by ingress source (api, pubsub, redis).
func (s *ReportsService) IngressBreakdown(ctx context.Context, f ReportFilters) ([]*IngressBreakdownItem, error) {
	var out []*IngressBreakdownItem
	if err := s.client.do(ctx, "GET", "/reports/ingress"+s.buildQuery(f), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SMSCountries returns SMS counts grouped by destination country prefix (top 10).
func (s *ReportsService) SMSCountries(ctx context.Context, f ReportFilters) ([]*BreakdownRow, error) {
	var out []*BreakdownRow
	if err := s.client.do(ctx, "GET", "/reports/sms-countries"+s.buildQuery(f), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// EmailDomains returns email counts grouped by recipient domain (top 10).
func (s *ReportsService) EmailDomains(ctx context.Context, f ReportFilters) ([]*BreakdownRow, error) {
	var out []*BreakdownRow
	if err := s.client.do(ctx, "GET", "/reports/email-domains"+s.buildQuery(f), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// VendorHealth returns real-time per-provider delivery metrics (last 12 hours).
func (s *ReportsService) VendorHealth(ctx context.Context) ([]*VendorMetric, error) {
	var out []*VendorMetric
	if err := s.client.do(ctx, "GET", "/reports/vendors", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// VendorBilling returns per-vendor cost data. Live from vendor APIs where available, estimated otherwise.
func (s *ReportsService) VendorBilling(ctx context.Context) ([]*VendorBilling, error) {
	var out []*VendorBilling
	if err := s.client.do(ctx, "GET", "/reports/billing", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ScheduledStats returns aggregate metrics for scheduled notifications in the given date range.
func (s *ReportsService) ScheduledStats(ctx context.Context, f ReportFilters) (*ScheduledStats, error) {
	var out ScheduledStats
	if err := s.client.do(ctx, "GET", fmt.Sprintf("/reports/scheduled-stats%s", s.buildQuery(f)), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
