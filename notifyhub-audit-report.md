# NotifyHub — Security & Implementation Audit Report
**Generated:** 2026-05-02
**Audited By:** Coding Agent
**Codebase Path:** /Users/spidey/personal/notification-service

---

## Executive Summary
NotifyHub implements a solid foundation with clear channel abstractions, idempotency guarantees, and resilient workflow orchestration via Temporal. Authentication and Role-Based Access Control (RBAC) are mostly comprehensive. However, critical gaps exist in tenant isolation: managers and API key callers can bypass client scoping when dispatching notifications. Additionally, PII leakage in application logs, missing dead-letter queue (DLQ) implementations, and inconsistent retry logic across dispatch modes require immediate attention before production release.

---

## Findings

### [SECURITY] Cross-Tenant Notification Dispatch (Bypass Client Scope)
| Field | Detail |
|---|---|
| **Severity** | Critical |
| **Location** | `api/internal/service/notification_service.go:392-396` & `api/internal/handler/notification_handler.go:61-68` |
| **Description** | `POST /v1/notifications` does not enforce the `RequireClientScope` middleware for non-admin users (`manager`, `dev`). For API key callers, the `ClientID` provided in the JSON payload blindly overrides the queue scoping (`schedAPIKeyID`) instead of enforcing the caller's assigned `n.APIKeyID`. This allows callers to inject workflows into another tenant's queue or dispatch global notifications without authorization. |
| **Evidence** | `scopeID = *initialReq.ClientID; if parsed, err := uuid.Parse(scopeID); err == nil { schedAPIKeyID = &parsed }` |
| **Recommendation** | Apply `RequireClientScope(deps.UserRepo)` to `notif.POST("")` and remove the ability for API payload `client_id` to override the authenticated API key's scope unless the caller is a global Admin. |

### [SECURITY] Information Leakage on Internal Errors
| Field | Detail |
|---|---|
| **Severity** | High |
| **Location** | `api/internal/handler/middleware.go:478-479` |
| **Description** | When an unmapped application error (HTTP 500) occurs, the raw `err.Error()` string is passed to `respondError` and returned to the client. This exposes internal stack traces, DB connection strings, and potentially provider credentials. |
| **Evidence** | `respondError(c, status, code, err.Error())` |
| **Recommendation** | Sanitize 500 error responses. Return a generic "Internal Server Error" message to the client while logging the raw `err.Error()` internally. |

### [SECURITY] PII Leakage in Application Logs and Webhook Payloads
| Field | Detail |
|---|---|
| **Severity** | High |
| **Location** | `api/internal/worker/base_worker.go:210`, `api/internal/handler/webhook_handler.go:88` |
| **Description** | Recipient emails and phone numbers are logged in plain text via `zap.String("recipient", n.Recipient)` when suppressed. Furthermore, raw inbound webhooks from providers containing full PII are dumped verbatim into the `provider_webhook_events` table without masking. |
| **Evidence** | `zap.String("recipient", n.Recipient)` and `webhookEvent.RawPayload = rawPayload` |
| **Recommendation** | Implement a PII redactor (masking all but last 4 digits of phone, or `***@domain.com` for email) for logging. Strip sensitive PII from `rawPayload` before persisting webhook events. |

### [SECURITY] Hardcoded Secrets in Configuration Files
| Field | Detail |
|---|---|
| **Severity** | Medium |
| **Location** | `api/config/config.yaml` |
| **Description** | Default configuration contains sensitive hardcoded credentials including `clerk.secret_key`, `jwt.secret`, `admin.password`, `security.vendor_config_encryption_key`, and a database DSN with credentials (`notif:notif`). |
| **Evidence** | `vendor_config_encryption_key: "ENCRYPTION_KEY_HERE_PLEASE_CHANGE_IN_PRODUCTION"` |
| **Recommendation** | Remove hardcoded values from committed configurations. Rely strictly on `viper.AutomaticEnv()` (e.g. `NS_JWT_SECRET`) or a secrets manager in all environments. |

### [IMPLEMENTATION] Missing Dead-Letter Queue (DLQ) and Inconsistent Retries
| Field | Detail |
|---|---|
| **Severity** | High |
| **Location** | `api/internal/pubsub/kafka.go:166-175`, `api/internal/worker/base_worker.go:331` |
| **Description** | The standalone pubsub implementation has no maximum retry bounded loop, no exponential backoff, and no Dead-Letter Queue. In Kafka mode, failing to commit a message acts as a nack, but a subsequent success on the same partition implicitly commits skipped messages. All attempts record `attemptNum = 1` rather than tracking increments. |
| **Evidence** | `_ = w.attemptRepo.RecordAttemptFromResult(ctx, notifID, 1, result)` and `// To avoid infinite loops... implement a DLQ` |
| **Recommendation** | Implement a strict max-retry loop in pubsub with an explicit fallback to a dead-letter table. Track real `attemptNum` dynamically in both pubsub and Temporal worker loops. |

### [IMPLEMENTATION] Preference Resolution at Submission vs. Dispatch
| Field | Detail |
|---|---|
| **Severity** | Medium |
| **Location** | `api/internal/service/notification_service.go:446-450` |
| **Description** | For standalone scheduled notifications (non-Temporal mode), user preferences and opt-out statuses are evaluated during the initial `Send()` submission. If a user opts out during the delay period, the notification will still be dispatched. |
| **Evidence** | `time.Sleep(delay); s.publishImmediate(context.Background(), &nCopy)` |
| **Recommendation** | Move preference resolution into the actual worker dispatch loop (`BaseWorker.dispatch`) so it is checked directly at send time for all pubsub tasks. |

### [IMPLEMENTATION] Sequential Channel Dispatch
| Field | Detail |
|---|---|
| **Severity** | Low |
| **Location** | `api/internal/service/notification_service.go:96-107` |
| **Description** | Multi-channel fan-out loops synchronously (`for _, ch := range req.Channels`), calling `notifRepo.Create` and `publishImmediate` sequentially. A timeout or latency spike in one channel's setup blocks the initialization of subsequent channels. |
| **Evidence** | `resp, err := s.sendToChannel(...)` executed within a blocking `for` loop. |
| **Recommendation** | Spawn goroutines or dispatch parallel workflow triggers for multi-channel sends. |

### [IMPLEMENTATION] Incomplete Status Polling Methods
| Field | Detail |
|---|---|
| **Severity** | Informational |
| **Location** | `api/internal/provider/sms/messagebird.go:91`, `api/internal/provider/email/mailgun.go:99`, etc. |
| **Description** | The `Sender` interface defines `GetStatus(ctx, providerMsgID)`, but many providers (Mailgun, SendGrid, Postmark, MessageBird, Plivo, OneSignal) fail with "not configured for status polling". |
| **Evidence** | `ErrorMessage: "mailgun not configured for status polling"` |
| **Recommendation** | Remove `GetStatus` from the base interface or clearly document that these providers rely entirely on inbound webhooks for receipt tracking. |

---

## RBAC Coverage Map
| Endpoint / Function | Auth Required | Role/Permission Check | Verdict |
|---|---|---|---|
| `POST /v1/notifications` | Yes | `admin, manager, dev, api_key` | ❌ Missing (Tenant scoping gap) |
| `POST /v1/notifications/bulk` | Yes | `admin, manager, dev, api_key` | ❌ Missing (Tenant scoping gap) |
| `GET /v1/notifications` | Yes | `admin, manager, dev, support, api_key` | ✅ Compliant |
| `GET /v1/notifications/:id` | Yes | `admin, manager, dev, support, api_key` | ✅ Compliant |
| `POST /v1/notifications/:id/retrigger` | Yes | `admin, manager, dev, api_key` | ✅ Compliant |
| `PATCH /v1/notifications/:id/schedule` | Yes | `admin, manager, dev` | ✅ Compliant |
| `POST /v1/otp/send` | Yes | `ServiceAuth` | ✅ Compliant |
| `POST /v1/webhooks/:provider` | No | Signature/HMAC Verification | ✅ Compliant |
| `GET /v1/admin/config/vendors` | Yes | `admin, manager, dev` + `RequireClientScope` | ✅ Compliant |
| `PUT /v1/admin/config/vendors/*` | Yes | `admin, manager, dev` + `RequireClientScope` | ✅ Compliant |
| `GET /v1/governance/*` | Yes | `admin, support` | ✅ Compliant |

---

## Channel Implementation Checklist
| Channel | Interface Implemented | Parallel Dispatch | Isolated Failure | Receipt Recorded | Retry Logic | Verdict |
|---|---|---|---|---|---|---|
| Email (Mailgun) | ✅ | ❌ | ✅ | ✅ | ❌ | ⚠️ Gap |
| Email (AWS SES) | ✅ | ❌ | ✅ | ✅ | ❌ | ⚠️ Gap |
| SMS (Plivo) | ✅ | ❌ | ✅ | ✅ | ❌ | ⚠️ Gap |
| SMS (Twilio) | ✅ | ❌ | ✅ | ✅ | ❌ | ⚠️ Gap |
| Push (FCM) | ✅ | ❌ | ✅ | ✅ | ❌ | ⚠️ Gap |
| Web (WebSocket) | ✅ | ❌ | ✅ | ✅ | ❌ | ⚠️ Gap |

*(Note: Retry logic is universally marked as a gap due to hardcoded `attemptNum = 1` and missing DLQ/backoff in pubsub mode. Parallel dispatch is sequential in the main submission loop for all channels.)*

---

## Gap Summary Table
| # | Severity | Category | Title | Location |
|---|---|---|---|---|
| 1 | Critical | Security | Cross-Tenant Notification Dispatch (Bypass Client Scope) | `notification_service.go` |
| 2 | High | Security | Information Leakage on Internal Errors | `middleware.go` |
| 3 | High | Security | PII Leakage in Application Logs and Webhook Payloads | `base_worker.go`, `webhook_handler.go` |
| 4 | High | Implementation | Missing Dead-Letter Queue (DLQ) and Inconsistent Retries | `kafka.go`, `base_worker.go` |
| 5 | Medium | Security | Hardcoded Secrets in Configuration Files | `config.yaml` |
| 6 | Medium | Implementation | Preference Resolution at Submission vs. Dispatch | `notification_service.go` |
| 7 | Low | Implementation | Sequential Channel Dispatch | `notification_service.go` |
| 8 | Informational | Implementation | Incomplete Status Polling Methods | `provider/*/*.go` |

---

## Recommendations — Priority Order

1. **[Critical] Enforce strict tenant boundaries in `/v1/notifications` (Medium Effort)** 
   Apply the `RequireClientScope` middleware to `POST /v1/notifications` and reject payloads that attempt to inject a `client_id` for workflow scoping that does not match the authenticated API key or manager's authorized subset.
2. **[High] Redact PII in logs and raw database stores (Medium Effort)** 
   Sanitize `recipient` emails and phone numbers before injecting them into `zap` loggers. Exclude raw payloads from the DB or anonymize specific fields before calling `webhookRepo.Create()`.
3. **[High] Mask 500 API responses (Small Effort)** 
   Update `respondDomainError` in `middleware.go` so fallback internal errors return a static error message instead of mapping `err.Error()` to the response body.
4. **[High] Implement PubSub DLQ & Accurate Attempt Tracking (Large Effort)** 
   Pass and increment `attemptNum` accurately across workers instead of hardcoding `1`. Configure a Dead-Letter Queue topic to offload continuously failing pubsub messages, stopping Kafka uncommitted implicit commit bypasses.
5. **[Medium] Move Preference Validation to Dispatch Time (Small Effort)** 
   Remove standalone timed sleep loops checking preferences prematurely; move `IsChannelEnabled` checks natively into the PubSub consumer's dispatch pipeline.
6. **[Medium] Eradicate Hardcoded Keys (Small Effort)** 
   Replace committed strings in `config.yaml` with placeholder values (e.g. `<SET_IN_ENV>`) to ensure keys cannot leak and require explicit injection.
7. **[Low] Parallelize Multi-Channel Enqueuing (Medium Effort)** 
   Refactor `Send()` to iterate and enqueue channel requests via a `sync.WaitGroup` to isolate transient enqueue failures and latency.