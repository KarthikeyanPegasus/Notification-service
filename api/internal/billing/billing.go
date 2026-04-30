package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spidey/notification-service/internal/config"
)

// BillingResult is the per-vendor billing summary returned to callers.
type BillingResult struct {
	Provider   string   `json:"provider"`
	TotalCost  float64  `json:"total_cost"`            // USD spent (0 if unknown / free)
	Balance    *float64 `json:"balance,omitempty"`     // remaining prepaid credit (USD)
	Currency   string   `json:"currency"`              // "USD" or provider-native
	Period     string   `json:"period"`                // "all_time" | "this_month" | "balance"
	IsActual   bool     `json:"is_actual"`             // true = from vendor API, false = estimated
	Source     string   `json:"source"`                // human-readable source label
	TotalCount int64    `json:"total_count"`           // messages we recorded in DB
	CostPerMsg float64  `json:"cost_per_msg"`          // per-message rate used for estimate
	Error      string   `json:"error,omitempty"`
}

// VendorSendTotal is what the DB repo provides for estimated-cost providers.
type VendorSendTotal struct {
	Provider string
	Total    int64
}

// Fetcher knows how to pull billing data for one vendor.
type Fetcher interface {
	Fetch(ctx context.Context) BillingResult
}

// httpGet is a minimal helper used by all fetchers.
func httpGet(ctx context.Context, url, authHeader string, client *http.Client) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 {
			req.Header.Set("Authorization", authHeader)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return body, resp.StatusCode, nil
}

// jsonField extracts a string or number field from raw JSON without full unmarshalling.
func jsonField[T any](body []byte, key string) (T, bool) {
	var m map[string]json.RawMessage
	var zero T
	if err := json.Unmarshal(body, &m); err != nil {
		return zero, false
	}
	raw, ok := m[key]
	if !ok {
		return zero, false
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return zero, false
	}
	return v, true
}

// --- Twilio ---

// TwilioFetcher fetches actual total spend from Twilio Usage Records (all-time, sms-outbound).
// Doc: https://www.twilio.com/docs/usage/api/usage-record#read-multiple-usagerecord-resources
type TwilioFetcher struct {
	cfg    config.TwilioConfig
	client *http.Client
}

func NewTwilioFetcher(cfg config.TwilioConfig) *TwilioFetcher {
	return &TwilioFetcher{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (f *TwilioFetcher) Fetch(ctx context.Context) BillingResult {
	res := BillingResult{
		Provider:   "twilio",
		Currency:   "USD",
		Period:     "all_time",
		Source:     "Twilio Usage Records API",
		CostPerMsg: 0.0079,
	}

	if f.cfg.AccountSID == "" || f.cfg.AuthToken == "" {
		res.Error = "twilio not configured"
		return res
	}

	// Use AllTime category for SMS outbound + MMS outbound costs
	// https://www.twilio.com/docs/usage/api/usage-record
	url := fmt.Sprintf(
		"https://api.twilio.com/2010-04-01/Accounts/%s/Usage/Records/AllTime.json?Category=sms-outbound&PageSize=1",
		f.cfg.AccountSID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.SetBasicAuth(f.cfg.AccountSID, f.cfg.AuthToken)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode >= 400 {
		res.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return res
	}

	// Parse usage_records array
	var payload struct {
		UsageRecords []struct {
			Count    string `json:"count"`
			Price    string `json:"price"`
			PriceUnit string `json:"price_unit"`
		} `json:"usage_records"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		res.Error = "parse error: " + err.Error()
		return res
	}
	if len(payload.UsageRecords) == 0 {
		res.IsActual = true
		return res
	}

	r := payload.UsageRecords[0]
	var count int64
	fmt.Sscanf(r.Count, "%d", &count)
	var price float64
	fmt.Sscanf(r.Price, "%f", &price)
	if price < 0 {
		price = -price
	}
	if r.PriceUnit != "" {
		res.Currency = strings.ToUpper(r.PriceUnit)
	}

	res.TotalCost = price
	res.TotalCount = count
	res.IsActual = true
	return res
}

// --- Plivo ---

// PlivoFetcher fetches account info from Plivo (remaining credit balance).
// Doc: https://www.plivo.com/docs/account/api/account/#retrieve-account
type PlivoFetcher struct {
	cfg    config.PlivoConfig
	client *http.Client
}

func NewPlivoFetcher(cfg config.PlivoConfig) *PlivoFetcher {
	return &PlivoFetcher{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (f *PlivoFetcher) Fetch(ctx context.Context) BillingResult {
	res := BillingResult{
		Provider:   "plivo",
		Currency:   "USD",
		Period:     "balance",
		Source:     "Plivo Account API",
		CostPerMsg: 0.0035,
	}

	if f.cfg.AuthID == "" || f.cfg.AuthToken == "" {
		res.Error = "plivo not configured"
		return res
	}

	url := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/", f.cfg.AuthID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.SetBasicAuth(f.cfg.AuthID, f.cfg.AuthToken)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))

	if resp.StatusCode >= 400 {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return res
	}

	credStr, _ := jsonField[string](body, "cash_credits")
	var balance float64
	fmt.Sscanf(credStr, "%f", &balance)
	res.Balance = &balance
	res.IsActual = true
	return res
}

// --- Vonage ---

// VonageFetcher fetches account balance from Vonage (remaining credit).
// Doc: https://developer.vonage.com/en/account/api-reference#getAccountBalance
type VonageFetcher struct {
	cfg    config.VonageConfig
	client *http.Client
}

func NewVonageFetcher(cfg config.VonageConfig) *VonageFetcher {
	return &VonageFetcher{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (f *VonageFetcher) Fetch(ctx context.Context) BillingResult {
	res := BillingResult{
		Provider:   "vonage",
		Currency:   "EUR",
		Period:     "balance",
		Source:     "Vonage Account API",
		CostPerMsg: 0.0065,
	}

	if f.cfg.APIKey == "" || f.cfg.APISecret == "" {
		res.Error = "vonage not configured"
		return res
	}

	url := fmt.Sprintf(
		"https://rest.nexmo.com/account/get-balance?api_key=%s&api_secret=%s",
		f.cfg.APIKey, f.cfg.APISecret,
	)
	body, status, err := httpGet(ctx, url, "", f.client)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if status >= 400 {
		res.Error = fmt.Sprintf("HTTP %d", status)
		return res
	}

	bal, ok := jsonField[float64](body, "value")
	if ok {
		res.Balance = &bal
	}
	res.IsActual = true
	return res
}

// --- MessageBird ---

// MessageBirdFetcher fetches account balance from MessageBird.
// Doc: https://developers.messagebird.com/api/balance/#the-balance-object
type MessageBirdFetcher struct {
	cfg    config.MessageBirdConfig
	client *http.Client
}

func NewMessageBirdFetcher(cfg config.MessageBirdConfig) *MessageBirdFetcher {
	return &MessageBirdFetcher{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (f *MessageBirdFetcher) Fetch(ctx context.Context) BillingResult {
	res := BillingResult{
		Provider:   "messagebird",
		Currency:   "EUR",
		Period:     "balance",
		Source:     "MessageBird Balance API",
		CostPerMsg: 0.009,
	}

	if f.cfg.AccessKey == "" {
		res.Error = "messagebird not configured"
		return res
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://rest.messagebird.com/balance", nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("Authorization", "AccessKey "+f.cfg.AccessKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))

	if resp.StatusCode >= 400 {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return res
	}

	// {"payment":"prepaid","type":"credits","amount":9.5}
	amount, ok := jsonField[float64](body, "amount")
	if ok {
		res.Balance = &amount
	}
	// currency is always EUR for MessageBird credits
	res.IsActual = true
	return res
}

// --- Mailgun (estimated) ---

// MailgunFetcher has no cost API; returns an estimated result using DB send counts.
// Doc: https://documentation.mailgun.com/docs/mailgun/api-reference/openapi-final/tag/Stats/
// Mailgun does expose /v3/domains/{domain}/stats/total but only gives message counts,
// not billing cost. We compute estimated cost from those counts × published rate.
type MailgunFetcher struct {
	cfg        config.MailgunConfig
	totalSent  int64
	client     *http.Client
}

func NewMailgunFetcher(cfg config.MailgunConfig, totalSent int64) *MailgunFetcher {
	return &MailgunFetcher{cfg: cfg, totalSent: totalSent, client: &http.Client{Timeout: 10 * time.Second}}
}

const mailgunPricePerMsg = 0.0008 // $0.80 / 1,000 (Flex plan)

func (f *MailgunFetcher) Fetch(ctx context.Context) BillingResult {
	res := BillingResult{
		Provider:   "mailgun",
		Currency:   "USD",
		Period:     "all_time",
		Source:     "Estimated (DB records × $0.0008/msg)",
		CostPerMsg: mailgunPricePerMsg,
		TotalCount: f.totalSent,
		TotalCost:  float64(f.totalSent) * mailgunPricePerMsg,
		IsActual:   false,
	}

	if f.cfg.Domain == "" || f.cfg.APIKey == "" {
		return res
	}

	// Fetch delivered count from Mailgun Stats API to refine the estimate.
	// GET https://api.mailgun.net/v3/{domain}/stats/total?event=delivered&duration=forever
	// (Note: Mailgun uses Basic auth with "api" as username.)
	url := fmt.Sprintf("https://api.mailgun.net/v3/%s/stats/total?event=delivered&duration=1m", f.cfg.Domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return res
	}
	req.SetBasicAuth("api", f.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			resp.Body.Close()
		}
		return res
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))

	// {"start":"...","end":"...","resolution":"month","stats":[{"time":"...","delivered":{"smtp":10,"http":5,"total":15}}]}
	var payload struct {
		Stats []struct {
			Delivered struct {
				Total int64 `json:"total"`
			} `json:"delivered"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || len(payload.Stats) == 0 {
		return res
	}

	var delivered int64
	for _, s := range payload.Stats {
		delivered += s.Delivered.Total
	}
	if delivered > 0 {
		// Use Mailgun's own delivered count as the base for cost estimate.
		res.TotalCount = delivered
		res.TotalCost = float64(delivered) * mailgunPricePerMsg
		res.Source = "Estimated (Mailgun Stats API × $0.0008/msg)"
	}
	return res
}

// --- Estimated (generic for SES, SendGrid, Postmark) ---

// EstimatedFetcher computes cost from DB send counts × a fixed per-message rate.
type EstimatedFetcher struct {
	provider   string
	totalSent  int64
	pricePerMsg float64
	currency   string
}

func NewEstimatedFetcher(provider string, totalSent int64, pricePerMsg float64) *EstimatedFetcher {
	return &EstimatedFetcher{
		provider:    provider,
		totalSent:   totalSent,
		pricePerMsg: pricePerMsg,
		currency:    "USD",
	}
}

func (f *EstimatedFetcher) Fetch(_ context.Context) BillingResult {
	source := fmt.Sprintf("Estimated (DB records × $%.4f/msg)", f.pricePerMsg)
	return BillingResult{
		Provider:   f.provider,
		Currency:   f.currency,
		Period:     "all_time",
		Source:     source,
		CostPerMsg: f.pricePerMsg,
		TotalCount: f.totalSent,
		TotalCost:  float64(f.totalSent) * f.pricePerMsg,
		IsActual:   false,
	}
}

// --- Free tier ---

// FreeFetcher is used for providers with no per-message charge (FCM, OneSignal, Slack).
type FreeFetcher struct{ provider string }

func NewFreeFetcher(provider string) *FreeFetcher { return &FreeFetcher{provider} }

func (f *FreeFetcher) Fetch(_ context.Context) BillingResult {
	return BillingResult{
		Provider: f.provider,
		Currency: "USD",
		Period:   "all_time",
		Source:   "No charge (free tier)",
		IsActual: true,
	}
}
