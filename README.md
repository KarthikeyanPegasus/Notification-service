# Notification Service

A high-performance, scalable, and resilient notification engine designed for production-level workloads. It supports unified multi-channel delivery, event-driven triggers via Pub/Sub, Temporal-driven reliability, and an interactive native documentation suite.

## ✨ Key Features

- **Multi-Channel Support**: Unified API for SMS (Twilio, Vonage, Plivo), Email (SES, Mailgun, SMTP), Push (FCM, APNs, Pushwoosh), Webhooks, WebSockets, and **Slack** (Incoming Webhook JSON via dedicated `slack` channel and `notifications-slack` worker topic).
- **SMS templates**: Template bodies are capped at **160 characters** (GSM-style segment) in API and dashboard.
- **Event-Driven Entry Point**: Trigger notifications asynchronously by publishing JSON events to a Pub/Sub topic, bypassing the need for synchronous REST calls.
- **Template Management**: Create and manage reusable message templates for Email, SMS, and Push with channel-specific options and dynamic variable substitution.
- **Interactive Native Documentation**: A custom-built, high-fidelity API documentation viewer integrated directly into the dashboard.
- **Resilient Workflows**: Message lifecycle is managed by **Temporal**, providing durable execution with automatic retries and exponential backoff.
- **Vendor Status Sync**: Real-time polling of external provider APIs to synchronize delivery status and track delivery latency.
- **Circuit Breakers**: Prevents cascading failures when vendors are unhealthy using the Sony Gobreaker pattern.
- **Dynamic Configuration**: Hot-reloading of provider credentials and settings via a modern, form-based "App Store" UI.

## 🚀 Quick Start (Docker)

The easiest way to get the entire stack (API, Worker, UI, and Observability) running is using Docker Compose.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
- (Optional) Service account key for GCP if running in production mode.

### Running the Stack

1. **Clone the repository.**
2. **Start the services:**
   ```bash
   make start
   ```

### Accessing the components

- **Frontend Dashboard**: [http://localhost:3000](http://localhost:3000)
- **API Documentation**: [http://localhost:3000/docs](http://localhost:3000/docs)
- **API Backend**: [http://localhost:8080](http://localhost:8080)
- **Temporal UI**: [http://localhost:8082](http://localhost:8082)
- **MailHog (Test Email)**: [http://localhost:8025](http://localhost:8025)

---

## 🏗️ Architecture

The system is built with a decoupled, asynchronous architecture powered by Temporal and Go.

### 1. Ingress Gateways
- **REST API**: Standard HTTP/JSON entry point with JWT and Service Token authentication.
- **Pub/Sub Ingress**: Event-driven entry point that subscribes to an `ingress` topic and triggers the same service logic.

### 2. Workflow Orchestration (`Temporal`)
- Manages the lifecycle of a notification: **Preference check → Template Rendering → Vendor Dispatch → Status Tracking**.

### 3. Native Dashboard (`/ui`)
- **Next.js (App Router)** with a macOS Sequoia-inspired design system.
- **Live Metrics**: Real-time throughput, success rates, and delivery latency monitoring.
- **Explorer**: Detailed view of delivery attempts, event timelines, and manual status syncing.
- **API Docs**: Searchable documentation loaded from the API OpenAPI spec (`NEXT_PUBLIC_API_URL` in production).
- **Settings**: FCM (service account upload, status) and a **Social** tab for vendor credentials (Slack, Discord, Teams, Telegram) without routing preferences.
- **Sidebar**: Collapsible rail; logo toggles layout; hover expands temporarily when collapsed.

---

## 📡 Event-Driven Notifications

You can trigger notifications by publishing a JSON payload to the `notifications-ingress` topic.

**Example Payload:**
```json
{
  "user_id": "550e8400-e29b-4142-a273-041772000000",
  "channels": ["email", "sms"],
  "type": "transactional",
  "recipient": "user@example.com",
  "idempotency_key": "unique-event-id-123"
}
```

Support for GCP Pub/Sub, Redis Pub/Sub, and Mock drivers is included.

Channel-specific worker topics include `notifications-slack` for Slack delivery (see `api/internal/pubsub/client.go`).

---

## 📚 API specification & Go SDK

- **OpenAPI**: `api/docs/openapi.yaml` (also served at `/v1/openapi.yaml` and `/v1/openapi.json` on the API).
- **Go client**: `sdk/go` — see [`sdk/README.md`](sdk/README.md) for install and examples (`NotifyBySlack`, `NotifyBySMS`, etc., or `Send` for multi-channel).

---

## 🛠️ Configuration

The system uses a combination of static YAML configs and dynamic database-backed settings.

- **Static Configuration**: Located in `api/config/config.yaml`.
- **Dynamic Configuration**: Managed via the UI **App Store** and stored in the `vendor_configs` table. These settings are hot-reloaded across the fleet using internal Pub/Sub signals.

---

## 📈 Monitoring & Observability

- **Prometheus Metrics**: Exposed at `:8080/metrics` (API) and `:8081/metrics` (Worker).
- **Grafana**: Pre-configured dashboards for success rates, latency (p50/p95), and circuit breaker states.
- **Tracing**: Integrated with Temporal for deep visibility into notification lifecycles.
