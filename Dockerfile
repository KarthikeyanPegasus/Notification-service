# syntax=docker/dockerfile:1
# Multi-stage Dockerfile for api and worker binaries.
# Usage: docker build --build-arg APP_NAME=api|worker -t notification-service-${APP_NAME} .
ARG APP_NAME=api

# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder
ARG APP_NAME

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src
COPY api/go.mod api/go.sum ./
RUN go mod download

COPY api/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/${APP_NAME} ./cmd/${APP_NAME}

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:latest AS final
RUN apk add --no-cache ca-certificates tzdata
ARG APP_NAME

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /out/${APP_NAME} /app
COPY --from=builder /src/config /config
COPY --from=builder /src/migrations /migrations

EXPOSE 8080
ENTRYPOINT ["/app"]
