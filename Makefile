.PHONY: help infra-up infra-down api worker ui db-shell stop clean check-deps

# Default target
help:
	@echo "Notification Service Frontend & Backend Commands:"
	@echo "------------------------------------------------"
	@echo "  make infra-up      - Start dependencies (Postgres, Redis, Mailhog) via Docker Compose"
	@echo "  make infra-down    - Stop dependencies"
	@echo "  make api           - Run the API server locally"
	@echo "  make worker        - Run the Worker service locally"
	@echo "  make ui            - Run the Next.js UI in development mode"
	@echo "  make db-shell      - Open a psql shell to the local database"
	@echo "  make stop          - Kill any existing API or Worker processes"
	@echo "  make clean         - Remove module caches and build outputs"

# Environment variables for local development
export NS_DATABASE_DSN ?= postgres://notif:notif@localhost:5432/notifdb?sslmode=disable
export NS_REDIS_ADDR ?= localhost:6379
export NS_PUBSUB_MODE ?= mock
export NS_LOG_FORMAT ?= console
export NS_LOG_LEVEL ?= debug
export NS_SERVER_MODE ?= debug
export NS_PROVIDERS_EMAIL_SMTP_HOST ?= localhost
export NS_PROVIDERS_EMAIL_SMTP_PORT ?= 1025
export NS_JWT_SECRET ?= change-me-in-production
export NEXT_PUBLIC_API_URL ?= http://localhost:8080

# Stand up only the infrastructure components needed for local dev
infra-up:
	docker compose up -d postgres redis mailhog temporal temporal-ui

# Tear down infrastructure
infra-down:
	docker compose down

# Start all application services concurrently (Infra + API + Worker + UI)
start: stop infra-up check-deps
	@npx concurrently -k -p "[{name}]" -n "API,WORKER,UI" -c "yellow.bold,cyan.bold,green.bold" \
		"make api" "make worker" "make ui"

# Stop existing services
stop:
	@echo "Stopping any existing API or Worker processes..."
	@lsof -ti :8080 | xargs kill -9 2>/dev/null || true
	@lsof -ti :8081 | xargs kill -9 2>/dev/null || true

# Run the API server
api:
	cd api && go run cmd/api/main.go

# Run the background worker
worker:
	cd api && go run cmd/worker/main.go

# Run the UI locally
ui: check-deps
	cd ui && npm run dev

# Helper to check if node_modules exists, install if missing
check-deps:
	@if [ ! -d "ui/node_modules" ]; then \
		echo "Installing UI dependencies..."; \
		cd ui && npm install; \
	fi

# Open database shell
db-shell:
	docker exec -it notification-service-postgres-1 psql -U notif -d notifdb

# Clean build artifacts
clean:
	cd api && go clean -cache -modcache
	rm -rf ui/.next ui/node_modules
