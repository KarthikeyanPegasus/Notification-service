# Plan: Show Workflow Orchestration in Notification Details

## Goal
Display which workflow orchestration engine (Temporal / Cadence / Go Routines) was used to dispatch each notification, both in the detail page and optionally the table.

## Approach — DB Column
Store the orchestration type on the `notifications` row so it's accurate for historical notifications (not derived from current config).

## Files to Modify

### Backend (5 files)

1. **`api/migrations/003_add_notification_orchestration.up.sql`** (NEW)
   - `ALTER TABLE notifications ADD COLUMN IF NOT EXISTS orchestration VARCHAR(32) NOT NULL DEFAULT '';`

2. **`api/internal/domain/notification.go`**
   - Add `Orchestration string` field to `Notification` struct with JSON/DB tags

3. **`api/internal/repository/notification_repo.go`**
   - Add `orchestration` to INSERT query
   - Add `n.orchestration` to all SELECT queries
   - Update `scanNotification` and `scanNotificationRow` to scan the new column
   - Add `SetOrchestration(ctx, id, orchestration)` method

4. **`api/internal/service/notification_service.go`**
   - In `publishImmediate()`: After resolving the workflow client, call `repo.SetOrchestration(ctx, n.ID, cli.ProviderName())`
   - Also set it in the Kafka fallback path

5. **`api/internal/handler/notification_handler.go`**
   - Return `orchestration` in the notification detail response (it'll be included via JSON tags)

### UI (2 files)

6. **`ui/src/types/index.ts`**
   - Add optional `orchestration?: string` to `Notification` interface

7. **`ui/src/app/notifications/[id]/page.tsx`**
   - Add `<DetailRow label="Orchestration" ...>` showing engine type with a badge
