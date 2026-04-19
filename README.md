# Notification Service

A high-performance, scalable, and resilient notification service designed for production-level workloads. It supports multiple delivery channels (SMS, Email, Push, Webhooks), Temporal-driven reliability, circuit breaking, automatic retries, and manual vendor status synchronization.

## ✨ Key Features

- **Multi-Channel Support**: unified API for SMS (Twilio, Vonage, Plivo), Email (SES, Mailgun, SMTP), Push (FCM), and Webhooks.
- **Resilient Workflows**: Message delivery is managed by **Temporal**, providing durable execution with automatic retries and exponential backoff.
- **Vendor Status Sync**: Real-time polling of external provider APIs (e.g., Twilio) to synchronize delivery status back to the dashboard.
- **Circuit Breakers**: Prevents cascading failures when vendors are unhealthy using the Sony Gobreaker pattern.
- **Dynamic Configuration**: Hot-reloading of provider credentials and settings via a modern, form-based App Store UI.
- **Master Integration Tests**: A unified, table-driven test suite with built-in rate-limiting and credential safety for end-to-end verification.

## 🚀 Quick Start (Docker)

The easiest way to get the entire stack (API, Worker, UI, and Observability) running is using Docker Compose.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
- (Optional) Service account key for GCP Pub/Sub at `api/config/pub-sub-key.json` if running in production mode.

### Running the Stack

1. **Clone the repository.**
2. **Start the services:**
   ```bash
   docker compose up -d --build
   ```

### Accessing the components

- **Frontend UI**: [http://localhost:3000](http://localhost:3000)
- **API Backend**: [http://localhost:8080](http://localhost:8080)
- **Temporal UI**: [http://localhost:8082](http://localhost:8082)
- **Prometheus Metrics**: [http://localhost:9090](http://localhost:9090)
- **Grafana Dashboard**: [http://localhost:3001](http://localhost:3001) (Default: admin/admin)
- **MailHog (Test Email)**: [http://localhost:8025](http://localhost:8025)

---

## 🏗️ Architecture

The system is built with a decoupled, asynchronous architecture powered by Temporal and Go.

### 1. API Service (`/api`)
- **Go (Gin), PostgreSQL, Redis**.
- Handles notification acceptance, idempotency checks, and starts Temporal workflows.
- Exposes administrative APIs for managing vendor configurations.

### 2. Workflow Orchestration (`Temporal`)
- Manages the lifecycle of a notification (Preference check -> Rendering -> Publishing -> Delivery Tracking).
- Handles retries and failure logic durably.

### 3. Dispatcher / Worker (`/api`)
- Responsible for the physical delivery of messages to third-party vendors.
- Implements **Circuit Breakers** and **Dynamic Provider Initialization**.

### 4. Admin Dashboard (`/ui`)
- **Next.js (App Router)**.
- **Live Metrics**: Real-time throughput and success rate monitoring.
- **Notification Explorer**: Detailed view of delivery attempts, event timelines, and manual status syncing.
- **App Store**: Modern, form-based configuration for all supported vendors.

---

## 🛠️ Configuration

The system uses a combination of static YAML configs and dynamic database-backed settings.

- **Static Configuration**: Located in `api/config/config.yaml`.
- **Dynamic Configuration**: Managed via the UI **App Store** and stored in the `vendor_configs` table. These settings override static defaults and are hot-reloaded across the fleet.

---

## 🧪 Development & Testing

### Running Integration Tests
We have a comprehensive integration test suite that hits real vendor APIs.
```bash
# Set required credentials in env or config.yaml
export RUN_INTEGRATION_TESTS=true
cd api && go test ./internal/provider/... -v
```
*Tests include a 2-second throttle between requests to prevent vendor rate-limit violations.*

### Running Locality (Individual Components)

**API Backend:**
```bash
cd api && go run cmd/api/main.go
```

**Worker Service:**
```bash
cd api && go run cmd/worker/main.go
```

**Frontend UI:**
```bash
cd ui && npm install && npm run dev
```

---

## 📈 Monitoring

The system exposes Prometheus metrics at `:8080/metrics` (API) and `:8081/metrics` (Worker). Grafana is used to visualize:
- Success rates per channel and provider.
- Latency (p50, p90, p99) breakdown.
- Circuit breaker trip states.
