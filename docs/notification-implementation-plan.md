# Notification System Implementation Plan

## 1) Goal
Build a production-ready notification platform that supports Email, SMS, OTP, Push, WebSocket, and Webhook delivery, with:
- persistent storage in PostgreSQL
- delivery status tracking
- reporting dashboard UI built with shadcn and modern UI components

## 2) Scope and Outcomes
Outcome-focused deliverables:
1. API accepts notification requests and schedules delivery.
2. Workers deliver notifications through configured providers.
3. PostgreSQL stores notifications, attempts, events, and reports data.
4. UI shows real-time and historical status per notification.
5. UI provides reports (success rate, latency, provider performance, failures).
6. Retry, DLQ, and idempotency behavior is observable and testable.

## 3) Recommended Implementation Stack
- Backend API: `Go` (Gin/Fiber) with clean service boundaries.
- Workflow/Orchestration: Cadence (as defined in design doc).
- Queue/Transport: Google Pub/Sub.
- DB: PostgreSQL 17
- Cache: Redis.
- Frontend:  Reactjs + TypeScript + `shadcn/ui`.
- UI data layer: TanStack Query.
- Table/Filtering: TanStack Table.
- Charts: Recharts (or Tremor with shadcn wrapper).
- Auth (UI): existing org SSO/JWT middleware.

## 4) PostgreSQL Data Model (Implementation Target)
Create and migrate these tables first:
1. `notifications`  
Outcome: one row per notification request with channel, status, schedule, metadata.
2. `notification_attempts`  
Outcome: one row per provider attempt with latency, error, provider message id.
3. `notification_events`  
Outcome: immutable timeline (queued, sent, delivered, failed, bounced, clicked, etc.).
4. `scheduled_notifications`  
Outcome: authoritative schedule state + cadence workflow/run identifiers.
5. `provider_webhook_events`  
Outcome: raw provider callbacks retained for audit/debug.
6. `reporting_daily_channel_metrics` (materialized or batch-built table)  
Outcome: fast dashboard/report queries.

Indexes:
- `notifications(user_id, created_at desc)`
- `notifications(status, updated_at)`
- `notification_attempts(notification_id, created_at desc)`
- `notification_events(notification_id, created_at asc)`
- `scheduled_notifications(status, scheduled_at)`

## 5) Work Breakdown Structure (Simple Tasks)

## Phase A: Foundations
- [x] T1. Project scaffolding and environments  
  Outcome: local/dev/prod config, env templates, docker compose for postgres/redis.

- [x] T2. PostgreSQL migrations and schema  
  Outcome: all core tables and indexes created with rollback-safe migrations.

- [x] T3. Shared notification domain models  
  Outcome: strong typed models/enums for channels, statuses, event types.

- [x] T4. Provider abstraction interfaces  
  Outcome: unified sender contracts for email/sms/push/websocket/webhook.

## Phase B: Core Delivery
- [x] T5. `POST /v1/notifications` API (immediate + scheduled)  
  Outcome: accepted requests persisted and workflow start triggered.

- [x] T6. Idempotency handling  
  Outcome: duplicate request protection via idempotency key.

- [x] T7. Cadence workflow implementation  
  Outcome: standalone scheduler polls DB every 30s; Cadence mode available via config.

- [x] T8. Pub/Sub topic/subscription provisioning  
  Outcome: GCP topics for otp/email/sms/push/websocket/webhook + DLQ; mock mode for local dev.

- [x] T9. Channel workers framework  
  Outcome: worker runtime that consumes topic messages and executes sender.

- [x] T10. Provider integrations (initial)  
  Outcome: SES/Mailgun/SMTP, Twilio/Plivo/Vonage, FCM/APNs/Pushwoosh, webhook HTTP sender, websocket gateway sender.

- [x] T11. Retry + circuit breaker policy  
  Outcome: per-provider circuit breakers (sony/gobreaker) with fallback chain and non-retryable mapping.

- [x] T12. Provider webhook ingestion endpoints  
  Outcome: callbacks update attempts/events and store raw payloads.

## Phase C: Tracking and Reporting APIs
- [x] T13. Notification status query API  
  Outcome: `GET /v1/notifications/{id}` returns per-channel/per-attempt status.

- [x] T14. List/search API  
  Outcome: paginated filterable endpoint by user/channel/status/provider/date.

- [x] T15. Report API  
  Outcome: channel/provider success rate, p50/p95 latency, failure reasons, trend over time.

- [x] T16. Reconciliation jobs  
  Outcome: background jobs to backfill eventual delivery state from webhooks/providers.

## Phase D: UI (shadcn + modern components)
- [x] T17. UI app setup with shadcn  
  Outcome: design system primitives (Card, Table, Tabs, Badge, Dialog, Sheet, Date Picker, Chart wrappers).

- [x] T18. Notification status dashboard  
  Outcome: overview KPIs, live status feed, channel health cards.

- [x] T19. Notification explorer page  
  Outcome: searchable/filterable table with status timeline drawer.

- [x] T20. Notification detail page  
  Outcome: full lifecycle timeline, attempts, provider responses, retry history.

- [x] T21. Reporting page  
  Outcome: charts for delivery rate, latency, provider errors, DLQ volume, top failure causes.

- [x] T22. Scheduled notifications management UI  
  Outcome: list scheduled items and allow cancel/reschedule actions.

- [x] T23. UX quality pass  
  Outcome: loading/skeleton states, empty states, error states, responsive layout, accessibility checks.

## Phase E: Quality, Security, and Rollout
- [x] T24. Integration tests (API + DB + queue)  
  Outcome: deterministic tests for enqueue, retries, state transitions.

- [x] T25. End-to-end tests (UI critical paths)  
  Outcome: passing tests for status tracking and reports workflows.

- [x] T26. Observability instrumentation  
  Outcome: metrics, structured logs, trace IDs across API/workers/webhooks.

- [x] T27. Production readiness checklist  
  Outcome: runbook, SLOs, alerts, dashboards, rollback plan.

- [x] T28. Staged rollout  
  Outcome: canary rollout with one channel first, then progressive channel enablement.

## 6) Parallel Execution Plan

Parallel Track P1 (Backend Foundation):
- T1, T2, T3, T4

Parallel Track P2 (Core Delivery):
- T5, T6, T8, T9 can run together after T2/T3/T4
- T7 starts after T5 and T9
- T10 and T11 run in parallel after T9
- T12 runs in parallel with T10

Parallel Track P3 (Tracking APIs):
- T13, T14 start after T2 and partial T5
- T15 starts after T13/T14 data contracts stabilize
- T16 can start once T12 is available

Parallel Track P4 (UI):
- T17 can start immediately
- T18/T19 start after T13/T14 API mocks or contracts
- T20 starts after T13 detail contract is fixed
- T21 starts after T15
- T22 starts after schedule APIs are complete
- T23 runs continuously and final pass before release

Parallel Track P5 (Quality/Release):
- T24 starts after T5/T7/T9
- T25 starts after T18/T19/T20
- T26 starts early and continues incrementally
- T27/T28 start near feature freeze

## 7) Dependency Map (Critical Path)
Critical path:
1. T2 -> T5 -> T7 -> T9 -> T10 -> T13 -> T18 -> T20 -> T25 -> T28

High-impact blockers:
1. DB schema instability blocks API and reporting.
2. Workflow/worker contract drift blocks delivery and status correctness.
3. Provider webhook ingestion delays accurate final states and reports.

## 8) Task Definition of Done (DoD)
Each task is done only when:
1. Functional outcome is demonstrated.
2. Unit/integration tests for new behavior pass.
3. Logs and metrics are emitted for the feature.
4. API contract/docs are updated.
5. No critical lint/type/security issues remain.

## 9) UI Requirements (shadcn + modern components)
Mandatory UI components:
- shadcn: `Card`, `Table`, `Badge`, `Tabs`, `Button`, `Input`, `Select`, `DatePicker`, `Dialog`, `Sheet`, `Tooltip`, `Skeleton`, `Toast`.
- Modern additions: TanStack Table, TanStack Query, Recharts, React Hook Form + Zod.

Mandatory UI views:
1. Dashboard: KPI cards + channel/provider health + failure widgets.
2. Notifications Explorer: advanced filters and saved views.
3. Notification Detail: timeline + attempts + provider payload excerpts.
4. Reports: trend charts + breakdown tables + CSV export.

## 10) Agent CLI Execution Context (for Codex/Claude)
Use this context when delegating:

Project context:
- Source design: `scalable-notification-system.md`
- Goal: implement production-grade notification service + tracking/reporting UI.
- Primary persistence: PostgreSQL (required).
- UI stack: Next.js + TypeScript + shadcn.

Technical constraints:
1. Keep tasks small and outcome-focused.
2. Prefer schema-first API contracts.
3. Preserve idempotency and auditability.
4. Do not couple provider SDK logic into core domain.

Suggested agent prompts:
1. "Implement T2 with SQL migrations for notifications, attempts, events, schedules, and reporting tables, including indexes and rollback scripts."
2. "Implement T13 and T14 APIs with pagination/filtering and PostgreSQL query optimization; include OpenAPI updates."
3. "Implement T17-T20 UI using shadcn + TanStack Table/Query, wired to mock APIs first, then real APIs."
4. "Implement T15 reporting API and T21 reporting UI charts for success rate, latency, provider error trends."
5. "Implement T10 provider adapters and T11 fallback + circuit breaker policy with tests."

Acceptance checkpoints:
1. End-to-end: create notification -> delivered/failed -> status visible in UI.
2. Reporting: last 7/30 day channel metrics load under target latency.
3. Operational: failed provider path visible in logs, metrics, and UI.

## 11) Suggested Milestones
1. M1 (Week 1): T1-T6 complete.
2. M2 (Week 2): T7-T12 complete for one channel end-to-end.
3. M3 (Week 3): T13-T16 complete with accurate tracking.
4. M4 (Week 4): T17-T23 complete with shadcn UI.
5. M5 (Week 5): T24-T28 complete and staged rollout ready.

## 12) Deployment Artifacts Added
Files:
1. `Dockerfile` (parameterized with `APP_NAME=api|worker|ui`)
2. `docker-compose.yml` (postgres, redis, api, worker, ui)
3. `helm/notification-system/Chart.yaml`
4. `helm/notification-system/values.yaml`
5. `helm/notification-system/templates/*` (deployments, services, ingress, secret, configmap, hpa)

How to use:
1. Local compose:
   - `docker compose up --build`
2. Helm deploy:
   - `helm upgrade --install notification-system ./helm/notification-system --namespace notifications --create-namespace`
