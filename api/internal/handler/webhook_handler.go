package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// WebhookHandler receives inbound delivery callbacks from email/SMS/push providers.
type WebhookHandler struct {
	eventRepo   *repository.EventRepository
	notifRepo   *repository.NotificationRepository
	attemptRepo *repository.AttemptRepository
	webhookRepo *repository.WebhookEventRepository
	govRepo     *repository.GovernanceRepository
	cfg         *config.Config
	log         *zap.Logger
}

func NewWebhookHandler(
	eventRepo *repository.EventRepository,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	webhookRepo *repository.WebhookEventRepository,
	govRepo *repository.GovernanceRepository,
	cfg *config.Config,
	log *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		eventRepo:   eventRepo,
		notifRepo:   notifRepo,
		attemptRepo: attemptRepo,
		webhookRepo: webhookRepo,
		govRepo:     govRepo,
		cfg:         cfg,
		log:         log,
	}
}

// deliveryEvent is the normalised result of parsing a single provider callback.
type deliveryEvent struct {
	providerMsgID  string
	notificationID *uuid.UUID // set when the provider embedded our ID
	eventStr       string     // "delivered", "bounced", "failed", "opened", "opted_out"
	isHardBounce   bool
	recipient      string // email or phone; used for auto-suppression
	isSMSStop      bool
	rawPayload     map[string]any
}

// HandleProviderEvent handles POST /v1/webhooks/:provider
func (h *WebhookHandler) HandleProviderEvent(c *gin.Context) {
	h.handleWebhook(c, false)
}

// HandleProviderCallback handles GET /v1/webhooks/:provider (Plivo, Vonage, MessageBird)
func (h *WebhookHandler) HandleProviderCallback(c *gin.Context) {
	h.handleWebhook(c, true)
}

func (h *WebhookHandler) handleWebhook(c *gin.Context, isGet bool) {
	provider := c.Param("provider")
	ctx := c.Request.Context()

	var bodyBytes []byte
	var rawPayload map[string]any
	var formValues url.Values

	if isGet {
		formValues = c.Request.URL.Query()
		rawPayload = queryToMap(formValues)
	} else {
		var err error
		bodyBytes, err = io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			respondError(c, http.StatusBadRequest, "READ_ERROR", "failed to read request body")
			return
		}
		ct := c.ContentType()
		if strings.Contains(ct, "application/x-www-form-urlencoded") {
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			_ = c.Request.ParseForm()
			formValues = c.Request.Form
			rawPayload = queryToMap(formValues)
		} else {
			// Try JSON — SendGrid sends an array; most others send an object
			_ = json.Unmarshal(bodyBytes, &rawPayload)
		}
	}

	events := h.parseProviderPayload(provider, bodyBytes, c.Request.Header, rawPayload, formValues)

	// Store one raw webhook record per HTTP call
	webhookEvent := &domain.ProviderWebhookEvent{
		ID:         uuid.New(),
		Provider:   provider,
		EventType:  "",
		RawPayload: rawPayload,
		ReceivedAt: time.Now(),
	}
	if len(events) > 0 {
		webhookEvent.EventType = events[0].eventStr
		if events[0].notificationID != nil {
			webhookEvent.NotificationID = events[0].notificationID
		}
	}
	if err := h.webhookRepo.Create(ctx, webhookEvent); err != nil {
		h.log.Error("storing webhook event", zap.Error(err))
	}

	for i := range events {
		h.processDeliveryEvent(ctx, provider, &events[i])
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

// --------------------------------------------------------------------------
// Provider payload parsers
// --------------------------------------------------------------------------

func (h *WebhookHandler) parseProviderPayload(
	provider string, body []byte, headers http.Header,
	payload map[string]any, form url.Values,
) []deliveryEvent {
	switch strings.ToLower(provider) {
	case "ses", "amazon-ses":
		return h.parseSNS(payload)
	case "mailgun":
		return h.parseMailgun(payload)
	case "sendgrid":
		return h.parseSendGrid(body)
	case "postmark":
		return h.parsePostmark(payload)
	case "twilio":
		return h.parseTwilio(payload, form, headers, c_requestURL(headers))
	case "plivo":
		return h.parsePlivo(payload)
	case "vonage", "nexmo":
		return h.parseVonage(payload)
	case "messagebird":
		return h.parseMessageBird(payload)
	case "pusher", "pusher-beams":
		return h.parsePusherBeams(body, payload, headers)
	default:
		return h.parseGeneric(payload)
	}
}

// c_requestURL reconstructs a best-effort URL from headers (for Twilio HMAC).
func c_requestURL(headers http.Header) string {
	host := headers.Get("X-Forwarded-Host")
	if host == "" {
		host = headers.Get("Host")
	}
	scheme := headers.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
	}
	path := headers.Get("X-Original-URI")
	return scheme + "://" + host + path
}

// --- AWS SES via SNS ---

func (h *WebhookHandler) parseSNS(payload map[string]any) []deliveryEvent {
	snsType := extractString(payload, "Type")

	if snsType == "SubscriptionConfirmation" {
		if subURL := extractString(payload, "SubscribeURL"); subURL != "" {
			go func() { _, _ = http.Get(subURL) }() //nolint:noctx
		}
		return nil
	}
	if snsType != "Notification" {
		return nil
	}

	msgStr := extractString(payload, "Message")
	if msgStr == "" {
		return nil
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(msgStr), &msg); err != nil {
		return nil
	}

	var ev deliveryEvent
	ev.rawPayload = msg

	// Notification ID embedded as SES email tag during send.
	// In SNS payloads, mail.tags values are []string arrays, not plain strings.
	if mail, ok := msg["mail"].(map[string]any); ok {
		// Use messageId as providerMsgID fallback for attempt lookup.
		ev.providerMsgID = extractString(mail, "messageId")

		if tags, ok := mail["tags"].(map[string]any); ok {
			var tagVal string
			// SNS serialises tag values as string arrays: {"notification-id": ["<uuid>"]}
			if arr, ok := tags["notification-id"].([]any); ok && len(arr) > 0 {
				tagVal, _ = arr[0].(string)
			} else {
				tagVal, _ = tags["notification-id"].(string)
			}
			if uid, err := uuid.Parse(tagVal); err == nil {
				ev.notificationID = &uid
			}
		}
	}

	eventType := strings.ToLower(extractString(msg, "eventType", "notificationType"))
	switch eventType {
	case "delivery":
		ev.eventStr = "delivered"
	case "bounce":
		ev.eventStr = "bounced"
		if bounce, ok := msg["bounce"].(map[string]any); ok {
			ev.isHardBounce = strings.EqualFold(extractString(bounce, "bounceType"), "permanent")
			if items, ok := bounce["bouncedRecipients"].([]any); ok && len(items) > 0 {
				if item, ok := items[0].(map[string]any); ok {
					ev.recipient = extractString(item, "emailAddress")
				}
			}
		}
	case "complaint":
		ev.eventStr = "complained"
	default:
		return nil
	}
	return []deliveryEvent{ev}
}

// --- Mailgun ---

func (h *WebhookHandler) parseMailgun(payload map[string]any) []deliveryEvent {
	signingKey := h.cfg.Providers.Email.Mailgun.WebhookSigningKey
	if signingKey == "" {
		signingKey = h.cfg.Providers.Email.Mailgun.APIKey
	}
	if signingKey != "" {
		if !verifyMailgunHMAC(payload, signingKey) {
			h.log.Warn("mailgun webhook HMAC verification failed")
		}
	}

	eventData, _ := payload["event-data"].(map[string]any)
	if eventData == nil {
		return nil
	}

	var ev deliveryEvent
	ev.rawPayload = payload

	// Notification ID from user-variables (set via message.AddVariable in sender)
	if uv, ok := eventData["user-variables"].(map[string]any); ok {
		if id := extractString(uv, "notification_id"); id != "" {
			if uid, err := uuid.Parse(id); err == nil {
				ev.notificationID = &uid
			}
		}
	}

	// Fallback: message-id header for attempt lookup
	if msg, ok := eventData["message"].(map[string]any); ok {
		if hdrs, ok := msg["headers"].(map[string]any); ok {
			ev.providerMsgID = extractString(hdrs, "message-id")
		}
	}

	ev.recipient = extractString(eventData, "recipient")
	eventStr := extractString(eventData, "event")
	severity := extractString(eventData, "severity")

	switch eventStr {
	case "delivered":
		ev.eventStr = "delivered"
	case "failed":
		ev.eventStr = "bounced"
		ev.isHardBounce = strings.EqualFold(severity, "permanent")
	case "bounced":
		ev.eventStr = "bounced"
		ev.isHardBounce = true
	case "complained":
		ev.eventStr = "complained"
	case "opened", "open":
		ev.eventStr = "opened"
	case "clicked", "click":
		ev.eventStr = "clicked"
	default:
		return nil
	}
	return []deliveryEvent{ev}
}

func verifyMailgunHMAC(payload map[string]any, signingKey string) bool {
	sig, _ := payload["signature"].(map[string]any)
	if sig == nil {
		return false
	}
	ts := extractString(sig, "timestamp")
	token := extractString(sig, "token")
	signature := extractString(sig, "signature")
	if ts == "" || token == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(ts + token))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// --- SendGrid ---

func (h *WebhookHandler) parseSendGrid(body []byte) []deliveryEvent {
	var rawEvents []map[string]any
	if err := json.Unmarshal(body, &rawEvents); err != nil {
		return nil
	}

	result := make([]deliveryEvent, 0, len(rawEvents))
	for _, raw := range rawEvents {
		var ev deliveryEvent
		ev.rawPayload = raw

		if id := extractString(raw, "notification_id"); id != "" {
			if uid, err := uuid.Parse(id); err == nil {
				ev.notificationID = &uid
			}
		}
		ev.providerMsgID = extractString(raw, "sg_message_id")
		ev.recipient = extractString(raw, "email")

		eventStr := extractString(raw, "event")
		typeField := extractString(raw, "type")
		switch eventStr {
		case "delivered":
			ev.eventStr = "delivered"
		case "bounce":
			ev.eventStr = "bounced"
			ev.isHardBounce = typeField == "bounce" // "bounce" = hard, "blocked" = soft
		case "dropped":
			ev.eventStr = "failed"
		case "deferred":
			ev.eventStr = "deferred"
		case "open":
			ev.eventStr = "opened"
		case "click":
			ev.eventStr = "clicked"
		default:
			continue
		}
		result = append(result, ev)
	}
	return result
}

// --- Postmark ---

func (h *WebhookHandler) parsePostmark(payload map[string]any) []deliveryEvent {
	var ev deliveryEvent
	ev.rawPayload = payload

	if meta, ok := payload["Metadata"].(map[string]any); ok {
		if id := extractString(meta, "notification_id"); id != "" {
			if uid, err := uuid.Parse(id); err == nil {
				ev.notificationID = &uid
			}
		}
	}
	ev.providerMsgID = extractString(payload, "MessageID")
	ev.recipient = extractString(payload, "Recipient", "Email")

	recordType := extractString(payload, "RecordType")
	switch recordType {
	case "Delivery":
		ev.eventStr = "delivered"
	case "Bounce":
		ev.eventStr = "bounced"
		ev.isHardBounce = strings.EqualFold(extractString(payload, "Type"), "HardBounce")
	case "SpamComplaint":
		ev.eventStr = "complained"
	case "Open":
		ev.eventStr = "opened"
	case "Click":
		ev.eventStr = "clicked"
	default:
		return nil
	}
	return []deliveryEvent{ev}
}

// --- Twilio ---

func (h *WebhookHandler) parseTwilio(payload map[string]any, form url.Values, headers http.Header, reqURL string) []deliveryEvent {
	// Verify HMAC-SHA1 if auth token is available and signature header is present
	if authToken := h.cfg.Providers.SMS.Twilio.AuthToken; authToken != "" {
		sig := headers.Get("X-Twilio-Signature")
		if sig != "" && reqURL != "" && !verifyTwilioHMAC(authToken, sig, reqURL, form) {
			h.log.Warn("twilio webhook HMAC verification failed")
		}
	}

	var ev deliveryEvent
	ev.rawPayload = payload

	ev.providerMsgID = getFormOrPayload(form, payload, "MessageSid")
	status := getFormOrPayload(form, payload, "MessageStatus")
	ev.recipient = getFormOrPayload(form, payload, "To")

	switch strings.ToLower(status) {
	case "delivered":
		ev.eventStr = "delivered"
	case "sent":
		ev.eventStr = "sent"
	case "failed", "undelivered":
		ev.eventStr = "failed"
	default:
		if status == "" {
			return nil
		}
		ev.eventStr = strings.ToLower(status)
	}

	// Detect STOP opt-out via OptOutType field
	if strings.EqualFold(getFormOrPayload(form, payload, "OptOutType"), "STOP") {
		ev.isSMSStop = true
		ev.eventStr = "opted_out"
	}

	return []deliveryEvent{ev}
}

func verifyTwilioHMAC(authToken, twilioSig, reqURL string, params url.Values) bool {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(reqURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(sb.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(twilioSig))
}

// --- Plivo ---

func (h *WebhookHandler) parsePlivo(payload map[string]any) []deliveryEvent {
	var ev deliveryEvent
	ev.rawPayload = payload

	ev.providerMsgID = extractString(payload, "MessageUUID")
	status := extractString(payload, "Status", "message_state")
	ev.recipient = extractString(payload, "To", "dst")

	switch strings.ToLower(status) {
	case "delivered":
		ev.eventStr = "delivered"
	case "sent":
		ev.eventStr = "sent"
	case "failed", "undelivered", "rejected":
		ev.eventStr = "failed"
	default:
		if status == "" {
			return nil
		}
		ev.eventStr = strings.ToLower(status)
	}
	return []deliveryEvent{ev}
}

// --- Vonage / Nexmo ---

func (h *WebhookHandler) parseVonage(payload map[string]any) []deliveryEvent {
	var ev deliveryEvent
	ev.rawPayload = payload

	ev.providerMsgID = extractString(payload, "messageId", "message-id")
	status := extractString(payload, "status")
	ev.recipient = extractString(payload, "to", "msisdn")
	errCode := extractString(payload, "err-code")

	switch strings.ToLower(status) {
	case "delivered":
		ev.eventStr = "delivered"
	case "buffered", "accepted":
		ev.eventStr = "sent"
	case "expired", "failed", "rejected":
		ev.eventStr = "failed"
		// err-code 9 is subscriber absent / STOP opt-out in some Vonage configurations
		if errCode == "9" {
			ev.isSMSStop = true
		}
	default:
		if status == "" {
			return nil
		}
		ev.eventStr = strings.ToLower(status)
	}
	return []deliveryEvent{ev}
}

// --- MessageBird ---

func (h *WebhookHandler) parseMessageBird(payload map[string]any) []deliveryEvent {
	var ev deliveryEvent
	ev.rawPayload = payload

	ev.providerMsgID = extractString(payload, "id")
	status := extractString(payload, "status")
	ev.recipient = extractString(payload, "recipient")

	switch strings.ToLower(status) {
	case "delivered":
		ev.eventStr = "delivered"
	case "delivery_failed", "failed":
		ev.eventStr = "failed"
	case "sent", "buffered":
		ev.eventStr = "sent"
	default:
		if status == "" {
			return nil
		}
		ev.eventStr = strings.ToLower(status)
	}
	return []deliveryEvent{ev}
}

// --- Pusher Beams ---

func (h *WebhookHandler) parsePusherBeams(body []byte, payload map[string]any, headers http.Header) []deliveryEvent {
	if secretKey := h.cfg.Providers.Push.Pusher.SecretKey; secretKey != "" {
		sig := headers.Get("X-Pusher-Signature")
		if sig != "" && !verifyPusherHMAC(body, secretKey, sig) {
			h.log.Warn("pusher beams webhook HMAC verification failed")
		}
	}

	meta, _ := payload["metadata"].(map[string]any)
	if meta == nil {
		return nil
	}

	var ev deliveryEvent
	ev.rawPayload = payload

	switch extractString(meta, "event_type") {
	case "v1.PublishToUsersAttempt":
		ev.eventStr = "sent"
	case "v1.UserNotificationAcknowledgement":
		ev.eventStr = "delivered"
	default:
		return nil
	}
	return []deliveryEvent{ev}
}

func verifyPusherHMAC(body []byte, secretKey, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// --- Generic fallback ---

func (h *WebhookHandler) parseGeneric(payload map[string]any) []deliveryEvent {
	notifIDStr := extractString(payload, "notification_id", "custom_id", "tag")
	eventTypeStr := strings.ToLower(extractString(payload, "event", "status", "Type"))

	var ev deliveryEvent
	ev.rawPayload = payload

	if notifIDStr != "" {
		if uid, err := uuid.Parse(notifIDStr); err == nil {
			ev.notificationID = &uid
		}
	}

	switch eventTypeStr {
	case "delivered", "delivery":
		ev.eventStr = "delivered"
	case "bounced", "bounce", "permanent_failure":
		ev.eventStr = "bounced"
		ev.isHardBounce = true
	case "failed", "failure":
		ev.eventStr = "failed"
	case "opened", "open":
		ev.eventStr = "opened"
	default:
		return nil
	}
	return []deliveryEvent{ev}
}

// --------------------------------------------------------------------------
// Event processing
// --------------------------------------------------------------------------

func (h *WebhookHandler) processDeliveryEvent(ctx context.Context, provider string, ev *deliveryEvent) {
	// Resolve notification ID — first from payload, then via attempt lookup
	notifID := ev.notificationID
	if notifID == nil && ev.providerMsgID != "" {
		attempt, err := h.attemptRepo.FindByProviderMsgID(ctx, ev.providerMsgID)
		if err == nil && attempt != nil {
			notifID = &attempt.NotificationID
		}
	}

	if notifID != nil {
		var domainEvt domain.EventType
		var status domain.NotificationStatus

		switch ev.eventStr {
		case "delivered":
			domainEvt = domain.EventDelivered
			status = domain.StatusDelivered
		case "bounced":
			domainEvt = domain.EventBounced
			status = domain.StatusBounced
		case "failed":
			domainEvt = domain.EventFailed
			status = domain.StatusFailed
		case "opened":
			domainEvt = domain.EventOpened
		case "opted_out":
			domainEvt = domain.EventOptedOut
			status = domain.StatusSuppressed
		case "complained":
			domainEvt = domain.EventComplained
			status = domain.StatusSuppressed
		}

		if status != "" {
			if err := h.notifRepo.UpdateStatus(ctx, *notifID, status); err != nil {
				h.log.Warn("updating notification status from webhook",
					zap.String("provider", provider),
					zap.String("notification_id", notifID.String()),
					zap.Error(err),
				)
			}
		}
		if domainEvt != "" {
			_ = h.eventRepo.Append(ctx, &domain.NotificationEvent{
				ID:             uuid.New(),
				NotificationID: *notifID,
				EventType:      domainEvt,
				Metadata:       map[string]any{"provider": provider, "event": ev.eventStr},
				CreatedAt:      time.Now(),
			})
		}
	}

	// Auto-suppress recipient on hard bounce (email)
	if ev.isHardBounce && ev.recipient != "" && h.govRepo != nil {
		_ = h.govRepo.AddSuppression(ctx, &domain.Suppression{
			ID:        uuid.New(),
			Type:      domain.SuppressionTypeEmail,
			Value:     ev.recipient,
			Reason:    fmt.Sprintf("hard_bounce from %s", provider),
			CreatedAt: time.Now(),
		})
	}

	// Auto-suppress phone on SMS STOP / opt-out
	if ev.isSMSStop && ev.recipient != "" && h.govRepo != nil {
		_ = h.govRepo.AddSuppression(ctx, &domain.Suppression{
			ID:        uuid.New(),
			Type:      domain.SuppressionTypeSMS,
			Value:     ev.recipient,
			Reason:    fmt.Sprintf("sms_stop from %s", provider),
			CreatedAt: time.Now(),
		})
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func queryToMap(v url.Values) map[string]any {
	m := make(map[string]any, len(v))
	for k, vals := range v {
		if len(vals) == 1 {
			m[k] = vals[0]
		} else {
			m[k] = vals
		}
	}
	return m
}

func getFormOrPayload(form url.Values, payload map[string]any, key string) string {
	if form != nil {
		if v := form.Get(key); v != "" {
			return v
		}
	}
	return extractString(payload, key)
}

// extractString checks multiple keys in order and returns the first non-empty string value.
func extractString(payload map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := payload[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

