# Sequence Diagrams

Two primary flows are documented here:

1. **API Flow** — a caller POSTs a notification through the HTTP API
2. **Pub/Sub Event-Driven Flow** — an external service publishes an ingress event that fans out into channel deliveries

---

## 1. API Flow — Send Notification

Covers the full lifecycle from HTTP request to final delivery confirmation, including the Temporal workflow, channel worker dispatch, and provider webhook callback.

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as API Server<br/>(Gin)
    participant NS as NotificationService
    participant Cache as Redis
    participant DB as PostgreSQL
    participant TMP as Temporal
    participant PS as Pub/Sub<br/>notifications-{channel}
    participant WRK as Channel Worker<br/>(email|sms|push|…)
    participant CB as Circuit Breaker
    participant PRV as External Provider<br/>(SES / Twilio / FCM)

    Client->>API: POST /v1/notifications<br/>Authorization: Bearer <jwt>
    API->>API: Validate JWT (middleware)
    API->>NS: Send(ctx, SendRequest)

    NS->>DB: SELECT WHERE idempotency_key = ?
    alt duplicate request
        DB-->>NS: existing Notification
        NS-->>API: 202 (cached response)
        API-->>Client: 202 {notification_id, status}
    end

    NS->>Cache: GetUserPreferences(user_id)
    Cache-->>NS: UserPreferences
    NS->>NS: IsChannelEnabled? IsInDND?
    NS->>Cache: IsRateLimited(user_id, channel, type)?
    Cache-->>NS: false

    NS->>DB: SELECT governance suppressions/opt-outs
    DB-->>NS: none

    NS->>DB: SELECT template (rendered_content)
    DB-->>NS: Template
    NS->>Cache: Cache rendered template

    NS->>DB: INSERT notification (status=pending)
    DB-->>NS: notification.id

    NS->>TMP: StartWorkflow(NotificationWorkflow, notification.id)
    TMP-->>NS: workflow started
    NS-->>API: SendResponse{notification_id, status="queued"}
    API-->>Client: 202 {notification_id, status="queued"}

    Note over TMP: Durable execution begins<br/>retry: 5 attempts, 2× backoff, max 16s

    TMP->>TMP: CheckPreferencesActivity
    TMP->>TMP: RenderTemplateActivity
    TMP->>PS: PublishToPubSubActivity<br/>Message{notificationID, channel, recipient, payload}
    PS-->>TMP: ack
    TMP->>DB: LogDeliveryActivity (status=queued)

    PS->>WRK: deliver message
    WRK->>DB: UPDATE notification SET status=sending

    WRK->>CB: Allowed(vendor)?
    CB-->>WRK: closed (allowed)

    WRK->>PRV: Send(recipient, subject, body)
    PRV-->>WRK: {provider_msg_id, success}

    WRK->>DB: INSERT notification_attempt<br/>(attempt=1, status=sent, latency_ms, provider)
    WRK->>DB: INSERT notification_event (type=sent)
    WRK->>DB: UPDATE notification SET status=sent, sent_at=now()
    WRK->>PS: ack message

    Note over PRV,API: Async — provider fires delivery callback

    PRV->>API: POST /v1/webhooks/{provider}<br/>X-Signature: hmac-sha256
    API->>API: WebhookHandler.HandleProviderEvent<br/>validate HMAC signature
    API->>DB: SELECT notification WHERE provider_msg_id = ?
    DB-->>API: Notification
    API->>DB: INSERT notification_event (type=delivered)
    API->>DB: UPDATE notification SET status=delivered, delivered_at=now()
    API-->>PRV: 200 OK
```

### Failure & Retry Path

```mermaid
sequenceDiagram
    autonumber
    participant WRK as Channel Worker
    participant CB as Circuit Breaker
    participant PRV1 as Primary Provider<br/>(e.g. SES)
    participant PRV2 as Fallback Provider<br/>(e.g. Mailgun)
    participant DB as PostgreSQL
    participant PS as Pub/Sub<br/>notifications-dlq

    WRK->>CB: Allowed(primary_vendor)?
    CB-->>WRK: open — fast fail

    WRK->>CB: Allowed(fallback_vendor)?
    CB-->>WRK: closed (allowed)

    WRK->>PRV2: Send(recipient, subject, body)
    PRV2-->>WRK: error (5xx)

    WRK->>DB: INSERT notification_attempt<br/>(status=failed, error_code, error_message)
    WRK->>CB: RecordFailure(fallback_vendor)

    Note over WRK: All providers exhausted

    WRK->>DB: UPDATE notification SET status=failed
    WRK->>DB: INSERT notification_event (type=failed)
    WRK->>PS: publish to notifications-dlq
    WRK->>PS: nack original message
```

---

## 2. Pub/Sub Event-Driven Flow — Ingress Event Fan-Out

Covers the path where an external service (e.g. an order service, auth service) publishes a domain event to the ingress topic, which the EventWorker transforms and fans out into per-channel deliveries.

```mermaid
sequenceDiagram
    autonumber
    actor ExtSvc as External Service<br/>(e.g. Order Service)
    participant PS_ING as Pub/Sub<br/>notifications-ingress
    participant EW as EventWorker
    participant NS as NotificationService
    participant DB as PostgreSQL
    participant Cache as Redis
    participant PS_CH as Pub/Sub<br/>notifications-{channel}
    participant WRK as Channel Workers<br/>(email + sms + push)
    participant CB as Circuit Breakers
    participant PRV as Providers<br/>(SES · Twilio · FCM)

    ExtSvc->>PS_ING: publish Event{<br/>  type: "order.placed",<br/>  user_id: "uuid",<br/>  payload: {order_id, amount}<br/>}

    PS_ING->>EW: deliver message

    EW->>EW: Transform event → SendRequest[]<br/>(map event type → notification type + channels)

    loop for each target channel
        EW->>NS: Send(ctx, SendRequest{channel, user_id, type, payload})

        NS->>Cache: GetUserPreferences(user_id)
        Cache-->>NS: UserPreferences
        NS->>NS: IsChannelEnabled(channel)?

        alt channel disabled or DND window
            NS-->>EW: skip channel
        end

        NS->>Cache: IsRateLimited(user_id, channel, type)?
        Cache-->>NS: false

        NS->>DB: SELECT governance suppressions
        DB-->>NS: none

        NS->>DB: SELECT template for {type, channel}
        DB-->>NS: RenderedContent{subject, body}

        NS->>DB: INSERT notification (status=pending)
        NS->>PS_CH: publish Message{notificationID, channel, recipient, payload}
        NS->>DB: UPDATE notification SET status=queued
    end

    EW->>PS_ING: ack message

    Note over PS_CH,PRV: Parallel delivery across channels

    par Email delivery
        PS_CH->>WRK: email worker receives message
        WRK->>CB: Allowed(ses)?
        CB-->>WRK: closed
        WRK->>PRV: SES.Send(email, subject, body)
        PRV-->>WRK: success
        WRK->>DB: UPDATE status=sent + INSERT attempt + INSERT event
    and SMS delivery
        PS_CH->>WRK: sms worker receives message
        WRK->>CB: Allowed(twilio)?
        CB-->>WRK: closed
        WRK->>PRV: Twilio.Send(phone, message)
        PRV-->>WRK: success
        WRK->>DB: UPDATE status=sent + INSERT attempt + INSERT event
    and Push delivery
        PS_CH->>WRK: push worker receives message
        WRK->>Cache: GetDeviceToken(user_id)
        Cache-->>WRK: device_token
        WRK->>CB: Allowed(fcm)?
        CB-->>WRK: closed
        WRK->>PRV: FCM.Send(token, title, body)
        PRV-->>WRK: success
        WRK->>DB: UPDATE status=sent + INSERT attempt + INSERT event
    end
```

---

## 3. OTP Flow

Short-lived, service-to-service path used for phone verification.

```mermaid
sequenceDiagram
    autonumber
    actor SVC as Internal Service
    participant API as API Server
    participant OS as OTPService
    participant Cache as Redis<br/>(TTL = expiry_seconds)
    participant PS as Pub/Sub<br/>notifications-otp
    participant WRK as SMSWorker
    participant PRV as SMS Provider<br/>(Twilio)
    actor User

    SVC->>API: POST /v1/otp/send<br/>X-Service-Token: <token><br/>{user_id, phone_number, purpose}
    API->>API: Validate service token (middleware)
    API->>OS: SendOTP(ctx, req)

    OS->>OS: Generate 6-digit OTP
    OS->>Cache: SET otp:{user_id}:{purpose} = {hash(otp), attempts=0}<br/>EXPIRE expiry_seconds (default 300)
    OS->>PS: publish Message{channel=otp, recipient=phone, payload={otp}}
    OS-->>API: OTPSendResponse{otp_id, expiry_at}
    API-->>SVC: 200 {otp_id, expiry_at}

    PS->>WRK: deliver message
    WRK->>PRV: Twilio.Send(phone, "Your OTP is: 123456")
    PRV-->>WRK: delivered
    WRK->>Cache: ack
    PRV->>User: SMS "Your OTP is: 123456"

    Note over User,SVC: User enters OTP on client

    User->>SVC: submit OTP
    SVC->>API: POST /v1/otp/verify<br/>{user_id, purpose, otp}
    API->>OS: VerifyOTP(ctx, req)
    OS->>Cache: GET otp:{user_id}:{purpose}
    Cache-->>OS: {hash, attempts}

    alt attempts >= 3
        OS-->>API: error: too many attempts
        API-->>SVC: 400 {code: "otp_locked"}
    end

    OS->>OS: bcrypt.Compare(otp, hash)

    alt invalid OTP
        OS->>Cache: INCR otp:{user_id}:{purpose}:attempts
        OS-->>API: verified: false
        API-->>SVC: 200 {verified: false}
    else valid OTP
        OS->>Cache: DEL otp:{user_id}:{purpose}
        OS-->>API: verified: true
        API-->>SVC: 200 {verified: true}
    end
```

---

## 4. Webhook Provider Callback Flow

Handles asynchronous delivery confirmations (bounces, delivery receipts) pushed by email/SMS providers.

```mermaid
sequenceDiagram
    autonumber
    participant PRV as External Provider<br/>(SES / Twilio / Mailgun)
    participant API as WebhookHandler
    participant DB as PostgreSQL

    PRV->>API: POST /v1/webhooks/{provider}<br/>X-Signature: hmac-sha256=<sig><br/>{event_type, message_id, timestamp, status}

    API->>API: Validate HMAC-SHA256 signature<br/>against vendor secret

    alt invalid signature
        API-->>PRV: 401 Unauthorized
    end

    API->>DB: SELECT notification<br/>WHERE provider_msg_id = message_id
    DB-->>API: Notification{id, status}

    API->>DB: INSERT notification_webhook_events<br/>{provider, event_type, payload, processed_at}

    alt event_type = delivered
        API->>DB: UPDATE notification SET status=delivered, delivered_at=now()
        API->>DB: INSERT notification_event (type=delivered)
    else event_type = bounced OR failed
        API->>DB: UPDATE notification SET status=bounced
        API->>DB: INSERT notification_event (type=bounced)
    else event_type = complaint
        API->>DB: INSERT governance suppression<br/>(auto-suppress recipient)
    end

    API-->>PRV: 200 OK
```
