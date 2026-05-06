# Notification Service (NotifyHub)

## 1. What This Project Does

This is a multi-channel notification delivery system — it sends emails, SMS, push notifications, webhooks, Slack messages, and WebSocket events through a unified API. By default it uses **in-process Go routines** for retry/backoff, with an optional **Temporal** mode for durable orchestration. **Kafka/Redpanda** handles priority-queue message routing between components.

Who this is for: backend teams that need a self-hosted notification engine with support for multiple providers (e.g., SES + Mailgun for email fallback, Twilio + Vonage for SMS fallback), template rendering, per-client API keys, rate limiting, and circuit breakers.

---

## 2. Why This Exists

The project centralizes notification logic that would otherwise live in every microservice. It handles:
- Provider failover (if Twilio is down, try Plivo)
- Idempotency (retrying won't send duplicate messages)
- Templates + variable substitution
- Per-client vendor configs (each API key can have its own Twilio account)
- Delivery tracking + reconciliation

**When to use this:** You need a self-hosted notification service with vendor redundancy and you're willing to run Postgres, Redis, and optionally Kafka/Redpanda + Temporal.

**When NOT to use this:** You want a SaaS solution (use Twilio SendGrid, Courier, or Knock). You want something you can set up in 5 minutes. You don't need multi-provider failover.

---

## 3. Key Features

- **Multi-channel**: Email (SES, Mailgun, SMTP, SendGrid), SMS (Twilio, Plivo, Vonage), Push (FCM, APNs, Pushwoosh), Webhooks, Slack, WebSocket
- **Provider failover**: Configure primary + backup providers per channel
- **Priority queues**: Per-channel, per-priority topics (high/medium/low) via Kafka/Redpanda
- **Workflow engine**: Standalone (Go routines) or Temporal (durable with retries, exponential backoff)
- **Template engine**: Channel-specific templates with `{{variable}}` substitution
- **Circuit breakers**: Sony Gobreaker per vendor — stops hammering unhealthy providers
- **Rate limiting**: Per-vendor rate limits (Redis-backed)
- **API key auth**: Scoped vendor configs per API key
- **Clerk auth**: User management + JWT verification via Clerk (optional — mock mode available for local dev)
- **Admin UI**: Next.js dashboard with metrics, notification explorer, template editor
- **Idempotency**: Dedup by idempotency key
- **Pub/Sub ingress**: Trigger notifications by publishing to a GCP/Redis/Kafka topic
- **Spam filtering**: SpamAssassin integration (enabled by default, gracefully skips when unreachable)

---

## 4. Tech Stack

| Layer | Tech | Notes |
|-------|------|-------|
| API | Go 1.24, Gin, Viper | |
| Worker | Go 1.24, Kafka consumers | Priority queue consumers per channel |
| UI | Next.js 15 (App Router), React 19, Tailwind, Clerk | |
| Database | PostgreSQL 17 | Via `golang-migrate/migrate` |
| Cache | Redis 7 | Rate limits, templates, session cache |
| Message bus | Redpanda (Kafka-compatible) or GCP Pub/Sub or Redis Pub/Sub | Default: mock |
| Workflows | Temporal (optional) or standalone mode | Default: `standalone` (Go routines); switch to `temporal` for Temporal Server |
| Metrics | prometheus/client_golang | `/metrics` endpoint on API (`:8080`) and Worker (`:8081`) |
| Auth | Clerk.com | Optional — set `NEXT_PUBLIC_CLERK_ENABLED=false` for mock auth |
| Spam filter | SpamAssassin | Included in docker-compose, defaults to enabled |

---

## 5. Project Structure

```
├── api/                          # Go backend
│   ├── cmd/api/main.go           # API server entry point
│   ├── cmd/worker/main.go        # Background worker entry point
│   ├── internal/
│   │   ├── config/               # Viper-based config loading
│   │   ├── handler/              # Gin HTTP handlers + router
│   │   ├── service/              # Business logic layer
│   │   ├── repository/           # PostgreSQL data access
│   │   ├── pubsub/               # Kafka / Redis / GCP / Mock publishers
│   │   ├── provider/             # Vendor integrations (SES, Twilio, FCM, etc.)
│   │   ├── workflow/             # Temporal / Cadence / Standalone engine
│   │   ├── worker/               # Kafka consumer workers
│   │   ├── circuit/              # Circuit breaker registry
│   │   ├── security/             # Encryption, content filtering, spam detection
│   │   ├── cache/                # Redis client wrapper
│   │   └── middleware/           # Gin middleware
│   ├── migrations/               # SQL migrations (consolidated into 001_initial_schema)
│   ├── config/config.yaml        # ⚠️ Gitignored — copy from config.yaml.example
│   ├── config/config.yaml.example# Annotated example config (standalone defaults)
│   ├── docs/openapi.yaml         # OpenAPI 3.0 spec
│   └── go.mod / go.sum
├── ui/                           # Next.js frontend
│   ├── src/app/                  # App Router pages
│   ├── src/components/           # React components
│   ├── .env.example              # Env template (documents Clerk bypass)
│   └── next.config.js            # Proxies /v1/* to API
├── helm/                         # Helm chart for Kubernetes deployment
├── sdk/go/                       # Go SDK client library
├── sdk/dotnet/                   # .NET SDK client library
├── docs/                         # Architecture docs, sequence diagrams
├── docker-compose.yml            # Postgres, Redis, Redpanda, SpamAssassin, API, Worker, UI
├── Dockerfile                    # Multi-stage Go build
├── Dockerfile.ui                 # Next.js standalone build
├── Makefile                      # All local dev commands
```

---

## 6. How to Run Locally (CRITICAL)

### Prerequisites

- **Go 1.24+**
- **Node.js 20+**
- **Docker + Docker Compose**
- **`lsof` or `fuser`** (both used by `make stop`)
- **`npx`** (comes with Node.js — `make start` uses `npx concurrently`)

You do **not** need a Clerk account for local dev — set `NEXT_PUBLIC_CLERK_ENABLED=false` to use mock auth (see Step 3).

### Step 1: Clone and create config file

⚠️ **`api/config/config.yaml` is gitignored.** A fresh clone will not have it.

```bash
# Copy the annotated example config (documents every field with standalone defaults)
cp api/config/config.yaml.example api/config/config.yaml
```

**Pick the config that matches your workflow setup:**

<details>
<summary><b>Standalone mode ✅ (default, no external dependencies)</b></summary>

Workers use Go routines instead of Temporal. Best for local dev and simple deployments.

```yaml
server:
  mode: debug

database:
  dsn: "postgres://notif:notif@localhost:5432/notifdb?sslmode=disable"
  migration_dir: "migrations"

redis:
  addr: "localhost:6379"

pubsub:
  mode: mock

cadence:
  mode: standalone       # <-- workers use in-process goroutines
  host_port: ""
  domain: ""

jwt:
  secret: "<SET_IN_ENV>"   # set NS_JWT_SECRET for auth to work
  service_secret: "<SET_IN_ENV>"

log:
  level: debug
  format: console

security:
  vendor_config_encryption_key: "<SET_IN_ENV>"
  spam_assassin:
    enabled: true           # gracefully degrades when unreachable
  rate_limit:
    enabled: true
    rps: 100
```
</details>

<details>
<summary><b>Temporal mode (advanced, requires a running Temporal Server)</b></summary>

Enables durable workflow orchestration with retries, backoff, and scheduled delivery.

```yaml
cadence:
  mode: temporal            # <-- requires Temporal Server at host_port
  host_port: "localhost:7233"
  domain: "default"
```

Start a Temporal Server:
```bash
docker run -d --name temporal -p 7233:7233 temporalio/auto-setup:latest
```
</details>

The full annotated config is at `api/config/config.yaml.example` — every field is documented inline.

### Step 2: Start infrastructure

```bash
# Start Postgres + Redis (enough for mock mode)
make infra-up

# Or start everything (including Redpanda/Kafka + SpamAssassin)
make infra-up-all
```

### Step 3: Set environment variables

**UI** — Choose your auth mode:

```bash
cp ui/.env.example ui/.env.local
# By default: NEXT_PUBLIC_CLERK_ENABLED=false (mock auth, no Clerk account needed)
# For real Clerk auth: set to true and fill in the key vars
```

**Go services** — the Makefile provides sensible defaults for local dev:

| Env Var | Makefile Default | Why |
|---------|-----------------|-----|
| `NS_DATABASE_DSN` | `postgres://notif:notif@localhost:5432/notifdb?sslmode=disable` | Postgres connection |
| `NS_REDIS_ADDR` | `localhost:6379` | Redis |
| `NS_PUBSUB_MODE` | `mock` | No Kafka needed |
| `NS_CADENCE_MODE` | `standalone` | No Temporal needed |
| `NS_LOG_LEVEL` | `debug` | Verbose logs |
| `NS_SERVER_MODE` | `debug` | Dev mode |

Required if you want JWT auth to work:
```bash
export NS_JWT_SECRET="some-strong-secret-at-least-32-chars-long"
```

### Step 4: Run the services

```bash
# Option A: Quick start (mock Pub/Sub, mock auth, everything local)
make start-mock

# Option B: Full stack with Kafka
make start

# Option C: Run individually (separate terminals)
make api      # Terminal 1
make worker   # Terminal 2
make ui       # Terminal 3
```

### Step 5: Verify

```bash
curl http://localhost:8080/health
# → {"status":"ok"}
```

### What you'll get:

| Service | URL | Notes |
|---------|-----|-------|
| API | http://localhost:8080 | Health at `/health` |
| Worker metrics | http://localhost:8081 | Prometheus `/metrics`, health at `/health` |
| UI | http://localhost:3000 | Mock auth by default (no login screen) |
| Redpanda Console | http://localhost:8084 | If you started infra-kafka |

---

## 7. Configuration

### Environment Variables (Go services)

The config system uses Viper with `NS_` prefix. Anything in `config.yaml` can be overridden via `NS_` + uppercase + underscores.

| Env Var | Default | Required | Description |
|---------|---------|----------|-------------|
| `NS_DATABASE_DSN` | `postgres://notif:notif@localhost:5432/notifdb?sslmode=disable` | Yes | PostgreSQL connection string |
| `NS_REDIS_ADDR` | `localhost:6379` | Yes | Redis address |
| `NS_PUBSUB_MODE` | `mock` | No | `mock`, `kafka`, `redis`, or `gcp` |
| `NS_PUBSUB_KAFKA_BROKERS` | `localhost:9092` | If kafka | Comma-separated brokers |
| `NS_JWT_SECRET` | none | Recommended | At least 32 chars in production |
| `NS_JWT_SERVICE_SECRET` | none | For OTP service-auth | Separate key for inter-service auth |
| `NS_CADENCE_MODE` | `standalone` | No | `standalone`, `temporal`, `cadence` |
| `NS_CLERK_SECRET_KEY` | none | If Clerk enabled | From Clerk dashboard |
| `NS_CLERK_WEBHOOK_SECRET` | none | If Clerk webhooks | From Clerk dashboard |
| `NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY` | ephemeral (non-release) | In production | 32-byte key, base64 encoded |
| `NS_ADMIN_EMAIL` | none | Optional | Bootstraps admin user |
| `NS_ADMIN_PASSWORD` | none | Optional | For admin bootstrap |
| `NS_PROVIDERS_SLACK_DEFAULT_USERNAME` | none | Optional | Fallback Slack display name |
| `NS_PROVIDERS_SLACK_CHANNELS` | none | Optional | JSON array of Slack channel configs |
| `NS_LOG_LEVEL` | `debug` (Makefile) | No | `debug`, `info`, `warn`, `error` |
| `NS_LOG_FORMAT` | `console` (Makefile) | No | `json` or `console` |
| `ENVIRONMENT` | (empty) | Recommended | Guards debug mode — set to `production` or `staging` in prod |
| `NS_SERVER_MODE` | `release` (code) / `debug` (Makefile) | No | `debug` or `release` |

### UI Environment Variables (`ui/.env.local`)

| Env Var | Required | Description |
|---------|----------|-------------|
| `NEXT_PUBLIC_API_URL` | Yes | `http://localhost:8080` |
| `NEXT_PUBLIC_CLERK_ENABLED` | No | `false` = mock auth (default for local dev), `true` = real Clerk |
| `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | If Clerk enabled | From Clerk dashboard (starts with `pk_test_`) |
| `CLERK_SECRET_KEY` | If Clerk enabled | From Clerk dashboard (starts with `sk_test_`) |

---

## 8. API Documentation

Full OpenAPI spec: `api/docs/openapi.yaml` or serve at `http://localhost:8080/v1/openapi.yaml`

### Key Endpoints

```
POST   /v1/notifications            Send a notification (multi-channel)
GET    /v1/notifications             List notifications
GET    /v1/notifications/:id         Get notification details
POST   /v1/notifications/schedule    Schedule a notification

POST   /v1/templates                 Create a template
GET    /v1/templates                 List templates

POST   /v1/api-keys                  Create API key (admin)
GET    /v1/api-keys                  List API keys

POST   /v1/webhooks/stripe           Stripe webhook receiver
POST   /v1/webhooks/sendgrid         SendGrid event webhook

GET    /v1/admin/config              Admin: get current config
POST   /v1/admin/config/vendor       Admin: update vendor config
POST   /v1/admin/migrate/vendors     Migrate vendor configs between modes
```

### Auth

- **API key auth**: Pass `X-API-Key` header — scoped to specific vendor configs
- **JWT auth**: Pass `Authorization: Bearer <jwt>` — requires `NS_JWT_SECRET`
- **Clerk auth**: UI uses Clerk session tokens — JWT verified via Clerk JWKS endpoint (optional — mock auth available)

### Example: Send a notification

```bash
curl -X POST http://localhost:8080/v1/notifications \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "user_id": "550e8400-e29b-4142-a273-041772000000",
    "channels": ["email"],
    "type": "transactional",
    "recipient": "user@example.com",
    "template_id": "welcome-template",
    "variables": {"name": "Alice"},
    "idempotency_key": "req-123"
  }'
```

---

## 9. Usage Examples

### Via the Go SDK

```go
import notification "github.com/spidey/notification-service/sdk/go"

client := notification.New(
    notification.WithBaseURL("http://localhost:8080/v1"),
    notification.WithAPIKey("my-api-key"),
)

client.Notifications.NotifyBySlack(ctx, "user-1", "slack-example",
    "transactional", "https://hooks.slack.com/...",
    &notification.NotifyOptions{Body: "Hello"})
```

### Via Pub/Sub ingress

Publish this JSON to the `notifications-ingress` topic (if you have the ingress subscriber running):

```json
{
  "user_id": "550e8400-e29b-4142-a273-041772000000",
  "channels": ["email", "sms"],
  "type": "transactional",
  "recipient": "user@example.com",
  "idempotency_key": "unique-event-id-123"
}
```

---

## 10. Testing

### Go tests

```bash
cd api

# Run all unit tests
go test ./internal/... -v -count=1

# Run specific package tests
go test ./internal/handler/... -v
go test ./internal/security/... -v

# Run integration tests (requires DB + Redis)
go test ./internal/test/... -v -count=1
```

**Test coverage is limited.** Existing tests:
- `internal/handler/notification_handler_test.go` — validates request payloads
- `internal/security/` — crypto round-trip, content filter checks (including SpamAssassin mock)
- `internal/test/` — lifecycle and channel integration tests
- `internal/provider/all_vendors_test.go` — vendor integration (requires real credentials)

⚠️ **No test for the UI.** Playwright is configured (`playwright.config.ts`) and there's a `tests/` directory, but there are no test files in it.

---

## 11. Known Issues / Limitations

### Onboarding Friction
1. **`api/config/config.yaml` is gitignored** — A fresh clone won't start without it. `api/config/config.yaml.example` is provided — copy it to `config.yaml` and fill in secrets. The real config file is gitignored so secrets are never committed.

### Known Gaps (all currently accepted)
1. **No UI tests** — Playwright config exists but no test files. PRs welcome.
2. **No WebSocket docs** — The WS endpoint (`/v1/ws`) exists in the router but is undocumented.
3. **No health endpoint for Redis/DB** — `/health` checks API liveness only.

---

## 12. Contributing

### Dev Setup

1. Follow "How to Run Locally" above
2. Install Go tools: `brew install golangci-lint` or `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
3. Pre-commit hooks use **gitleaks** to detect secrets — run `pre-commit install` if you have `pre-commit` installed

### Code conventions (inferred from the code)

- **Go**: Standard project layout (`cmd/`, `internal/`, `pkg/`). Gin handlers in `handler/`. Business logic in `service/`. Data access in `repository/`.
- **Naming**: Env vars prefixed with `NS_`. Config keys match env var names (viper auto-binding).
- **Migrations**: Use `golang-migrate/migrate` format. Single consolidated migration (`001_initial_schema`) with `IF NOT EXISTS` guards. Add new migrations as numbered files (`002_description.up.sql` / `002_description.down.sql`).
- **UI**: Next.js App Router with optional Clerk auth. Components in `src/components/`, pages in `src/app/`.

### PR Expectations

- Include tests for new handlers/services
- Add migration files for schema changes (both up and down)
- Update OpenAPI spec if API surface changes
- Update `.env.example` if adding new UI env vars

---

## 13. Deployment

### Docker Compose (for testing)

```bash
docker compose up --build
```

This builds and runs API, Worker, UI, Postgres, Redis, Redpanda, and SpamAssassin.

### Kubernetes (Helm)

Helm chart at `helm/notification-system/`:

```bash
helm install notification-service ./helm/notification-system
```

### Production Checklist

- Set `ENVIRONMENT=production` (fatally rejects debug mode in `cmd/api/main.go`)
- Set `NS_SERVER_MODE=release`
- Set `NS_JWT_SECRET` to a strong 32+ char secret
- Set `NS_JWT_SERVICE_SECRET` for OTP service-auth
- Set `NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY` to a stable key (not ephemeral)
- Set `NS_CLERK_SECRET_KEY` and `NS_CLERK_WEBHOOK_SECRET` if Clerk-enabled
- Set `NS_DATABASE_DSN` with proper credentials (not `notif:notif`)
- Configure `NS_PUBSUB_MODE` to `kafka` or `gcp` (not `mock`)
- Set `NS_SECURITY_HEADERS_ALLOWED_ORIGINS` to your actual domains
- Run a Temporal Server or set `NS_CADENCE_MODE=standalone`

---

## 14. Troubleshooting

| Problem | Likely Cause | Fix |
|---------|-------------|-----|
| API won't start: "loading config" | `api/config/config.yaml` doesn't exist | Copy `config.yaml.example` → `config.yaml` |
| Worker fails to connect to `localhost:7233` | `NS_CADENCE_MODE` set to `temporal` but no Temporal Server running | `export NS_CADENCE_MODE=standalone` or start temporal via docker-compose |
| UI shows Clerk login error | Clerk keys not set in `ui/.env.local` | Set `NEXT_PUBLIC_CLERK_ENABLED=false` in `ui/.env.local` for mock auth |
| DB migration fails / dirty state | Migration error mid-run | `make db-fix` or `make db-force V=<version>` |
| `make stop` fails | `lsof` or `fuser` not installed | Try `fuser -k <port>/tcp` directly; install via package manager |
| API starts but returns 401 on everything | No `NS_JWT_SECRET` set, and JWT middleware rejects | Set `NS_JWT_SECRET` or check if auth is optional for your endpoint |
| Email notifications don't arrive | No SES/Mailgun/SMTP configured | Configure a provider via the App Store UI or set SMTP credentials |
| "vendor_config_encryption_key is empty" warning | `NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY` not set | Fine for local dev (ephemeral key generated). Set it for persistence. |

---

## 15. Roadmap / Improvements

### Future work
- [ ] Add Playwright tests for the UI
- [ ] Add a health endpoint that checks Redis + DB connectivity
- [ ] Document the WebSocket endpoint (`/v1/ws`)

---

## 16. License

Not specified. Check the repository owner for licensing details.
