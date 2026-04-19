# Designing a Scalable Notifications System
## SMS | OTP | Email | Push | WebSocket | Webhook — HLD & LLD

> Based on: *System Design 15: Design Scalable Notifications System | HLD | LLD*

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Functional Requirements](#2-functional-requirements)
3. [Non-Functional Requirements](#3-non-functional-requirements)
4. [Scale Estimation](#4-scale-estimation)
5. [High-Level Design (HLD)](#5-high-level-design-hld)
6. [Core Components Deep Dive](#6-core-components-deep-dive)
7. [Pub/Sub Architecture](#7-pubsub-architecture)
8. [Cadence Workflow Orchestration](#8-cadence-workflow-orchestration)
9. [Low-Level Design (LLD)](#9-low-level-design-lld)
10. [Channel-Specific Design](#10-channel-specific-design)
11. [Database Schema](#11-database-schema)
12. [API Design](#12-api-design)
13. [Retry, Idempotency & Dead-Letter](#13-retry-idempotency--dead-letter)
14. [Rate Limiting & User Preferences](#14-rate-limiting--user-preferences)
15. [Reliability & Fault Tolerance](#15-reliability--fault-tolerance)
16. [Scalability Strategies](#16-scalability-strategies)
17. [Security](#17-security)
18. [Monitoring & Observability](#18-monitoring--observability)
19. [Technology Stack Reference](#19-technology-stack-reference)

---

## 1. Problem Statement

Build a **platform-agnostic, multi-channel notification service** that reliably delivers millions of notifications per day across:

- **SMS** — transactional and promotional text messages
- **OTP** — time-sensitive one-time passwords (login, 2FA, payment confirmation)
- **Email** — rich-content transactional and marketing emails
- **Push** — mobile/web push using FCM, APNs, and Pushwoosh
- **Web Notifications** — in-app/browser real-time notifications via WebSocket
- **Webhook Notifications** — server-to-server event callbacks to partner endpoints

The system must be:
- Scalable to 250M+ notifications/day at peak
- Extensible to add new channels without architectural changes
- Respectful of user preferences and regulatory constraints
- Resilient to provider outages and transient failures

---

## 2. Functional Requirements

| #     | Requirement                                                                       |
| ----- | --------------------------------------------------------------------------------- |
| FR-1  | Accept notification requests from multiple internal services and external clients |
| FR-2  | Support six channels: SMS, OTP, Email, Push, WebSocket, Webhook                        |
| FR-3  | Route to correct channel(s) based on notification type and user preferences       |
| FR-4  | Support scheduled (future) and immediate delivery                                 |
| FR-5  | Support bulk/broadcast notifications with user segment filtering                  |
| FR-6  | Track delivery status per notification per channel                                |
| FR-7  | Allow users to manage opt-in/opt-out and do-not-disturb windows                   |
| FR-8  | Support notification templates with variable substitution                         |
| FR-9  | Retry on failure with configurable backoff                                        |
| FR-10 | Provide delivery receipts and audit logs                                          |

---

## 3. Non-Functional Requirements

| # | Requirement | Target |
|---|-------------|--------|
| NFR-1 | **Availability** | 99.99% uptime |
| NFR-2 | **OTP Latency** | < 2s end-to-end delivery |
| NFR-3 | **Email/SMS Latency** | < 10s for transactional; minutes for bulk |
| NFR-4 | **Throughput** | 17,000+ notifications/second at peak |
| NFR-5 | **Durability** | Zero message loss (at-least-once delivery) |
| NFR-6 | **Idempotency** | No duplicate deliveries on retries |
| NFR-7 | **Extensibility** | New channels added with no core changes |
| NFR-8 | **Observability** | Full delivery audit trail |

---

## 4. Scale Estimation

```
Daily Active Users (DAU):        50,000,000
Notifications per user per day:   5
Total daily notifications:       250,000,000

Peak factor (10x average):
  Average throughput:  250M / 86,400s  ≈  2,893 /sec
  Peak throughput:     2,893 × 10      ≈  28,935 /sec  (target: 17K–30K/sec)

Storage (per notification row ~1KB):
  Daily log storage:   250M × 1KB     = 250 GB/day
  Monthly:             ~7.5 TB/month
  User preferences:    50M × 1KB      = 50 GB

Queue throughput (Pub/Sub):
  Assume 4 channels per user avg:
  Queue messages/sec at peak:    ~116,000 /sec → 6 topics (otp/email/sms/push/websocket/webhook)
  Pub/Sub sustained throughput:  1 GB/s per topic (well within limits)
```

---

## 5. High-Level Design (HLD)

```
                         ┌─────────────────────────────────────────────┐
                         │              NOTIFICATION CLIENTS            │
                         │  Internal Services │ External API Consumers  │
                         └────────────────────┬────────────────────────┘
                                              │ HTTPS / gRPC
                                              ▼
                         ┌────────────────────────────────┐
                         │         API Gateway            │
                         │  (Auth, Rate Limit, Routing)   │
                         └───────────────┬────────────────┘
                                         │
                                         ▼
                         ┌────────────────────────────────┐
                         │       Notification Service     │
                         │  Validate → Prioritize → Start │
                         │  Cadence Workflow               │
                         │  (immediate OR delayed start)  │
                         └──┬──────────┬────────────┬─────┘
                            │          │            │
                     ┌──────▼──┐ ┌─────▼───┐ ┌─────▼──────┐
                     │  Redis  │ │  User   │ │  Template  │
                     │  Cache  │ │  Prefs  │ │   Engine   │
                     └─────────┘ └─────────┘ └────────────┘
                                         │
                    ┌────────────────────▼───────────────────────┐
                    │          Cadence Workflow Engine            │
                    │  ┌──────────────────────────────────────┐  │
                    │  │   NotificationWorkflow (per message) │  │
                    │  │   ├─ Activity: CheckPreferences      │  │
                    │  │   ├─ Activity: RenderTemplate        │  │
                    │  │   ├─ Activity: PublishToPubSub       │  │
                    │  │   ├─ Activity: AwaitDeliveryReceipt  │  │
                    │  │   └─ Activity: LogResult             │  │
                    │  └──────────────────────────────────────┘  │
                    │  ┌──────────────────────────────────────┐  │
                    │  │   BulkNotificationWorkflow           │  │
                    │  └──────────────────────────────────────┘  │
                    └────────────────────┬───────────────────────┘
                                         │ publishes
                           ┌─────────────▼──────────────────┐
                           │    Google Cloud Pub/Sub         │
                           │ ┌──────┐ ┌─────┐ ┌─────┐ ┌─────┐ ┌──────┐ ┌──────┐│
                           │ │ otp  │ │email│ │ sms │ │push │ │websoc│ │webhok││
                           │ │topic │ │topic│ │topic│ │topic│ │ topic│ │ topic││
                           │ └──┬───┘ └──┬──┘ └──┬──┘ └──┬──┘ └──┬───┘ └──┬───┘│
                           └────┼────────┼────────┼────────┼───────┼────────┼────┘
                                │        │        │        │       │        │
              ┌─────────────────┼────────┼────────┼────────┼───────┼────────┼──────────┐
              │                 ▼        ▼        ▼        ▼       ▼        ▼          │
              │       ┌────────┐ ┌──────┐ ┌─────┐ ┌───────┐ ┌──────────┐ ┌──────────┐  │
              │       │  OTP   │ │Email │ │ SMS │ │ Push  │ │WebSocket │ │ Webhook  │  │
              │       │Worker  │ │Worker│ │Workr│ │Worker │ │  Worker  │ │  Worker  │  │
              │       └───┬────┘ └──┬───┘ └──┬──┘ └──┬────┘ └────┬─────┘ └────┬─────┘  │
              │           │         │         │       │           │            │        │
              │    ┌──────▼─┐ ┌─────▼───┐ ┌───▼────┐ ┌────▼─────┐ ┌────▼─────┐ ┌──▼────┐│
              │    │Twilio /│ │ SES /   │ │Twilio/ │ │FCM/APNs/ │ │ WS Hub / │ │Partner││
              │    │ Plivo  │ │Mailgun/ │ │Plivo/  │ │Pushwoosh │ │ Gateway  │ │Webhook││
              │    │        │ │  SMTP   │ │Vonage  │ │          │ │          │ │ APIs  ││
              │    └────────┘ └─────────┘ └────────┘ └──────────┘ └──────────┘ └───────┘│
              │                                                       │
              │              ┌──────────────────┐                    │
              │              │  Notification Log │                    │
              │              │    (PostgreSQL)   │                    │
              │              └──────────────────┘                    │
              └───────────────────────────────────────────────────────┘
                                   WORKER LAYER
```

### Data Flow Summary

**Immediate delivery:**
```
1.  Client sends POST /notifications  (scheduledAt: null)
2.  API Gateway authenticates & rate-limits
3.  Notification Service validates payload
4.  Starts Cadence NotificationWorkflow immediately
5.  Cadence Activity: CheckPreferences — verify opt-in, DND window
6.  Cadence Activity: RenderTemplate  — render channel-specific content
7.  Cadence Activity: PublishToPubSub — publish to channel Pub/Sub topic
8.  Pub/Sub delivers to Channel Worker subscriber
9.  Channel Worker calls provider API (Amazon SES/Mailgun/SMTP, Twilio/Plivo/Vonage, FCM/APNs/Pushwoosh, WebSocket gateway, webhook endpoint) via circuit-breaker-wrapped sender
10. Cadence Activity: LogResult — write delivery status to notification_logs
11. On transient failure → Cadence retries Activity with exponential backoff
12. On max retries exhausted → Dead-Letter Topic; workflow fails
```

**Scheduled delivery (e.g. "send at 2pm tomorrow"):**
```
1.  Client sends POST /notifications  (scheduledAt: "2026-04-18T14:00:00Z")
2.  API Gateway authenticates & rate-limits
3.  Notification Service persists to scheduled_notifications (status=PENDING)
4.  StartWorkflow with DelayStartSeconds = seconds until deliverAt
      WorkflowID = "sched-notif-{notificationId}"  ← deterministic
      No execution is running — Cadence server holds a pending start entry
5.  Returns 202 { status: PENDING, scheduledAt, workflowId }
── time passes; NO open workflow execution, NO history, safe to deploy ──
6.  DelayStartSeconds elapses → Cadence starts NotificationWorkflow fresh
7.  Same steps 5–12 as immediate delivery above (current deployed code)
```

**Edit / Cancel:**
```
PATCH  /v1/notifications/{id}/schedule → TerminateWorkflow + StartWorkflow with new DelayStartSeconds
DELETE /v1/notifications/{id}/schedule → TerminateWorkflow (workflow was never running)
```

---

## 6. Core Components Deep Dive

### 6.1 API Gateway

Responsibilities:
- **Authentication**: OAuth 2.0 / API Key validation
- **Rate Limiting**: Per-client plan enforcement (free vs paid tiers)
- **Request Routing**: Routes to Notification Service
- **DoS Protection**: Blocks burst abuse at entry point

### 6.2 Notification Service

The orchestrator. It:
1. Validates the incoming payload (required fields, valid channels)
2. Looks up **User Preferences** — is the user opted in? DND window active?
3. Calls the **Template Engine** to render the message body per channel
4. Assigns **priority** based on notification type:

| Type | Priority | Max Allowed Delay |
|------|----------|-------------------|
| OTP | HIGH | < 2 seconds |
| Transactional (order update, alert) | MEDIUM | < 30 seconds |
| Promotional / Marketing | LOW | Minutes acceptable |

5. Starts a **Cadence `NotificationWorkflow`** with the notification as input
6. Returns `202 Accepted` with a `notificationId` and Cadence `workflowId` for status tracking

### 6.3 User Preferences Service

- Stored in **NoSQL** (DynamoDB / MongoDB) for fast key-value access
- Cached in **Redis** (TTL ~5 minutes) to avoid DB hit on every notification
- Fields per user:

```json
{
  "userId": "u-123",
  "channels": {
    "email": true,
    "sms": true,
    "push": true
  },
  "doNotDisturb": {
    "enabled": true,
    "startHour": 22,
    "endHour": 8,
    "timezone": "Asia/Kolkata"
  },
  "frequencyLimits": {
    "promotional_sms": 2,
    "promotional_email": 5
  }
}
```

### 6.4 Template Engine

- Templates stored in DB with variable placeholders: `Hello {{name}}, your OTP is {{otp}}`
- Renders per-channel (email gets HTML, SMS gets plain text, push gets short title + body)
- Caches compiled templates in Redis

### 6.5 Scheduler Service

Scheduled notifications use **Cadence's native `DelayStartSeconds`** in `StartWorkflowOptions` — not cron, not `workflow.sleep`, not an external timer service.

**Why not cron?** Polling has inherent latency (up to 1 poll interval) and misses guarantees if the poller crashes.

**Why not `workflow.sleep`?** A workflow sleeping for hours stays open with an execution history. When you deploy new code, Cadence replays that history against the new code. If the workflow function changed at all, it throws a **non-determinism error**, breaking every sleeping workflow in production.

**Solution — `DelayStartSeconds` (Cadence native delayed start):**
- `StartWorkflow` is called immediately, but with `DelayStartSeconds = secondsUntilDeliverAt`
- During the delay, **no workflow execution exists** — there is no running instance, no open history, nothing to replay
- When the delay elapses, Cadence starts a **fresh** `NotificationWorkflow` using whatever code is currently deployed
- Because the workflow starts fresh (empty history), deploying new code during the wait period is completely safe
- `WorkflowID` is deterministic (`"sched-notif-{notificationId}"`), enabling cancel and reschedule by ID

See the detailed design in [Section 8](#8-cadence-workflow-orchestration).

### 6.6 Bulk Notification Service

For marketing blasts, a dedicated Cadence `BulkNotificationWorkflow` handles fan-out:
- Accepts filter criteria (e.g., "all users in India who ordered in last 30 days")
- Cadence Activity: `QueryUserSegment` — queries Elasticsearch, returns paginated user batches
- Cadence Activity: `FanOutBatch` — starts a child `NotificationWorkflow` per user (async, rate-throttled)
- Cadence's built-in concurrency limits (`MaxConcurrentActivityExecutionSize`) prevent downstream saturation
- Workflow progress is fully durable — a crash mid-fan-out resumes from the last committed batch

---

## 7. Pub/Sub Architecture

### Why Google Cloud Pub/Sub?

| Need | Pub/Sub Feature |
|------|----------------|
| High throughput (millions/day) | Serverless, auto-scales to millions of msgs/sec |
| Message durability | Messages retained up to 7 days by default |
| At-least-once delivery | Guaranteed by the platform; ack/nack model |
| Multiple independent consumers | Multiple subscriptions per topic |
| Priority lanes | Separate topics per channel (otp/email/sms/push/websocket/webhook) |
| Dead-letter handling | Native dead-letter topic per subscription |
| No infrastructure ops | Fully managed — no brokers to provision |
| Ordering within a key | Ordering keys for per-user message ordering |

### Topic Design

```
notifications-otp          ← OTP messages (highest priority consumers)
notifications-email        ← Email (transactional + promotional)
notifications-sms          ← SMS (transactional + promotional)
notifications-push         ← Push (mobile/web via FCM/APNs/Pushwoosh)
notifications-websocket    ← Real-time in-app/browser notifications
notifications-webhook      ← Outbound webhook callbacks

notifications-dlq          ← Dead-letter topic (auto-forwarded by Pub/Sub)
```

> Note: Priority lanes (HIGH/MEDIUM/LOW) are now managed by Cadence workflow
> scheduling rather than separate queue topics. Cadence controls delivery order
> and concurrency; Pub/Sub is used only as the transport to channel workers.

### Subscription Design

```
Topic                  Subscription               Subscriber
─────────────────────────────────────────────────────────────
notifications-otp   → otp-worker-sub           → OTP Worker  (pull, max 5 concurrent)
notifications-email → email-worker-sub         → Email Worker (pull, max 50 concurrent)
notifications-sms   → sms-worker-sub           → SMS Worker   (pull, max 20 concurrent)
notifications-push      → push-worker-sub          → Push Worker       (pull, max 100 concurrent)
notifications-websocket → websocket-worker-sub     → WebSocket Worker  (pull, max 200 concurrent)
notifications-webhook   → webhook-worker-sub       → Webhook Worker    (pull, max 50 concurrent)

Dead-letter config per subscription:
  max_delivery_attempts: 5
  dead_letter_topic:     notifications-dlq
```

### Message Ordering

```
Ordering key = userId
→ All notifications for a user are delivered in publish order
→ Enabled per-subscription with message ordering flag
→ Prevents out-of-order OTP delivery to the same user
```

### Pub/Sub vs Kafka Trade-off

| Concern | Pub/Sub | Kafka |
|---------|---------|-------|
| Replay past messages | Limited (7-day retention) | Unlimited (offset replay) |
| Partition control | None (managed) | Full control |
| Ops overhead | Zero | High (cluster, ZooKeeper/KRaft) |
| Exactly-once | Needs idempotency layer | Transactional API available |
| Cost model | Per-message | Per-cluster (fixed + data) |
| Ordering | Per ordering-key | Per partition |

> **Why Pub/Sub here**: Since Cadence owns retry logic, workflow state, and
> ordering guarantees, we don't need Kafka's replay or offset-level control.
> Pub/Sub's zero-ops model fits better when Cadence is the durable backbone.

---

## 8. Cadence Workflow Orchestration

Cadence is an open-source workflow engine (from Uber) that provides **durable execution** of multi-step processes. Each notification delivery is modelled as a Cadence workflow, replacing ad-hoc retry loops, cron jobs, and state machines scattered across services.

### 8.1 Why Cadence for Notifications?

| Problem | Without Cadence | With Cadence |
|---------|----------------|--------------|
| Retry with backoff | Custom retry loop in each worker | Activity retry policy on the workflow definition |
| Scheduled delivery | Cron poller vs `workflow.sleep` vs `DelayStartSeconds` | **`DelayStartSeconds`** — Cadence-native, no open execution during wait, fires at exact time, no external service |
| Scheduled edit/cancel | Signal + re-sleep vs external task delete | **`TerminateWorkflow` + new `StartWorkflow`** — synchronous, no race window; NOT_FOUND means already started (clean 409) |
| Avoiding non-determinism on deploy | Long-sleeping workflows | `DelayStartSeconds` creates no execution during wait — deploy freely; workflow starts fresh at delivery time with current code |
| Bulk fan-out progress | Job tracker in DB, manual resume | Workflow state is implicit; resumes after crash |
| Multi-step delivery (validate → render → send → log) | Distributed saga with compensation logic | Sequential activity chain in a single workflow |
| Audit trail | Manually logged at each step | Cadence event history is a complete audit log |
| Timeouts per step | Manual deadline tracking | `ScheduleToCloseTimeout` per activity |

### 8.2 Workflow Definitions

#### NotificationWorkflow (core)

```go
// Triggered for every notification (immediate or after Cadence timer wakes up)
func NotificationWorkflow(ctx workflow.Context, req NotificationRequest) error {
    ao := workflow.ActivityOptions{
        ScheduleToCloseTimeout: 30 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    16 * time.Second,
            MaximumAttempts:    5,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    // Step 1: Check user preferences & DND
    var prefs UserPreferences
    if err := workflow.ExecuteActivity(ctx, CheckPreferencesActivity, req.UserID).Get(ctx, &prefs); err != nil {
        return err
    }
    if !prefs.ChannelEnabled(req.Channel) {
        return nil // silently skip — user opted out
    }

    // Step 2: Render template
    var rendered RenderedNotification
    if err := workflow.ExecuteActivity(ctx, RenderTemplateActivity, req).Get(ctx, &rendered); err != nil {
        return err
    }

    // Step 3: Publish to Pub/Sub (channel-specific topic)
    var msgID string
    if err := workflow.ExecuteActivity(ctx, PublishToPubSubActivity, rendered).Get(ctx, &msgID); err != nil {
        return err
    }

    // Step 4: Log result
    return workflow.ExecuteActivity(ctx, LogDeliveryActivity,
        LogEntry{NotificationID: req.ID, MsgID: msgID, Channel: req.Channel}).Get(ctx, nil)
}
```

#### Scheduled Notifications — `DelayStartSeconds` (Cadence native) {#scheduled-notification-design}

No external timer service. Cadence's `StartWorkflowOptions.DelayStartSeconds` registers a pending workflow start server-side. During the delay there is **no workflow execution** — no running instance, no history to replay.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SCHEDULED NOTIFICATION FLOW                          │
│                                                                         │
│  POST /v1/notifications                                                 │
│  { scheduledAt: "2026-04-18T14:00:00Z" }                                │
│         │                                                               │
│         ▼                                                               │
│  Notification Service                                                   │
│    1. Validate + persist to scheduled_notifications (status=PENDING)    │
│    2. StartWorkflow:                                                    │
│         WorkflowID        = "sched-notif-{notificationId}" ← determ.   │
│         DelayStartSeconds = seconds until deliverAt                     │
│         Workflow          = NotificationWorkflow                        │
│    3. Store returned runID in DB (needed for cancel/reschedule)         │
│    4. Return 202 { notificationId, scheduledAt, status: PENDING }       │
│                                                                         │
│  ── time passes ────────────────────────────────────────────────────── │
│    No workflow execution exists. No history. Safe to deploy new code.   │
│                                                                         │
│  DelayStartSeconds elapses:                                             │
│    Cadence starts NotificationWorkflow fresh (empty history)            │
│    Runs with whatever code is currently deployed — zero replay risk     │
│                                                                         │
│  NotificationWorkflow activity chain runs (seconds to minutes):         │
│    CheckPreferences → RenderTemplate → PublishToPubSub → LogResult      │
└─────────────────────────────────────────────────────────────────────────┘
```

**Scheduling (Go)**:

```go
func (s *SchedulerService) Schedule(ctx context.Context, req ScheduleRequest) (string, error) {
    delaySeconds := int(time.Until(req.DeliverAt).Seconds())
    if delaySeconds < 0 {
        return "", errors.New("deliverAt is in the past")
    }

    run, err := s.cadenceClient.StartWorkflow(ctx,
        client.StartWorkflowOptions{
            ID:                           "sched-notif-" + req.NotificationID, // deterministic
            TaskList:                     "notification-default",
            ExecutionStartToCloseTimeout: 10 * time.Minute,
            // Cadence server holds a pending start entry — no execution, no history
            DelayStartSeconds:            delaySeconds,
        },
        NotificationWorkflow, req.NotificationRequest,
    )
    if err != nil {
        return "", fmt.Errorf("schedule workflow: %w", err)
    }
    return run.RunID, nil // store in DB for cancel/reschedule
}
```

**Rescheduling — why Cadence has no in-place update and what to do instead:**

Cadence has **no API to update `DelayStartSeconds` on an existing pending workflow**. There is no `RescheduleWorkflow`, no `UpdateDelay`, nothing. The only option is:

1. `TerminateWorkflow` — removes the pending entry
2. `StartWorkflow` again with a new `DelayStartSeconds` and the same deterministic `WorkflowID`

This is the intended pattern and is cheap — the pending entry has no execution history to clean up. The notification payload stays in your DB; you just re-register a new pending start.

**The critical requirement: `WorkflowIDReusePolicy`**

When you call `StartWorkflow` with the same `WorkflowID` a second time, Cadence blocks it by default. You must explicitly set `WorkflowIDReusePolicy: AllowDuplicate` to allow reuse of the same ID after termination:

```go
func (s *SchedulerService) Schedule(ctx context.Context, req ScheduleRequest) (string, error) {
    delaySeconds := int(time.Until(req.DeliverAt).Seconds())
    if delaySeconds < 0 {
        return "", errors.New("deliverAt is in the past")
    }

    run, err := s.cadenceClient.StartWorkflow(ctx,
        client.StartWorkflowOptions{
            ID:                           "sched-notif-" + req.NotificationID,
            TaskList:                     "notification-default",
            ExecutionStartToCloseTimeout: 10 * time.Minute,
            DelayStartSeconds:            delaySeconds,
            // REQUIRED for reschedule: allows reusing the same WorkflowID
            // after the prior pending entry has been terminated
            WorkflowIDReusePolicy: client.WorkflowIDReusePolicyAllowDuplicate,
        },
        NotificationWorkflow, req.NotificationRequest,
    )
    if err != nil {
        return "", fmt.Errorf("schedule workflow: %w", err)
    }
    return run.RunID, nil
}
```

**Reschedule flow:**

```go
func (s *SchedulerService) Reschedule(ctx context.Context, notifID string, newDeliverAt time.Time) error {
    workflowID := "sched-notif-" + notifID

    // Step 1: Terminate the pending entry.
    // If the delay has already elapsed and the workflow started running,
    // TerminateWorkflow succeeds but the notification is already in-flight.
    // If the workflow completed, Cadence returns NOT_FOUND.
    err := s.cadenceClient.TerminateWorkflow(ctx, workflowID, "", "rescheduled", nil)
    if err != nil {
        if isNotFoundErr(err) {
            // Delay already elapsed AND workflow has completed — too late to reschedule
            return ErrAlreadyDelivered
        }
        return fmt.Errorf("terminate for reschedule: %w", err)
    }

    // Step 2: Update DB before re-registering (idempotent on failure)
    if err := s.repo.UpdateScheduledAt(ctx, notifID, newDeliverAt); err != nil {
        return err
    }

    // Step 3: Re-register with the new delay under the same WorkflowID.
    // WorkflowIDReusePolicy: AllowDuplicate allows this even though the
    // prior execution (in TERMINATED state) shares the same ID.
    runID, err := s.Schedule(ctx, ScheduleRequest{
        NotificationID:      notifID,
        DeliverAt:           newDeliverAt,
        NotificationRequest: s.repo.GetRequest(ctx, notifID),
    })
    if err != nil {
        return fmt.Errorf("re-register schedule: %w", err)
    }

    return s.repo.UpdateRunID(ctx, notifID, runID)
}
```

**Edge case: delay elapses between Terminate and StartWorkflow**

This cannot happen. `TerminateWorkflow` is synchronous — either:
- It succeeds → the pending entry is gone, `StartWorkflow` registers a new one safely
- It returns NOT_FOUND → the workflow already completed (returned `ErrAlreadyDelivered`)

There is no state where the old pending entry is gone AND a new workflow hasn't started yet that would cause a gap.

**Cancellation:**

```go
func (s *SchedulerService) Cancel(ctx context.Context, notifID string) error {
    workflowID := "sched-notif-" + notifID

    err := s.cadenceClient.TerminateWorkflow(ctx, workflowID, "", "cancelled by user", nil)
    if err != nil {
        if isNotFoundErr(err) {
            return ErrAlreadyDelivered
        }
        return fmt.Errorf("terminate workflow: %w", err)
    }

    return s.repo.UpdateStatus(ctx, notifID, StatusCancelled)
}
```

**Summary: what Cadence does and does not support**

| Operation | Cadence API | Notes |
|-----------|------------|-------|
| Schedule for future | `StartWorkflow` + `DelayStartSeconds` | No execution open during wait |
| In-place delay update | ❌ Not supported | No such API exists |
| Reschedule | `TerminateWorkflow` → `StartWorkflow` | Cheap — no history to clean up; needs `AllowDuplicate` policy |
| Cancel | `TerminateWorkflow` | Synchronous — no race window |

**Why `DelayStartSeconds` still beats `workflow.sleep`:**

| | `workflow.sleep` | `DelayStartSeconds` |
|---|---|---|
| Workflow execution during wait | Open (has history) | None (pending entry only) |
| Deploy new code during wait | ⚠️ Non-determinism error on replay | ✅ Fresh start, no replay |
| Reschedule | Signal + cancel sleep + re-sleep (complex) | Terminate + restart (2 API calls) |
| Cancel | `CancelWorkflow` (interrupts sleep) | `TerminateWorkflow` (removes pending entry) |
| Cadence worker goroutine during wait | Held open | Zero — server-side only |

#### BulkNotificationWorkflow

```go
func BulkNotificationWorkflow(ctx workflow.Context, job BulkJob) error {
    ao := workflow.ActivityOptions{ScheduleToCloseTimeout: 10 * time.Minute}
    ctx = workflow.WithActivityOptions(ctx, ao)

    var cursor string
    for {
        // Paginate through user segments
        var batch UserBatch
        if err := workflow.ExecuteActivity(ctx, QueryUserSegmentActivity,
            QueryParams{Filter: job.Filter, Cursor: cursor}).Get(ctx, &batch); err != nil {
            return err
        }

        // Fan out — start child workflow per user (rate-throttled by Cadence)
        for _, userID := range batch.UserIDs {
            childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
                WorkflowID:             "bulk-" + job.ID + "-" + userID,
                ParentClosePolicy:      enums.PARENT_CLOSE_POLICY_ABANDON,
            })
            workflow.ExecuteChildWorkflow(childCtx, NotificationWorkflow,
                NotificationRequest{UserID: userID, TemplateID: job.TemplateID, Channel: job.Channel})
            // Note: not awaiting — fire-and-forget, Cadence tracks each child independently
        }

        if batch.NextCursor == "" {
            break // all pages exhausted
        }
        cursor = batch.NextCursor
    }
    return nil
}
```

### 8.3 Activity Definitions

```go
// Each Activity is a plain Go function — Cadence handles timeout, retry, heartbeat

func CheckPreferencesActivity(ctx context.Context, userID string) (UserPreferences, error) {
    return prefsService.Get(ctx, userID) // Redis-cached, DB fallback
}

func RenderTemplateActivity(ctx context.Context, req NotificationRequest) (RenderedNotification, error) {
    tmpl, err := templateRepo.Get(ctx, req.TemplateID)
    if err != nil { return RenderedNotification{}, err }
    return templateEngine.Render(tmpl, req.Variables)
}

func PublishToPubSubActivity(ctx context.Context, rendered RenderedNotification) (string, error) {
    topic := pubsubClient.Topic(topicFor(rendered.Channel)) // e.g. "notifications-email"
    result := topic.Publish(ctx, &pubsub.Message{
        Data:        rendered.Payload,
        OrderingKey: rendered.UserID,  // per-user ordering
        Attributes:  map[string]string{"channel": rendered.Channel, "notifId": rendered.ID},
    })
    return result.Get(ctx) // returns server-assigned messageID
}

func LogDeliveryActivity(ctx context.Context, entry LogEntry) error {
    return notifLogRepo.Insert(ctx, entry)
}
```

### 8.4 OTP Workflow

OTP has stricter latency requirements — the workflow is deliberately short-circuited for speed:

```go
func OtpNotificationWorkflow(ctx workflow.Context, req OtpRequest) error {
    ao := workflow.ActivityOptions{
        // Very tight timeout — OTP must arrive fast or fail fast
        ScheduleToCloseTimeout: 5 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            MaximumAttempts: 2,  // fail fast; don't backoff — OTP expires anyway
        },
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    // Generate & store OTP in Redis
    var otp string
    if err := workflow.ExecuteActivity(ctx, GenerateOtpActivity, req).Get(ctx, &otp); err != nil {
        return err
    }

    // Publish directly to OTP Pub/Sub topic (bypass template engine)
    rendered := RenderedNotification{
        Channel:   "otp",
        Recipient: req.PhoneNumber,
        Payload:   []byte(`Your OTP is ` + otp + `. Valid for 5 minutes.`),
        UserID:    req.UserID,
    }
    var msgID string
    if err := workflow.ExecuteActivity(ctx, PublishToPubSubActivity, rendered).Get(ctx, &msgID); err != nil {
        return err
    }

    return workflow.ExecuteActivity(ctx, LogDeliveryActivity,
        LogEntry{NotificationID: req.OtpID, MsgID: msgID, Channel: "otp"}).Get(ctx, nil)
}
```

### 8.5 Cadence Task Lists & Worker Routing

```
Task List                Workers                       Purpose
─────────────────────────────────────────────────────────────────
notification-high        OTP workers (high CPU prio)   OTP workflows + activities
notification-default     Email/SMS/Push workers        Standard notification workflows
notification-realtime    WebSocket workers             Real-time web notification fan-out
notification-webhook     Webhook workers               Outbound webhook delivery + retry
notification-bulk        Bulk workers (large memory)   BulkNotificationWorkflow
// No scheduler task list needed — DelayStartSeconds workflows start on notification-default
```

Workers register on specific task lists, allowing independent scaling per workload type.

---

## 9. Low-Level Design (LLD)

### 9.1 Core Interfaces

```java
// Channel enum
public enum Channel { EMAIL, SMS, PUSH, OTP, WEBSOCKET, WEBHOOK }

// Notification contract
public interface Notification {
    Channel getChannel();
    String getRecipient();    // email addr / phone / device token
    String getContent();
    String getNotificationId();
    Priority getPriority();
}

// Sender contract
public interface NotificationSender {
    DeliveryResult send(Notification notification);
}

// Schedulable extension
public interface SchedulableNotificationSender extends NotificationSender {
    DeliveryResult schedule(Notification notification, LocalDateTime deliverAt);
}
```

### 9.2 Channel Implementations

```java
public class EmailNotification implements Notification {
    private String to;
    private String subject;
    private String htmlBody;
    private String notificationId;
    // ... getters
}

public class SmsNotification implements Notification {
    private String phoneNumber;
    private String body;
    // ...
}

public class PushNotification implements Notification {
    private String deviceToken;
    private String title;
    private String body;
    private Map<String, String> data;
    // ...
}

public class OtpNotification implements SmsNotification {
    private String otp;
    private int expirySeconds;
    // ...
}
```

### 9.3 Factory Pattern

```java
public interface NotificationSenderFactory {
    NotificationSender getSender(Channel channel);
}

public class DefaultNotificationSenderFactory implements NotificationSenderFactory {
    private final Map<Channel, NotificationSender> senders;

    public DefaultNotificationSenderFactory(
        EmailSender emailSender,
        SmsSender smsSender,
        PushSender pushSender,
        OtpSender otpSender
    ) {
        this.senders = Map.of(
            Channel.EMAIL, emailSender,
            Channel.SMS,   smsSender,
            Channel.PUSH,  pushSender,
            Channel.OTP,   otpSender
        );
    }

    @Override
    public NotificationSender getSender(Channel channel) {
        return Optional.ofNullable(senders.get(channel))
            .orElseThrow(() -> new UnsupportedChannelException(channel));
    }
}
```

### 9.4 Dispatcher

```java
public class NotificationDispatcher {
    private final NotificationSenderFactory factory;
    private final NotificationLogger logger;

    public void dispatch(Notification notification) {
        try {
            NotificationSender sender = factory.getSender(notification.getChannel());
            DeliveryResult result = sender.send(notification);
            logger.log(notification, result);
        } catch (Exception e) {
            logger.logFailure(notification, e);
            throw new DispatchException(notification.getNotificationId(), e);
        }
    }
}
```

### 9.5 Retry — Owned by Cadence, Not the Worker

Worker code no longer implements retry loops. Retry policy is declared on the Cadence Activity options and enforced by the Cadence server:

```go
// Retry is configured once on the workflow, not in each sender
retryPolicy := &temporal.RetryPolicy{
    InitialInterval:        time.Second,      // 1s
    BackoffCoefficient:     2.0,              // 1s → 2s → 4s → 8s → 16s
    MaximumInterval:        16 * time.Second,
    MaximumAttempts:        5,
    NonRetryableErrorTypes: []string{"InvalidRecipientError", "UnsubscribedError"},
}
```

The `PublishToPubSubActivity` (and by extension the channel worker) simply returns an error on failure. Cadence schedules the next attempt automatically. Workers stay stateless and simple:

```java
// Channel worker — no retry logic needed
public DeliveryResult send(Notification notification) {
    Response resp = providerClient.send(buildRequest(notification));
    if (!resp.isSuccess()) {
        // Throw — Cadence will retry per the policy
        throw new ProviderException(resp.errorCode(), resp.errorMessage());
    }
    return DeliveryResult.success(resp.messageId());
}
```

---

## 10. Channel-Specific Design

### 9.1 Email

**Providers**: Amazon SES, Mailgun, SMTP relay

**Design Considerations**:
- HTML templates rendered server-side (Handlebars/Mustache)
- Unsubscribe token embedded in footer (CAN-SPAM/GDPR compliance)
- Bounce handling via provider webhooks → update user preferences
- SPF/DKIM/DMARC configured on sending domain

**Flow**:
```
Cadence NotificationWorkflow
  → RenderTemplateActivity (HTML + plain text fallback)
  → PublishToPubSubActivity → notifications-email topic
  → Email Worker (pull subscription) consumes message
  → Amazon SES / Mailgun API call or SMTP relay send
  → Webhook receiver for delivery events (delivered/bounced/opened)
  → LogDeliveryActivity → notification_logs
```

**Provider abstraction** (each vendor is a thin leaf — no retry logic here):
```java
public class AmazonSesEmailSender implements NotificationSender {
    public DeliveryResult send(Notification n) {
        EmailNotification email = (EmailNotification) n;
        SesRequest req = new SesRequest()
            .to(email.getTo())
            .subject(email.getSubject())
            .html(email.getHtmlBody());
        Response resp = sesClient.send(req);
        if (!resp.isSuccess()) throw new ProviderException("amazon-ses", resp.statusCode());
        return DeliveryResult.of(resp.statusCode(), resp.messageId());
    }
}
// MailgunSender and SmtpRelaySender follow the same pattern.
// The EmailSenderWithFallback (Section 15.2) wraps all three with circuit breakers.
```

---

### 9.2 SMS

**Providers**: Twilio, Plivo, Vonage

**Design Considerations**:
- Regional routing: route to local telecom partners to reduce latency and cost
- Character limits: 160 chars for GSM-7, 70 chars for Unicode (handle multi-part SMS)
- Carrier filtering: some carriers block certain keywords → content moderation hook
- Phone number normalization to E.164 format before sending

**Flow**:
```
Cadence NotificationWorkflow
  → PublishToPubSubActivity → notifications-sms topic (ordering key = userId)
  → SMS Worker (pull subscription) consumes message
  → Phone number validation + E.164 normalization
  → Regional vendor selection (based on country code)
  → Twilio/Plivo/Vonage API call
  → Delivery receipt via webhook
  → LogDeliveryActivity → notification_logs
```

**Multi-provider fallback with circuit breakers**:

See [Section 15.2](#152-circuit-breaker-per-vendor-all-channels) for the full `SmsSenderWithFallback` implementation. Each vendor (Twilio, Plivo, Vonage) has its own `CircuitBreaker` instance. The fallback chain short-circuits immediately if a vendor's breaker is already `OPEN` — no wasted HTTP call to a known-broken provider.

```
Twilio CB: CLOSED → try Twilio
  Twilio returns 503 → CB records failure → eventually trips OPEN
  Next message: CB is OPEN → skip Twilio immediately → try Plivo CB
    Plivo CB: CLOSED → try Plivo → success
```

---

### 9.3 OTP (One-Time Password)

OTP is a **special case of SMS** with critical design constraints:

**Constraints**:
- Must arrive in **< 2 seconds**
- Must **expire** (typically 30–300 seconds)
- Must be **cryptographically random** (not sequential or predictable)
- Must be **single-use** (invalidated after first successful verification)
- Must limit **attempt count** (max 3–5 guesses before lockout)
- Must limit **generation rate** (max N OTPs per phone per time window — prevent toll fraud)

**OTP Generation**:
```java
public class OtpService {
    private static final int OTP_LENGTH = 6;
    private static final int OTP_EXPIRY_SECONDS = 300;

    public String generateOtp(String userId, String purpose) {
        // Cryptographically secure random
        String otp = String.format("%06d",
            new SecureRandom().nextInt(999999));

        String key = "otp:" + userId + ":" + purpose;

        // Store in Redis with TTL
        redis.setex(key, OTP_EXPIRY_SECONDS, otp);

        // Track generation count (rate limiting)
        String rateKey = "otp:rate:" + userId + ":" + purpose;
        redis.incr(rateKey);
        redis.expire(rateKey, 3600); // 1-hour window

        return otp;
    }

    public boolean verifyOtp(String userId, String purpose, String inputOtp) {
        String key = "otp:" + userId + ":" + purpose;
        String stored = redis.get(key);

        if (stored == null) throw new OtpExpiredException();

        // Track attempts
        String attemptsKey = "otp:attempts:" + userId + ":" + purpose;
        long attempts = redis.incr(attemptsKey);
        redis.expire(attemptsKey, OTP_EXPIRY_SECONDS);

        if (attempts > 5) throw new TooManyAttemptsException();

        if (!stored.equals(inputOtp)) throw new InvalidOtpException();

        // Single-use: delete on success
        redis.del(key);
        return true;
    }
}
```

**OTP Rate Limiting (Fraud Prevention)**:
```
Max OTP requests per phone per hour:     5
Max OTP requests per IP per hour:        10
Lockout duration on breach:              1 hour
Toll fraud detection:                    flag > 3 unique phones from same IP
```

**OTP Pub/Sub & Cadence Design**:
- Dedicated `notifications-otp` Pub/Sub topic — never mixed with other channels
- `OtpNotificationWorkflow` runs on the `notification-high` Cadence task list (dedicated workers)
- Cadence retry policy: `MaximumAttempts: 2` — fail fast, no long backoff (OTP expires quickly)
- Circuit breaker on OTP providers (Resilience4j) — fail-fast, not retry with delay

---

### 9.4 Push Notifications

**Providers**:
- **Android**: Firebase Cloud Messaging (FCM)
- **iOS**: Apple Push Notification Service (APNs)
- **Cross-platform orchestration**: Pushwoosh

**Design Considerations**:
- Device tokens expire or rotate → stale token cleanup required
- Users may have **multiple devices** (phone + tablet + web)
- iOS requires separate APNs certificate per app environment (sandbox vs prod)
- Silent push vs visible push (badge/alert)
- Deep link payload for in-app navigation on tap

**Device Token Registry**:
```sql
CREATE TABLE device_tokens (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL,
    token        VARCHAR(512) NOT NULL UNIQUE,
    platform     VARCHAR(10) NOT NULL,  -- 'ios', 'android', 'web'
    app_version  VARCHAR(20),
    created_at   TIMESTAMP DEFAULT NOW(),
    last_seen_at TIMESTAMP,
    is_active    BOOLEAN DEFAULT TRUE,
    INDEX idx_user_active (user_id, is_active)
);
```

**Stale token handling**:
```java
// FCM returns 404/410 for unregistered tokens
if (fcmResponse.getError() == "UNREGISTERED") {
    deviceTokenRepo.deactivate(token);
    log.info("Deactivated stale token: {}", token);
}
```

**Fan-out for multi-device users**:
```java
public void sendPushToUser(String userId, PushPayload payload) {
    List<DeviceToken> tokens = tokenRepo.getActiveTokens(userId);

    List<CompletableFuture<DeliveryResult>> futures = tokens.stream()
        .map(token -> CompletableFuture.supplyAsync(
            () -> sendToDevice(token, payload), executor))
        .collect(toList());

    CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).join();
}
```

---

### 9.5 Web Notifications (WebSocket)

Web notifications are delivered in real time to active browser/app sessions through a persistent WebSocket connection.

**Design Considerations**:
- Connection lifecycle: heartbeat + reconnect support
- Presence tracking: map `userId -> active socket/session IDs`
- Backpressure: per-connection outbound queue limits
- Fallback behavior: if user is offline, persist notification for in-app inbox fetch

**Flow**:
```
Cadence NotificationWorkflow
  → PublishToPubSubActivity → notifications-websocket topic
  → WebSocket Worker consumes message
  → Lookup active sessions for user in Redis (presence map)
  → Push payload to all active sockets for that user
  → If no active socket: persist in inbox store for pull-on-next-login
  → LogDeliveryActivity → notification_logs
```

**Presence mapping (Redis)**:
```text
ws:presence:{userId} -> Set(socketId1, socketId2, ...)
ws:socket:{socketId} -> { userId, connectedAt, appVersion }
```

---

### 9.6 Webhook Notifications

Webhook notifications are outbound HTTP callbacks for partner systems that need machine-to-machine event delivery.

**Design Considerations**:
- Endpoint verification and HMAC signature on every webhook
- Retry with exponential backoff on 5xx / timeout
- Idempotency key in webhook headers to avoid duplicate processing
- Per-endpoint rate limiting and quarantine on repeated 4xx failures

**Flow**:
```
Cadence NotificationWorkflow
  → PublishToPubSubActivity → notifications-webhook topic
  → Webhook Worker consumes message
  → Build signed HTTP request (HMAC SHA-256)
  → POST to partner endpoint
  → 2xx: mark delivered
  → 4xx/5xx/timeout: retry via Cadence policy, then DLQ
  → LogDeliveryActivity → notification_logs
```

**Webhook request example**:
```http
POST /partner/events HTTP/1.1
Content-Type: application/json
X-Webhook-Event: order.shipped
X-Webhook-Id: wh_9f81ab
X-Webhook-Timestamp: 2026-04-18T08:30:00Z
X-Webhook-Signature: sha256=2f8d...
Idempotency-Key: notif-789
```

---

## 11. Database Schema

### 10.1 notifications Table

```sql
CREATE TABLE notifications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key  VARCHAR(128) UNIQUE NOT NULL,
    user_id          UUID NOT NULL,
    channel          VARCHAR(10) NOT NULL,  -- EMAIL, SMS, PUSH, OTP, WEBSOCKET, WEBHOOK
    priority         VARCHAR(10) NOT NULL,  -- HIGH, MEDIUM, LOW
    type             VARCHAR(50) NOT NULL,  -- 'otp', 'order_update', 'promo', etc.
    template_id      UUID,
    rendered_content JSONB,               -- {subject, body, html}
    recipient        VARCHAR(256) NOT NULL, -- email / phone / device_token / webhook_url / user_session
    status           VARCHAR(20) DEFAULT 'PENDING',
    scheduled_at     TIMESTAMP,
    created_at       TIMESTAMP DEFAULT NOW(),
    updated_at       TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_notifications_user    ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_status  ON notifications (status, updated_at);
CREATE INDEX idx_notifications_idem    ON notifications (idempotency_key);
```

### 10.2 notification_logs Table

```sql
-- Time-partitioned (monthly) for efficient queries + archival
CREATE TABLE notification_logs (
    id               UUID DEFAULT gen_random_uuid(),
    notification_id  UUID NOT NULL,
    attempt_number   INT NOT NULL DEFAULT 1,
    status           VARCHAR(20) NOT NULL,  -- SENT, FAILED, DELIVERED, BOUNCED
    provider         VARCHAR(50),           -- 'mailgun', 'twilio', 'fcm'
    provider_msg_id  VARCHAR(256),
    error_code       VARCHAR(50),
    error_message    TEXT,
    latency_ms       INT,
    created_at       TIMESTAMP DEFAULT NOW()
) PARTITION BY RANGE (created_at);

CREATE TABLE notification_logs_2026_04 PARTITION OF notification_logs
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
```

### 10.3 templates Table

```sql
CREATE TABLE notification_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) UNIQUE NOT NULL,
    channel     VARCHAR(10) NOT NULL,
    subject     VARCHAR(256),        -- email only
    body        TEXT NOT NULL,       -- Handlebars template string
    version     INT DEFAULT 1,
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMP DEFAULT NOW()
);
```

### 10.4 user_preferences Table

```sql
-- Stored in NoSQL (DynamoDB) for key-value speed
-- Schema (logical):
{
  "PK": "USER#u-123",
  "channels_enabled": ["email", "sms", "push", "websocket", "webhook"],
  "dnd_start_hour": 22,
  "dnd_end_hour": 8,
  "timezone": "Asia/Kolkata",
  "frequency_caps": {
    "promotional_sms_per_day": 2,
    "promotional_email_per_day": 5
  },
  "unsubscribed_types": ["marketing"],
  "updated_at": "2026-04-17T10:00:00Z"
}
```

### 10.5 scheduled_notifications Table

Source of truth for every scheduled notification. Cadence holds the pending start entry; this table holds the intent, current schedule, status, and the workflow IDs needed for cancel/reschedule.

```sql
CREATE TABLE scheduled_notifications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id  UUID NOT NULL UNIQUE,
    user_id          UUID NOT NULL,
    channel          VARCHAR(10) NOT NULL,
    template_id      UUID,
    template_vars    JSONB,
    scheduled_at     TIMESTAMP NOT NULL,       -- current intended delivery time
    original_at      TIMESTAMP NOT NULL,       -- first scheduled time (audit)
    cadence_workflow_id VARCHAR(256) NOT NULL, -- "sched-notif-{notificationId}"
    cadence_run_id      VARCHAR(256) NOT NULL, -- returned by StartWorkflow, used to terminate
    status           VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    --   PENDING     → DelayStartSeconds pending in Cadence, no execution open
    --   CANCELLED   → TerminateWorkflow called, pending entry removed
    --   RUNNING     → delay elapsed, NotificationWorkflow has started
    --   DELIVERED   → NotificationWorkflow completed successfully
    reschedule_count INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMP DEFAULT NOW(),
    updated_at       TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_sched_user   ON scheduled_notifications (user_id, status);
CREATE INDEX idx_sched_status ON scheduled_notifications (status, scheduled_at);
```

**No polling index needed** — there is no background scanner. Cadence fires the workflow at the right time. The index on `(status, scheduled_at)` is for the admin UI and audit queries only.

---

## 12. API Design

### 12.1 Send Notification

```
POST /v1/notifications
Authorization: Bearer <token>
Content-Type: application/json

Request:
{
  "idempotencyKey": "order-456-shipped-email",  // caller-provided dedup key
  "userId": "u-123",
  "channels": ["email", "sms"],                 // explicit channels OR omit for preference-based routing
  "type": "order_shipped",
  "templateId": "tmpl-order-shipped-v2",
  "templateVariables": {
    "name": "Alice",
    "orderId": "ORD-456",
    "trackingUrl": "https://track.example.com/ORD-456"
  },
  "scheduledAt": null                           // null = immediate; RFC3339 for future
}

Response 202 Accepted (immediate):
{
  "notificationId": "notif-789",
  "status": "QUEUED",
  "workflowId": "notif-789"
}

Response 202 Accepted (scheduled):
{
  "notificationId": "notif-789",
  "status": "PENDING",
  "scheduledAt": "2026-04-18T14:00:00Z",
  "workflowId": "sched-notif-notif-789"   // Cadence workflow ID for cancel/reschedule
}
```

### 12.2 Send OTP

```
POST /v1/otp/send
Authorization: Bearer <service-token>

Request:
{
  "userId": "u-123",
  "phoneNumber": "+919876543210",
  "purpose": "login",                  // login | payment | 2fa
  "expirySeconds": 300
}

Response 200 OK:
{
  "otpId": "otp-abc",
  "expiryAt": "2026-04-17T10:05:00Z"
  // OTP itself is NOT returned to caller — sent directly to user
}
```

### 12.3 Verify OTP

```
POST /v1/otp/verify

Request:
{
  "userId": "u-123",
  "purpose": "login",
  "otp": "483920"
}

Response 200 OK:  { "verified": true }
Response 400:     { "error": "INVALID_OTP" }
Response 410:     { "error": "OTP_EXPIRED" }
Response 429:     { "error": "TOO_MANY_ATTEMPTS" }
```

### 12.4 Get Notification Status

```
GET /v1/notifications/{notificationId}

Response 200 OK:
{
  "notificationId": "notif-789",
  "userId": "u-123",
  "channels": [
    {
      "channel": "email",
      "status": "DELIVERED",
      "providerMessageId": "sg-msg-123",
      "deliveredAt": "2026-04-17T10:00:07Z"
    },
    {
      "channel": "sms",
      "status": "SENT",
      "attempts": 1
    }
  ]
}
```

### 12.5 Reschedule a Scheduled Notification

```
PATCH /v1/notifications/{notificationId}/schedule
Authorization: Bearer <token>

Request:
{
  "scheduledAt": "2026-04-18T16:00:00Z"   // new delivery time
}

Rules:
- Only valid when notification status = PENDING
- Returns 409 ALREADY_RUNNING  if the delay already elapsed (workflow started)
- Returns 409 ALREADY_CANCELLED if status = CANCELLED

Response 200 OK:
{
  "notificationId": "notif-789",
  "status": "PENDING",
  "scheduledAt": "2026-04-18T16:00:00Z",
  "workflowId": "sched-notif-notif-789",
  "rescheduleCount": 1
}
```

**What happens internally:**
1. Load record — verify status = `PENDING`
2. Call `TerminateWorkflow("sched-notif-{id}")` — removes the Cadence pending entry
3. If Cadence returns NOT_FOUND → delay already elapsed and workflow started → return 409 ALREADY_RUNNING
4. Update `scheduled_at`, `cadence_run_id`, `reschedule_count++` in DB
5. Call `StartWorkflow` with new `DelayStartSeconds` — store new `runID`
6. Steps 2–5 use optimistic locking on `updated_at` to handle concurrent edits

### 12.6 Cancel a Scheduled Notification

```
DELETE /v1/notifications/{notificationId}/schedule
Authorization: Bearer <token>

Response 200 OK:
{
  "notificationId": "notif-789",
  "status": "CANCELLED"
}

Response 409 Conflict:
{
  "error": "ALREADY_RUNNING",
  "message": "Notification delivery already started at 2026-04-18T14:00:02Z"
}
```

**What happens internally:**
1. Load record — verify status = `PENDING`
2. Call `TerminateWorkflow("sched-notif-{id}")` — removes the Cadence pending entry
3. If Cadence returns NOT_FOUND → delay already elapsed → return 409 ALREADY_RUNNING
4. Set `status = CANCELLED` in DB

**No race condition** — unlike a Cloud Tasks delete-then-check pattern, `TerminateWorkflow` is synchronous. If it succeeds, the workflow is guaranteed not to start. If it returns NOT_FOUND, the workflow already started (delay elapsed). There is no window between "task deleted" and "task fires".

### 12.7 Update User Preferences

```
PUT /v1/users/{userId}/notification-preferences

Request:
{
  "channels": { "email": true, "sms": false, "push": true },
  "doNotDisturb": { "enabled": true, "startHour": 22, "endHour": 8 }
}
```

### 12.8 Bulk Notification

```
POST /v1/notifications/bulk

Request:
{
  "type": "promotional",
  "templateId": "tmpl-diwali-sale",
  "templateVariables": { "discount": "30%" },
  "userSegment": {
    "country": "IN",
    "lastOrderedWithinDays": 30
  },
  "channels": ["email"],
  "scheduledAt": "2026-10-28T09:00:00Z"
}

Response 202 Accepted:
{
  "bulkJobId": "bulk-job-001",
  "estimatedRecipients": 450000,
  "status": "SCHEDULED"
}
```

---

## 13. Retry, Idempotency & Dead-Letter

### 13.1 Retry Policy — Declared in Cadence, Not in Workers

Retry is configured once on the Cadence Activity options and enforced by the Cadence server. Workers are stateless and simply throw on failure.

```
Default (email/sms/push):
  Attempt 1: immediate
  Attempt 2: +1 second
  Attempt 3: +2 seconds
  Attempt 4: +4 seconds
  Attempt 5: +8 seconds
  → MaximumAttempts exceeded → Cadence marks workflow as failed
  → Pub/Sub dead-letter policy forwards message to notifications-dlq

OTP (notification-high task list):
  Attempt 1: immediate
  Attempt 2: +1 second
  → MaximumAttempts: 2 — fail fast; OTP expires quickly
  → Alert on-call immediately on failure

Non-retryable errors (no retry attempted):
  InvalidRecipientError  — bad phone/email, retrying won't help
  UnsubscribedError      — user opted out mid-flight
  OtpExpiredError        — OTP TTL passed before delivery
```

### 13.2 Idempotency

- Every notification carries an `idempotency_key` (caller-supplied or auto-generated from payload hash)
- Before delivery, worker checks Redis: `EXISTS delivered:{idempotencyKey}`
- If key exists → skip delivery, return cached result
- On successful delivery → `SET delivered:{idempotencyKey} {result} EX 86400`

```java
public DeliveryResult sendIdempotent(Notification n) {
    String key = "delivered:" + n.getIdempotencyKey();
    String cached = redis.get(key);

    if (cached != null) {
        return DeliveryResult.fromJson(cached); // already delivered
    }

    DeliveryResult result = delegate.send(n);

    if (result.isSuccess()) {
        redis.setex(key, 86400, result.toJson()); // 24h dedup window
    }

    return result;
}
```

### 13.3 Dead-Letter Handling

Two layers of dead-letter handling work in tandem:

**Layer 1 — Cadence workflow failure**:
- When all Cadence Activity retries are exhausted, the workflow transitions to `FAILED`
- Cadence event history retains the full execution log (all attempts, errors, timestamps)
- A Cadence **workflow failure listener** emits an alert and writes a failure record to `notification_logs`

**Layer 2 — Pub/Sub dead-letter topic**:
- Each Pub/Sub subscription is configured with `max_delivery_attempts: 5`
- Messages that are nacked 5 times are automatically forwarded to `notifications-dlq`
- A **DLQ Processor** service subscribes to `notifications-dlq` and:
  - Alerts on-call via PagerDuty/Slack if DLQ message rate exceeds threshold
  - Provides an admin UI for manual replay (re-publishes to original topic)
  - Auto-replays after provider incident resolution (triggered by status webhook)

```
Pub/Sub subscription config:
  dead_letter_policy:
    dead_letter_topic: projects/my-proj/topics/notifications-dlq
    max_delivery_attempts: 5
  retry_policy:
    minimum_backoff: 1s
    maximum_backoff: 16s
```

---

## 14. Rate Limiting & User Preferences

### 14.1 Multi-Level Rate Limiting

```
Level 1 — API Gateway:
  - Per API key: 1,000 req/min (default plan)
  - Burst: 200 req/10s

Level 2 — Per-User Frequency Caps (Redis):
  - promotional_sms: max 2/day
  - promotional_email: max 5/day
  - Key: rate:{userId}:{channel}:{date}
  - Increment on send, reject if > cap

Level 3 — Do-Not-Disturb:
  - Check user's timezone + DND window before dispatch
  - If in DND: queue for next allowed window (low priority messages)
  - If in DND: DROP for OTP/transactional (user expects them any time)
```

### 14.2 Redis Rate Limit Implementation

```java
public boolean isRateLimited(String userId, String channel, String type) {
    if (!isPromotional(type)) return false;

    String key = String.format("rate:%s:%s:%s", userId, channel, today());
    long cap = prefsService.getFrequencyCap(userId, channel, type);

    long count = redis.incr(key);
    if (count == 1) redis.expire(key, 86400); // set TTL on first increment

    return count > cap;
}
```

---

## 15. Reliability & Fault Tolerance

### 15.1 Multi-AZ / Multi-Region Deployment

```
Region: asia-south1 (GCP Mumbai)
  ├── Zone a: Notification Service × 3, Cadence Workers × N
  ├── Zone b: Notification Service × 3, Cadence Workers × N
  └── Zone c: Channel Workers × N

Pub/Sub:  Globally replicated by default — no config needed
Cadence:  Cadence server on Kubernetes (3 replicas), Cassandra backend (RF=3)
          Multi-cluster active-active supported for global deployments
PostgreSQL: Cloud SQL Multi-AZ with read replicas
Redis:      Redis Cluster with 3 shards × 2 replicas (Memorystore)
```

### 15.2 Circuit Breaker per Vendor (All Channels)

Every external provider call is wrapped in a **Resilience4j `CircuitBreaker`**. Each vendor gets its own breaker instance — a failure at Amazon SES does not affect the Twilio breaker, and vice versa.

#### Circuit Breaker States

```
         ┌──────────────────────────────────────────────────────┐
         │                  CIRCUIT BREAKER FSM                 │
         │                                                       │
         │   calls succeed           failure rate > threshold   │
         │  ┌─────────┐  ─────────────────────────────────────► │
         │  │  CLOSED  │                                ┌──────┐ │
         │  │(normal)  │ ◄───────────────────────────── │ OPEN │ │
         │  └─────────┘   probe succeeds      wait      └──┬───┘ │
         │                                  duration       │     │
         │                              ┌──────────────────┘     │
         │                              ▼                        │
         │                       ┌────────────┐                  │
         │                       │ HALF-OPEN  │                  │
         │                       │ (N probes) │                  │
         │                       └────────────┘                  │
         └──────────────────────────────────────────────────────┘
```

#### Tuned Configuration per Channel

```java
// Shared builder helper
private CircuitBreakerConfig configFor(int failurePct, Duration waitOpen, Duration slowThreshold) {
    return CircuitBreakerConfig.custom()
        .slidingWindowType(COUNT_BASED)
        .slidingWindowSize(20)                         // evaluate last 20 calls
        .failureRateThreshold(failurePct)              // % failures → OPEN
        .waitDurationInOpenState(waitOpen)             // wait before probing
        .permittedNumberOfCallsInHalfOpenState(3)      // probes before CLOSED
        .slowCallDurationThreshold(slowThreshold)      // count slow calls as failures
        .slowCallRateThreshold(50)                     // 50% slow → OPEN
        .recordExceptions(IOException.class, TimeoutException.class, ProviderException.class)
        .ignoreExceptions(InvalidRecipientException.class, UnsubscribedException.class)
        .build();
}

// Per-channel configs (different SLAs → different thresholds)
CircuitBreakerConfig emailConfig = configFor(50, Duration.ofSeconds(30), Duration.ofSeconds(5));
CircuitBreakerConfig smsConfig   = configFor(50, Duration.ofSeconds(20), Duration.ofSeconds(3));
CircuitBreakerConfig otpConfig   = configFor(30, Duration.ofSeconds(10), Duration.ofMillis(800)); // tightest
CircuitBreakerConfig pushConfig  = configFor(60, Duration.ofSeconds(60), Duration.ofSeconds(5));
```

#### Vendor Registry — One Breaker per Vendor

```java
@Component
public class VendorCircuitBreakerRegistry {

    private final CircuitBreakerRegistry registry;

    public VendorCircuitBreakerRegistry(CircuitBreakerRegistry registry) {
        this.registry = registry;
        register("amazon-ses", emailConfig);
        register("mailgun",   emailConfig);
        register("smtp-relay", emailConfig);
        register("twilio-sms", smsConfig);
        register("plivo",     smsConfig);
        register("vonage",    smsConfig);
        register("twilio-otp", otpConfig);
        register("plivo-otp", otpConfig);
        register("fcm",       pushConfig);
        register("apns",      pushConfig);
        register("pushwoosh", pushConfig);
        register("websocket-gateway", pushConfig);
        register("webhook-delivery", pushConfig);
    }

    public CircuitBreaker get(String vendorName) {
        return registry.circuitBreaker(vendorName);
    }

    private void register(String name, CircuitBreakerConfig config) {
        registry.circuitBreaker(name, config);
    }
}
```

#### Email — Three-Vendor Fallback Chain

```
  Amazon SES ──(OPEN)──► Mailgun ──(OPEN)──► SMTP Relay ──(OPEN)──► DLQ
```

```java
@Component
public class EmailSenderWithFallback implements NotificationSender {

    private final NotificationSender amazonSes;
    private final NotificationSender mailgun;
    private final NotificationSender smtpRelay;
    private final VendorCircuitBreakerRegistry cbRegistry;

    @Override
    public DeliveryResult send(Notification n) {
        return tryVendor("amazon-ses", () -> amazonSes.send(n))
            .or(() -> tryVendor("mailgun",    () -> mailgun.send(n)))
            .or(() -> tryVendor("smtp-relay", () -> smtpRelay.send(n)))
            .orElseThrow(() -> new AllProvidersOpenException(Channel.EMAIL, n.getNotificationId()));
    }

    private Optional<DeliveryResult> tryVendor(String vendor, Supplier<DeliveryResult> call) {
        CircuitBreaker cb = cbRegistry.get(vendor);
        if (cb.getState() == CircuitBreaker.State.OPEN) {
            log.warn("CB OPEN — skipping vendor: {}", vendor);
            return Optional.empty();
        }
        try {
            return Optional.of(
                CircuitBreaker.decorateSupplier(cb, call).get()
            );
        } catch (CallNotPermittedException e) {
            return Optional.empty(); // CB just opened during this call
        } catch (Exception e) {
            log.error("Vendor {} failed: {}", vendor, e.getMessage());
            return Optional.empty();
        }
    }
}
```

#### SMS — Three-Vendor Fallback Chain

```
  Twilio ──(OPEN)──► Plivo ──(OPEN)──► Vonage ──(OPEN)──► DLQ
```

```java
@Component
public class SmsSenderWithFallback implements NotificationSender {

    private final NotificationSender twilio;
    private final NotificationSender plivo;
    private final NotificationSender vonage;
    private final VendorCircuitBreakerRegistry cbRegistry;

    @Override
    public DeliveryResult send(Notification n) {
        return tryVendor("twilio-sms", () -> twilio.send(n))
            .or(() -> tryVendor("plivo", () -> plivo.send(n)))
            .or(() -> tryVendor("vonage", () -> vonage.send(n)))
            .orElseThrow(() -> new AllProvidersOpenException(Channel.SMS, n.getNotificationId()));
    }

    // tryVendor() same pattern as EmailSenderWithFallback
}
```

#### OTP — Two-Vendor Fallback, Fail-Fast Config

OTP breakers use tighter thresholds (30% failure rate, 10s open window) because a degraded OTP channel blocks user logins.

```
  Twilio ──(OPEN)──► Plivo ──(OPEN)──► ALERT ON-CALL IMMEDIATELY
```

```java
@Component
public class OtpSenderWithFallback implements NotificationSender {

    private final NotificationSender twilioOtp;
    private final NotificationSender plivoOtp;
    private final VendorCircuitBreakerRegistry cbRegistry;
    private final AlertService alertService;

    @Override
    public DeliveryResult send(Notification n) {
        return tryVendor("twilio-otp",  () -> twilioOtp.send(n))
            .or(() -> tryVendor("plivo-otp", () -> plivoOtp.send(n)))
            .orElseGet(() -> {
                // Both OTP providers open — page on-call immediately
                alertService.triggerP1("Both OTP providers OPEN — user login blocked");
                throw new AllProvidersOpenException(Channel.OTP, n.getNotificationId());
            });
    }
}
```

#### Push — Per-Platform Breakers (FCM / APNs / Pushwoosh)

Push providers are platform-specific — there is no strict cross-platform fallback. Instead, each gets its own breaker. When a breaker opens, the push is dropped gracefully (user will see it on next app open via in-app inbox).

```java
@Component
public class PushSenderWithCircuitBreaker implements NotificationSender {

    private final Map<String, NotificationSender> senders = Map.of(
        "android", fcmSender,
        "ios",     apnsSender,
        "web",     pushwooshSender
    );
    private final Map<String, String> platformToVendor = Map.of(
        "android", "fcm",
        "ios",     "apns",
        "web",     "pushwoosh"
    );
    private final VendorCircuitBreakerRegistry cbRegistry;

    @Override
    public DeliveryResult send(Notification n) {
        PushNotification push = (PushNotification) n;
        String vendor = platformToVendor.get(push.getPlatform());
        CircuitBreaker cb = cbRegistry.get(vendor);

        try {
            return CircuitBreaker.decorateSupplier(cb,
                () -> senders.get(push.getPlatform()).send(n)).get();
        } catch (CallNotPermittedException e) {
            // CB is OPEN — log and drop gracefully; do not retry
            log.warn("Push CB OPEN for platform={} — dropping push notif={}", push.getPlatform(), n.getNotificationId());
            return DeliveryResult.droppedDueToCbOpen(vendor);
        }
    }
}
```

#### Circuit Breaker State Transition Summary

| Vendor | Channel | Failure % → OPEN | Open Wait | Fallback |
|--------|---------|-------------------|-----------|---------|
| Amazon SES | Email | 50% / 20 calls | 30s | Mailgun → SMTP Relay → DLQ |
| Mailgun | Email | 50% / 20 calls | 30s | SMTP Relay → DLQ |
| SMTP Relay | Email | 50% / 20 calls | 30s | DLQ |
| Twilio | SMS | 50% / 20 calls | 20s | Plivo → Vonage → DLQ |
| Plivo | SMS | 50% / 20 calls | 20s | Vonage → DLQ |
| Vonage | SMS | 50% / 20 calls | 20s | DLQ |
| Twilio | OTP | 30% / 20 calls | 10s | Plivo → P1 Alert |
| Plivo | OTP | 30% / 20 calls | 10s | P1 Alert |
| FCM | Push | 60% / 20 calls | 60s | Drop gracefully |
| APNs | Push | 60% / 20 calls | 60s | Drop gracefully |
| Pushwoosh | Push | 60% / 20 calls | 60s | Drop gracefully |
| WebSocket Gateway | WebSocket | 60% / 20 calls | 30s | Retry + DLQ |
| Webhook Delivery | Webhook | 60% / 20 calls | 30s | Retry + DLQ |

#### Integration with Cadence

When `AllProvidersOpenException` is thrown from within a Cadence Activity, Cadence marks it as a non-retryable error (configured via `NonRetryableErrorTypes`). This prevents Cadence from retrying an activity that will deterministically fail because all providers are currently open. The workflow fails fast and the message lands in the dead-letter topic.

```go
RetryPolicy: &temporal.RetryPolicy{
    NonRetryableErrorTypes: []string{
        "AllProvidersOpenException",   // no point retrying — all CBs open
        "InvalidRecipientException",
        "UnsubscribedException",
    },
},
```

### 15.3 Exactly-Once Semantics

| Guarantee | Implementation |
|-----------|---------------|
| At-least-once from Pub/Sub | Explicit ack after successful delivery; nack on failure |
| Durable workflow state | Cadence event history ensures no step is silently lost |
| Idempotent delivery | Redis dedup key per `idempotency_key` (24h window) |
| Effective result: exactly-once | Pub/Sub at-least-once + Cadence durability + Redis dedup |

---

## 16. Scalability Strategies

### 16.1 Horizontal Scaling

Each component scales independently:

```
Component               | Scale Trigger                      | Scale Unit
------------------------|------------------------------------|-------------------
Notification Service    | CPU > 70%                          | +2 instances
Cadence Workers         | Cadence task queue backlog > 1K    | +worker pods
Pub/Sub                 | Serverless — auto-scales           | N/A
Email Workers           | Pub/Sub sub undelivered msg > 5K   | +worker pods
SMS Workers             | Pub/Sub sub undelivered msg > 2K   | +worker pods
Push Workers            | Pub/Sub sub undelivered msg > 10K  | +worker pods
OTP Workers             | p99 latency > 500ms                | +worker pods (pre-scale)
Redis                   | Memory > 75%                       | +shards
PostgreSQL              | CPU > 60%                          | +read replicas
```

### 16.2 Database Sharding

```
notification_logs → Time-partitioned (monthly) + archived to S3 after 90 days

user_preferences  → Sharded by userId (DynamoDB auto-shards)

notifications     → Sharded by userId % 16 shards
```

### 16.3 Caching Strategy

```
User Preferences:  Redis, TTL=5min  (read-heavy, low update rate)
Templates:         Redis, TTL=1hr   (static, only invalidate on update)
Device Tokens:     Redis, TTL=24hr  (per-user token list)
OTP values:        Redis, TTL=300s  (auth-critical, no DB persistence)
Rate limit counts: Redis, TTL=86400 (per-day counters)
```

---

## 17. Security

### 17.1 API Authentication

- **Service-to-service**: mTLS certificates + short-lived JWT (1-hour expiry)
- **External clients**: API keys with HMAC-SHA256 request signing
- **OAuth 2.0**: Client credentials flow for third-party integrations

### 17.2 OTP Security

```
✓ Cryptographically random (SecureRandom, not Math.random)
✓ Time-limited (configurable TTL, default 5 minutes)
✓ Single-use (deleted on successful verification)
✓ Attempt-limited (max 5 attempts, then lockout)
✓ Rate-limited generation (max 5 OTPs/phone/hour)
✓ Never logged in plaintext (only hash stored in audit log)
✗ Never returned to caller API (only sent directly to user)
```

### 17.3 Data Protection

```
PII in transit:  TLS 1.3 everywhere
PII at rest:     AES-256 encryption for phone numbers and emails in DB
Credential mgmt: Provider API keys in AWS Secrets Manager / HashiCorp Vault
Audit logs:      Immutable (append-only), retained 7 years (regulatory)
```

### 17.4 GDPR / Regulatory

- User can request deletion → soft-delete preferences, anonymize logs
- Unsubscribe tokens in every marketing email (CAN-SPAM compliance)
- SMS opt-out via STOP keyword handling via webhook
- DPA agreements with all third-party providers (Twilio, Amazon SES, Mailgun, Plivo, etc.)

---

## 18. Monitoring & Observability

### 18.1 Key Metrics

```
Delivery Metrics:
  notification.sent.count         [channel, type, status]
  notification.delivery.latency   [channel] — p50, p99
  notification.failure.rate       [channel, provider]
  notification.dlq.depth          [channel]

OTP Metrics:
  otp.generated.count             [purpose]
  otp.verified.count              [purpose, result]
  otp.expired.count               [purpose]
  otp.attempts.exceeded.count     []

Provider Metrics:
  provider.api.latency                    [vendor, channel]
  provider.api.error.rate                 [vendor, channel]
  provider.circuit_breaker.state          [vendor]        -- 0=CLOSED, 1=OPEN, 2=HALF_OPEN
  provider.circuit_breaker.failure_rate   [vendor]        -- rolling failure %
  provider.circuit_breaker.calls_not_permitted [vendor]   -- calls rejected while OPEN
  provider.fallback.triggered             [channel, from_vendor, to_vendor]

Pub/Sub Metrics:
  pubsub.subscription.num_undelivered_messages  [subscription]
  pubsub.subscription.oldest_unacked_message_age [subscription]
  pubsub.topic.send_message_operation_count      [topic]

Cadence Metrics:
  cadence.workflow.completed      [workflow_type, status]
  cadence.workflow.failed         [workflow_type]
  cadence.activity.failed         [activity_type, workflow_type]
  cadence.task_queue.backlog      [task_list]
```

### 18.2 Dashboards (Grafana)

```
1. Notification Overview:   Total sent, failure rate, DLQ depth
2. Channel Health:          Per-channel success/failure timeseries
3. OTP Dashboard:           Generation rate, verify success rate, fraud attempts
4. Provider Status:         Latency heatmaps per vendor, error rates per vendor
5. Circuit Breaker Status:  Per-vendor CB state (CLOSED/OPEN/HALF_OPEN) + failure %
                             + fallback trigger rate per channel
6. Pub/Sub Health:          Undelivered message count + oldest unacked age per subscription
7. Cadence Health:          Workflow completion rate, failed workflows, task queue backlog
```

### 18.3 Alerts

| Alert | Threshold | Severity |
|-------|-----------|----------|
| DLQ (notifications-dlq) message count | > 100 messages | P2 |
| OTP delivery failure rate | > 5% | P1 |
| **Amazon SES CB state = OPEN** | Any | P2 |
| **Mailgun CB state = OPEN** | Any | P2 |
| **SMTP Relay CB state = OPEN** (all email CBs open) | All 3 simultaneously | P1 |
| **Twilio (SMS) CB state = OPEN** | Any | P2 |
| **Plivo (SMS) CB state = OPEN** | Any | P2 |
| **Vonage (SMS) CB state = OPEN** (all SMS CBs open) | Twilio + Plivo + Vonage simultaneously | P1 |
| **Twilio (OTP) CB state = OPEN** | Any | P1 |
| **Plivo (OTP) CB state = OPEN** (both OTP CBs open) | Both simultaneously | P1 — page on-call |
| **FCM CB state = OPEN** | Any | P2 |
| **APNs CB state = OPEN** | Any | P2 |
| **Pushwoosh CB state = OPEN** | Any | P2 |
| **WebSocket gateway CB state = OPEN** | Any | P2 |
| **Webhook delivery CB state = OPEN** | Any | P2 |
| Fallback triggered rate (any channel) | > 5% of messages | P2 |
| Pub/Sub undelivered msgs (OTP subscription) | > 500 | P1 |
| Cadence task queue backlog (notification-high) | > 200 | P1 |
| Cadence workflow failure rate | > 1% | P2 |
| Overall failure rate | > 2% | P2 |
| p99 OTP delivery latency | > 3 seconds | P1 |

### 18.4 Logging

```java
// Structured log on every delivery attempt
{
  "event": "notification.delivered",
  "notificationId": "notif-789",
  "userId": "u-123",
  "channel": "sms",
  "provider": "twilio",
  "latencyMs": 823,
  "attempt": 1,
  "providerMessageId": "SM1234567890",
  "timestamp": "2026-04-17T10:00:05.123Z"
}
```

---

## 19. Technology Stack Reference

| Layer                   | Technology                                   | Why                                                                             |
| ----------------------- | -------------------------------------------- | ------------------------------------------------------------------------------- |
| API Gateway             | GCP API Gateway / Kong                       | Auth, rate limiting, routing                                                    |
| App Framework           | Spring Boot / Go (Cadence workers)           | REST services + workflow workers                                                |
| Message Queue           | **Google Cloud Pub/Sub**                     | Serverless, zero-ops, at-least-once, global                                     |
| Workflow Engine         | **Cadence (Uber open-source)**               | Durable execution, retries, fan-out, native delayed start (`DelayStartSeconds`) |
| Cadence Backend         | Cassandra (persistence) + Kafka (visibility) | Cadence server's internal storage                                               |
| Cache                   | Redis Cluster (GCP Memorystore)              | Sub-ms reads, TTL support                                                       |
| Primary DB              | PostgreSQL (Cloud SQL)                       | Structured notification logs                                                    |
| User Prefs DB           | Firestore / DynamoDB                         | Fast key-value, auto-scaling                                                    |
| Email Provider          | Amazon SES / Mailgun / SMTP relay            | Deliverability, analytics, fallback flexibility                                 |
| SMS Provider            | Twilio / Plivo / Vonage                      | Global coverage, delivery receipts                                              |
| Push (Android)          | Firebase FCM                                 | Google official, free                                                           |
| Push (iOS)              | Apple APNs                                   | Required for iOS                                                                |
| Push Orchestration      | Pushwoosh                                    | Unified multi-platform push control                                             |
| Web Notifications       | WebSocket Gateway (Socket.IO / native WS)    | Real-time in-app/browser delivery                                               |
| Webhook Notifications   | Outbound Webhook Dispatcher                  | Server-to-server event callbacks                                                |
| Circuit Breaker         | Resilience4j                                 | Per-provider fault isolation                                                    |
| Secret Management       | GCP Secret Manager                           | Rotate provider credentials                                                     |
| Observability           | Prometheus + Grafana                         | Metrics dashboards                                                              |
| Distributed Tracing     | Google Cloud Trace / Jaeger                  | End-to-end request tracing                                                      |
| Log Aggregation         | Google Cloud Logging / ELK                   | Centralized structured logs                                                     |
| Container Orchestration | Kubernetes (GKE)                             | Auto-scaling, rolling deploys                                                   |
| CI/CD                   | GitHub Actions + ArgoCD                      | GitOps deployment pipeline                                                      |

---

## Appendix: Design Decision Log

| Decision                 | Options Considered                                          | Chosen                                                                   | Reason                                                                                           |
| ------------------------ | ----------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| Message broker           | Kafka vs Pub/Sub vs SQS                                     | **Google Cloud Pub/Sub**                                                 | Zero-ops, globally replicated, sufficient when Cadence owns replay/retry                         |
| Workflow orchestration   | Custom retry + cron + saga vs Cadence                       | **Cadence**                                                              | Durable execution, built-in timers, fan-out, full event history — replaces 3 separate systems    |
| Retry ownership          | Worker-level retry loops vs Cadence Activity policy         | **Cadence Activity retry**                                               | Single declaration, uniform behavior, visible in Cadence UI                                      |
| Scheduled delivery       | `scheduled_notifications` table + cron job vs Cadence timer | **Cadence `workflow.sleep`**                                             | Durable, no polling, survives restarts                                                           |
| Bulk fan-out             | Batch job service vs Cadence child workflows                | **Cadence child workflows**                                              | Progress tracked per-user, resumable after crash                                                 |
| User preferences storage | PostgreSQL vs Firestore/DynamoDB                            | **Firestore** (GCP stack)                                                | Key-value pattern, auto-scale, < 1ms reads, GCP-native                                           |
| OTP storage              | DB vs Redis                                                 | **Redis**                                                                | TTL-native, sub-ms, no cleanup job needed                                                        |
| Delivery guarantee       | At-most-once vs at-least-once                               | **At-least-once + idempotency**                                          | Zero message loss; dedup via Redis key                                                           |
| Priority handling        | Separate Pub/Sub topics vs Cadence task lists               | **Both** — separate topics per channel + Cadence task lists per priority | Topics isolate channel throughput; task lists isolate OTP worker pool                            |
| Push fan-out             | Sequential vs parallel                                      | **Parallel (CompletableFuture)**                                         | Multi-device users need concurrent sends                                                         |
| Circuit breaker scope    | One breaker per channel vs per vendor                       | **Per vendor**                                                           | SES failing ≠ Mailgun failing — fine-grained CB prevents healthy vendors from being blocked       |
| Push CB fallback         | Cross-platform fallback vs graceful drop                    | **Graceful drop**                                                        | No viable cross-platform push fallback; dropping is safer than forcing SMS for every failed push |
| OTP CB threshold         | Same as SMS (50%) vs tighter (30%)                          | **30% failure rate, 10s window**                                         | OTP blocks user login — fail fast and alert rather than silently degrading                       |

---

*Sources & Further Reading:*
- [Design a Scalable Notification Service — AlgoMaster](https://blog.algomaster.io/p/design-a-scalable-notification-service)
- [Scalable Notification System HLD to LLD — Tanushree / Medium](https://medium.com/@tanushree2102/designing-a-scalable-notification-system-from-hld-to-lld-e2ed4b3fb348)
- [How to Design a Notification System — System Design Handbook](https://www.systemdesignhandbook.com/guides/design-a-notification-system/)
- [Notification Service System Design — CodeKarle](https://www.codekarle.com/system-design/Notification-system-design.html)
- [Notification System Design — AlgoMaster Blog](https://blog.algomaster.io/p/design-a-scalable-notification-service)
