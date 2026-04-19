# System Architecture

## Overview

The notification service is a multi-channel, event-driven delivery platform built around three runtimes — an **HTTP API**, a **pool of channel workers**, and **Temporal** for durable workflow orchestration — backed by PostgreSQL and Redis, with GCP Pub/Sub or Redis Pub/Sub as the message bus.

---

## Component Diagram

```mermaid
flowchart TB
    subgraph Clients["Clients"]
        WEB["Web / Mobile App"]
        SVC["Internal Services"]
        EXT["External Event Source"]
    end

    subgraph API["API Server  :8080"]
        GIN["Gin Router\n+ Middleware\n(JWT · Rate-limit · CORS)"]
        NH["NotificationHandler"]
        OH["OTPHandler"]
        WH["WebhookHandler\n(provider callbacks)"]
        PH["PreferencesHandler"]
        GH["GovernanceHandler"]
        RH["ReportHandler"]
        NS["NotificationService\n(idempotency · prefs · governance)"]
        TS["TemplateService"]
        OS["OTPService"]
        RS["ReconciliationService"]
    end

    subgraph Temporal["Temporal :7233"]
        TW["NotificationWorkflow"]
        TA1["CheckPreferencesActivity"]
        TA2["RenderTemplateActivity"]
        TA3["PublishToPubSubActivity"]
        TA4["LogDeliveryActivity"]
    end

    subgraph PubSub["Message Bus  (GCP Pub/Sub · Redis)"]
        T_EMAIL["notifications-email"]
        T_SMS["notifications-sms"]
        T_PUSH["notifications-push"]
        T_WS["notifications-websocket"]
        T_HOOK["notifications-webhook"]
        T_OTP["notifications-otp"]
        T_ING["notifications-ingress"]
        T_DLQ["notifications-dlq"]
        T_CFG["internal-config-reload"]
    end

    subgraph Workers["Worker Pool  :8081"]
        EW["EventWorker\n(ingress fan-out)"]
        EMW["EmailWorker"]
        SMSW["SMSWorker"]
        PUW["PushWorker"]
        WSW["WebSocketWorker"]
        WHW["WebhookWorker"]
        CB["Circuit Breaker\nRegistry"]
    end

    subgraph Providers["External Providers"]
        P_EMAIL["SES · Mailgun · SMTP"]
        P_SMS["Twilio · Plivo · Vonage"]
        P_PUSH["FCM · APNs"]
        P_WH["HTTP Webhooks"]
    end

    subgraph Storage["Storage"]
        PG[("PostgreSQL 17\nnotifications\nattempts · events\ntemplates · tokens")]
        RD[("Redis 7\ncache · rate-limit\nOTP · prefs\ncircuit states")]
    end

    subgraph Observability["Observability"]
        PROM["Prometheus :9090"]
        GRAF["Grafana :3001"]
    end

    WEB -->|"POST /v1/notifications\nBearer JWT"| GIN
    SVC -->|"POST /v1/otp/send\nX-Service-Token"| GIN
    EXT -->|"publish"| T_ING

    GIN --> NH & OH & WH & PH & GH & RH
    NH --> NS
    OH --> OS
    NS --> TS
    NS -->|"start workflow"| TW
    NS --> PG
    NS --> RD

    TW --> TA1 --> TA2 --> TA3 --> TA4
    TA3 -->|"publish"| T_EMAIL & T_SMS & T_PUSH & T_WS & T_HOOK & T_OTP

    T_ING --> EW
    EW -->|"fan-out publish"| T_EMAIL & T_SMS & T_PUSH

    T_EMAIL --> EMW
    T_SMS   --> SMSW
    T_PUSH  --> PUW
    T_WS    --> WSW
    T_HOOK  --> WHW

    EMW --> CB --> P_EMAIL
    SMSW --> CB --> P_SMS
    PUW  --> CB --> P_PUSH
    WHW  --> CB --> P_WH

    EMW & SMSW & PUW & WSW & WHW -->|"upsert status\ninsert attempt+event"| PG
    EMW & SMSW & PUW -->|"failed → nack"| T_DLQ

    P_EMAIL & P_SMS -->|"delivery webhook"| WH
    WH -->|"update status"| PG

    T_CFG -->|"hot-reload vendor config"| Workers

    API  -->|"/metrics"| PROM
    Workers -->|"/metrics"| PROM
    PROM --> GRAF
```

---

## Layer Responsibilities

| Layer | Technology | Responsibility |
|---|---|---|
| **API** | Gin · JWT · pgx | Accept requests, idempotency, governance checks, template rendering, workflow start |
| **Workflow Engine** | Temporal 1.24 | Durable orchestration, retry policies (5 attempts, 2× backoff), scheduled delivery |
| **Message Bus** | GCP Pub/Sub · Redis Pub/Sub | Decouple API from workers; per-channel topics; DLQ for exhausted retries |
| **Workers** | Go goroutines | Subscribe per-channel, dispatch via provider with circuit-breaker fallback, record attempts |
| **Circuit Breaker** | Sony Gobreaker | Per-vendor open/half-open/closed state; fast-fail + automatic recovery |
| **Providers** | SES, Mailgun, SMTP / Twilio, Plivo, Vonage / FCM / HTTP | Actual delivery to end-user |
| **PostgreSQL** | pgx · golang-migrate | Source of truth: notifications, attempts, events, templates, governance |
| **Redis** | go-redis v9 | Rate-limit sliding windows, OTP TTLs, preference cache, circuit-breaker state |
| **Observability** | Prometheus · Grafana | Per-channel delivery rate, latency percentiles, error rates, circuit-breaker flips |

---

## Deployment Topology

```mermaid
flowchart LR
    subgraph K8s["Kubernetes Cluster"]
        subgraph ns["namespace: notification-system"]
            API_D["API Deployment\nreplicas: 2–10\nHPA @ 70% CPU"]
            WRK_D["Worker Deployment\nreplicas: 2–8\nHPA @ 70% CPU"]
            UI_D["UI Deployment\nreplicas: 1"]
            ING["Ingress\n(nginx)"]
        end
    end

    subgraph Infra["Managed Infrastructure"]
        PG2[("PostgreSQL")]
        RD2[("Redis")]
        PS["GCP Pub/Sub"]
        TMP["Temporal Cloud\nor self-hosted"]
    end

    Internet -->|"HTTPS"| ING
    ING --> API_D & UI_D
    API_D <-->|"pgx"| PG2
    API_D <-->|"go-redis"| RD2
    API_D <-->|"gRPC"| TMP
    TMP  -->|"publish"| PS
    PS   --> WRK_D
    WRK_D <-->|"pgx"| PG2
    WRK_D <-->|"go-redis"| RD2
```
