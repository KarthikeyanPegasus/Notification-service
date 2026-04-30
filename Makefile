.PHONY: help
.PHONY: infra-up infra-down infra-kafka infra-up-all
.PHONY: api worker ui db-shell stop clean check-deps
.PHONY: db-version db-migrate-up db-force db-fix db-ensure
.PHONY: start start-local start-kafka start-kafka-local

# ── Help ─────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "NotifyHub — local development commands"
	@echo "======================================="
	@echo ""
	@echo "  make start           Start everything in mock-pubsub mode (default, no Kafka)"
	@echo "  make start-kafka     Start everything in Kafka mode (includes Redpanda)"
	@echo ""
	@echo "Infrastructure tiers:"
	@echo "  make infra-up        Base infra: Postgres, Redis, Mailhog, Temporal"
	@echo "  make infra-kafka     Add Redpanda (Kafka-compatible) + Redpanda Console"
	@echo "  make infra-up-all    Base infra + Redpanda"
	@echo "  make infra-down      Stop and remove all Docker Compose services"
	@echo ""
	@echo "Individual services:"
	@echo "  make api             Run the API server (Go)"
	@echo "  make worker          Run the background Worker (Go)"
	@echo "  make ui              Run the Next.js UI in dev mode"
	@echo ""
	@echo "Database:"
	@echo "  make db-migrate-up   Apply pending migrations"
	@echo "  make db-version      Print current migration version"
	@echo "  make db-force V=N    Force migration to version N (fixes dirty state)"
	@echo "  make db-fix          Fix dirty migration (force to N-1, then up)"
	@echo "  make db-ensure       Auto-fix dirty DB if detected"
	@echo "  make db-shell        Open a psql shell"
	@echo ""
	@echo "Utilities:"
	@echo "  make stop            Kill any local API / Worker / UI processes"
	@echo "  make clean           Remove Go build cache and UI build output"
	@echo ""

# ── Environment ───────────────────────────────────────────────────────────────
# Override any of these in your shell before running make.

export NS_DATABASE_DSN       ?= postgres://notif:notif@localhost:5432/notifdb?sslmode=disable
export NS_REDIS_ADDR         ?= localhost:6379
export NS_PUBSUB_MODE        ?= mock
# Kafka brokers — only used when NS_PUBSUB_MODE=kafka
export NS_PUBSUB_KAFKA_BROKERS ?= localhost:9092
export NS_WORKER_INTERNAL_URL ?= http://localhost:8081
export NS_LOG_FORMAT         ?= console
export NS_LOG_LEVEL          ?= debug
export NS_SERVER_MODE        ?= debug
export NS_PROVIDERS_EMAIL_SMTP_HOST ?= localhost
export NS_PROVIDERS_EMAIL_SMTP_PORT ?= 1025
export NS_JWT_SECRET         ?= change-me-in-production
export NEXT_PUBLIC_API_URL   ?= http://localhost:8080

# ── Migration helper ──────────────────────────────────────────────────────────
MIGRATE = go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# ── Infrastructure tiers ──────────────────────────────────────────────────────

# Base: everything needed for mock-pubsub local dev.
infra-up:
	docker compose up -d postgres redis mailhog temporal temporal-ui

# Kafka: Redpanda (Kafka-compatible broker) + web console.
# Use when NS_PUBSUB_MODE=kafka.
infra-kafka:
	docker compose up -d redpanda redpanda-console
	@echo "Redpanda ready on localhost:9092  |  Console at http://localhost:8084"

# All services.
infra-up-all: infra-up infra-kafka

# Tear everything down.
infra-down:
	docker compose down

# ── Start targets ─────────────────────────────────────────────────────────────

# Default: mock pubsub, no Kafka required.
start: stop infra-up db-ensure db-migrate-up check-deps start-local

start-local:
	@npx concurrently -k -p "[{name}]" -n "API,WORKER,UI" -c "yellow.bold,cyan.bold,green.bold" \
		"make api" \
		"make worker" \
		"make ui"

# Kafka mode: full stack including Redpanda and observability.
# Switches pubsub mode to kafka for both API and Worker.
start-kafka: stop infra-up-all db-ensure db-migrate-up check-deps
	@echo "Starting in Kafka mode (brokers: $(NS_PUBSUB_KAFKA_BROKERS))"
	@NS_PUBSUB_MODE=kafka npx concurrently -k -p "[{name}]" -n "API,WORKER,UI" -c "yellow.bold,cyan.bold,green.bold" \
		"NS_PUBSUB_MODE=kafka make api" \
		"NS_PUBSUB_MODE=kafka make worker" \
		"make ui"

# ── Individual services ───────────────────────────────────────────────────────

api: db-ensure db-migrate-up
	cd api && exec go run cmd/api/main.go

worker: db-ensure db-migrate-up
	cd api && exec go run cmd/worker/main.go

ui: check-deps
	cd ui && exec npm run dev

# ── Utilities ─────────────────────────────────────────────────────────────────

# Kill any locally running Go or UI dev-server processes.
stop:
	@echo "Stopping local processes on :8080 :8081 :3000 ..."
	@lsof -ti :8080 | xargs kill -9 2>/dev/null || true
	@lsof -ti :8081 | xargs kill -9 2>/dev/null || true
	@lsof -ti :3000 | xargs kill -9 2>/dev/null || true

check-deps:
	@if [ ! -d "ui/node_modules" ]; then \
		echo "Installing UI dependencies..."; \
		cd ui && npm install; \
	fi

db-shell:
	docker exec -it notification-service-postgres-1 psql -U notif -d notifdb

db-version:
	@cd api && $(MIGRATE) -path migrations -database "$(NS_DATABASE_DSN)" version

db-migrate-up:
	@cd api && $(MIGRATE) -path migrations -database "$(NS_DATABASE_DSN)" up

# Usage: make db-force V=10
db-force:
	@if [ -z "$(V)" ]; then echo "Usage: make db-force V=<version>"; exit 1; fi
	@cd api && $(MIGRATE) -path migrations -database "$(NS_DATABASE_DSN)" force $(V)

db-fix:
	@echo "Fixing dirty migration — forcing to previous version, then migrating up..."
	@cd api && \
		dirty_ver=$$($(MIGRATE) -path migrations -database "$(NS_DATABASE_DSN)" version 2>&1 | grep -oE '[0-9]+' | head -1); \
		prev=$$((dirty_ver - 1)); \
		echo "Forcing to version $$prev..."; \
		$(MIGRATE) -path migrations -database "$(NS_DATABASE_DSN)" force $$prev
	@$(MAKE) db-migrate-up

db-ensure:
	@set -e; \
	out="$$(cd api && $(MIGRATE) -path migrations -database "$(NS_DATABASE_DSN)" version 2>/dev/null || true)"; \
	echo "$$out" | grep -q "(dirty)" && { \
		echo "DB is dirty ($$out). Running db-fix..."; \
		$(MAKE) db-fix; \
	} || { \
		echo "DB migration version OK ($$out)"; \
	}

clean:
	cd api && go clean -cache -modcache
	rm -rf ui/.next ui/node_modules
