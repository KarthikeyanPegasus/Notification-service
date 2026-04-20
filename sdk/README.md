# NotifyHub Go SDK

Minimal Go client for the Notification Service HTTP API (`/v1`).

## Install

```bash
go get github.com/spidey/notification-service/sdk/go
```

## Usage

```go
package main

import (
	"context"
	"log"

	notification "github.com/spidey/notification-service/sdk/go"
)

func main() {
	ctx := context.Background()
	client := notification.New(
		notification.WithBaseURL("http://localhost:8080/v1"),
		notification.WithBearerToken("<jwt>"),
	)

	_, err := client.Notifications.NotifyBySlack(ctx,
		"550e8400-e29b-4142-a273-041772000000",
		"slack-example-1",
		"transactional",
		"https://hooks.slack.com/services/XXX/YYY/ZZZ",
		&notification.NotifyOptions{Body: "Hello from NotifyHub"},
	)
	if err != nil {
		log.Fatal(err)
	}
}
```

## API surface

- **Models**: `sdk/go/models.go` — includes `ChannelSlack`, `SendRequest.Body`, and `NotifyOptions`.
- **Per-channel sends**: `NotifyByEmail`, `NotifyBySMS`, `NotifyByPush`, `NotifyByWebSocket`, `NotifyByWebhook`, `NotifyBySlack` (each wraps `Send` with a single channel). Use `Send` when you need multiple channels in one request.
- **Calls**: `Notifications.Send`, `List`, `Get`; OTP and reports helpers live alongside in the same module.

Regenerate or extend types from `api/docs/openapi.yaml` when the API changes.
