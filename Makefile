.PHONY: help
.PHONY: infra-up infra-down infra-kafka infra-up-all
.PHONY: api worker ui db-shell stop clean check-deps
.PHONY: db-version db-migrate-up db-force db-fix db-ensure
.PHONY: start start-mock start-kafka

# ── Local env overrides ───────────────────────────────────────────────────────
# If a .env file exists at the project root, source it for NS_* variables.
# This allows you to persist secrets like NS_CLERK_SECRET_KEY without
# hardcoding them in the Makefile or committing them to git.
ifneq (,$(wildcard .env))
include .env
export
endif

# ── Help ─────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "NotifyHub — local development commands"
	@echo "======================================="
	@echo ""
	@echo "  make start           Start everything in Kafka mode (includes Redpanda)"
	@echo "  make start-mock      Start everything in mock-pubsub mode (no Kafka)"
	@echo ""
	@echo "Infrastructure tiers:"
	@echo "  make infra-up        Base infra: Postgres, Redis"
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
export NS_CLERK_SECRET_KEY   ?=
export NS_CLERK_WEBHOOK_SECRET ?=
# Kafka brokers — only used when NS_PUBSUB_MODE=kafka
export NS_PUBSUB_KAFKA_BROKERS ?= localhost:9092
export NS_WORKER_INTERNAL_URL ?= http://localhost:8081
export NS_LOG_FORMAT         ?= console
export NS_LOG_LEVEL          ?= debug
export NS_SERVER_MODE        ?= debug
export NS_JWT_SECRET         ?=
export NEXT_PUBLIC_API_URL   ?= http://localhost:8080

# ── Shared env vars ──────────────────────────────────────────────────────────
# Used by both `make api` and `make worker`. Add new NS_ vars here, not in
# individual targets, to avoid duplication.
NS_CLERK_EXPORTS = \
	export NS_CLERK_SECRET_KEY="$(NS_CLERK_SECRET_KEY)" && \
	export NS_CLERK_WEBHOOK_SECRET="$(NS_CLERK_WEBHOOK_SECRET)" && \
	export NS_CLERK_JWKS_URL="$(NS_CLERK_JWKS_URL)"
NS_PUBSUB_EXPORTS = \
	export NS_PUBSUB_MODE="$(NS_PUBSUB_MODE)" && \
	export NS_PUBSUB_KAFKA_BROKERS="$(NS_PUBSUB_KAFKA_BROKERS)"

# ── Migration helper ──────────────────────────────────────────────────────────
# Use locally installed migrate binary to avoid Go proxy timeouts.
# Install with: cd api && go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
MIGRATE = $(HOME)/go/bin/migrate

# ── Infrastructure tiers ──────────────────────────────────────────────────────

# Base: everything needed for mock-pubsub local dev.
infra-up:
	docker compose up -d postgres redis

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

# Default: full stack with Redpanda (Kafka-compatible broker) and observability.
# Use `make infra-up` to skip Redpanda if you don't need Kafka.
start: stop infra-up-all db-ensure db-migrate-up check-deps
	@echo "Starting in Kafka mode (brokers: $(NS_PUBSUB_KAFKA_BROKERS))"
	@NS_PUBSUB_MODE=kafka npx concurrently -k -p "[{name}]" -n "API,WORKER,UI" -c "yellow.bold,cyan.bold,green.bold" \
		"NS_PUBSUB_MODE=kafka NS_CADENCE_MODE=standalone make api" \
		"NS_PUBSUB_MODE=kafka NS_CADENCE_MODE=standalone make worker" \
		"make ui"

# Mock pubsub mode (no Kafka required) — for when you only need base infra.
start-kafka: start

start-mock: stop infra-up db-ensure db-migrate-up check-deps
	@npx concurrently -k -p "[{name}]" -n "API,WORKER,UI" -c "yellow.bold,cyan.bold,green.bold" \
		"make api" \
		"make worker" \
		"make ui"

# ── Individual services ───────────────────────────────────────────────────────

api: db-ensure db-migrate-up
	@$(NS_PUBSUB_EXPORTS) && \
	$(NS_CLERK_EXPORTS) && \
	cd api && exec go run cmd/api/main.go

worker: db-ensure db-migrate-up
	@$(NS_PUBSUB_EXPORTS) && \
	$(NS_CLERK_EXPORTS) && \
	cd api && exec go run cmd/worker/main.go

ui: check-deps
	cd ui && exec npm run dev

# ── Utilities ─────────────────────────────────────────────────────────────────

# Kill any locally running Go or UI dev-server processes.
# Uses fuser (Linux) and falls back to lsof (macOS).
KILL_PORT = if command -v fuser >/dev/null 2>&1; then fuser -k $(1)/tcp 2>/dev/null || true; elif command -v lsof >/dev/null 2>&1; then lsof -ti $(1) | xargs kill -9 2>/dev/null || true; else echo "No port-killer found (tried fuser, lsof)"; fi

stop:
	@echo "Stopping local processes on :8080 :8081 :3000 ..."
	@$(call KILL_PORT,:8080)
	@$(call KILL_PORT,:8081)
	@$(call KILL_PORT,:3000)

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
