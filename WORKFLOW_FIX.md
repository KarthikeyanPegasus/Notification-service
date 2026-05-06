# Workflow Orchestration Fix

## Problem
Workflows were not starting when client-specific orchestration configs were added, even though the configuration was saved correctly in the database.

## Root Cause
The global `cadence.mode: standalone` setting was blocking **all** workflow execution, including client-specific configurations. The code checked the global mode first and returned `nil` immediately, never looking up the client's custom config.

## Solution
Modified `/api/internal/service/workflow_client_provider.go` to:
1. **Check client-specific configs first** before applying global standalone mode
2. **Only apply standalone mode** when no client-specific config exists
3. **Add proper error logging** when workflow engine creation fails

### Changes Made

1. **workflow_client_provider.go:80-106** - Reordered logic to check client configs before global mode
2. **workflow_client_provider.go:160-184** - Added error logging when engine creation fails
3. **worker/manager.go:554-567** - Added error logging when getting client workflow engines
4. **scripts/test-workflow-config.sh** - Added diagnostic script

## How Client-Specific Configs Work Now

| Scenario | Global Mode | Client Config | Result |
|----------|-------------|---------------|--------|
| Global request (no API key) | standalone | N/A | No workflow (uses Kafka) |
| Global request (no API key) | temporal/cadence | N/A | Uses global engine |
| Client request | standalone | **Has config** | ✅ Uses client engine |
| Client request | standalone | No config | No workflow (fallback to standalone) |
| Client request | temporal/cadence | Has config | Uses client engine |
| Client request | temporal/cadence | No config | Uses global engine |

## Testing

### 1. Check Current State
```bash
./scripts/test-workflow-config.sh
```

This will show:
- Number of workers running
- Workers by channel and priority
- Whether client-specific workers exist

### 2. Restart Worker
After the code changes, restart the worker process:
```bash
# Kill the current worker
pkill -f "go run cmd/worker/main.go"

# Start new worker
make worker
```

### 3. Add Client Orchestration Config
1. Go to **Client Settings** → **Workflow Orchestration**
2. Select provider (Temporal or Cadence)
3. Configure:
   - **Host:Port**: e.g., `localhost:7233`
   - **Namespace/Domain**: e.g., `default`
4. Save

### 4. Wait for Reconciliation
- In **mock/redis** pubsub mode: ~instant (config reload event)
- In **Kafka** mode: up to **30 seconds** (next reconcile cycle)

### 5. Verify Workers Started
```bash
curl -s http://localhost:8081/workers | jq '.by_client'
```

Should show your client ID with workers.

### 6. Check Logs for Errors
```bash
# Check for workflow engine creation errors
tail -f api/logs/worker.log | grep -i "workflow engine"

# Check for client worker creation
tail -f api/logs/worker.log | grep -i "client_id"
```

## Common Issues

### Workers Not Appearing
**Symptom**: `by_client` is null or doesn't show your client

**Possible Causes**:
1. **Temporal/Cadence not running**
   - Check: `nc -zv localhost 7233`
   - Solution: Start your Temporal/Cadence server
   
2. **Incorrect host/port or namespace**
   - Check logs for "failed to create workflow engine"
   - Solution: Verify config matches your server
   
3. **Reconcile hasn't run yet** (Kafka mode only)
   - Wait 30 seconds after saving config
   - Or restart worker to force immediate reconcile

4. **Config not saved**
   - Check database: 
   ```sql
   SELECT vendor_type, api_key_id, is_active 
   FROM vendor_configs 
   WHERE vendor_type = 'workflow_orchestration';
   ```

### Connection Errors in Logs
```
failed to create workflow engine: connection refused
```

**Solution**: 
1. Verify your Temporal/Cadence server is running
2. Check firewall rules
3. Test connectivity: `telnet localhost 7233`

### Silent Failures
After this fix, errors are now **logged** instead of silently ignored. Check worker logs for specific error messages.

## Architecture Notes

### Workflow Modes

1. **Global Mode** (`config.yaml` → `cadence.mode`)
   - `standalone`: No global engine, publish to Kafka
   - `temporal`: Global Temporal engine
   - `cadence`: Global Cadence engine

2. **Client Mode** (Database → `vendor_configs` table)
   - Per-client override via workflow orchestration settings
   - Always takes precedence over global mode

### Worker Manager Reconciliation

- Runs every **30 seconds** automatically
- Triggered immediately on config reload (mock/redis modes)
- Creates workers for: Global + each client with orchestration config
- Each worker handles: 1 channel × 1 priority × 1 client scope

## Migration Notes

If you're upgrading from a version without this fix:

1. **No database changes needed** - this is a code-only fix
2. **Restart workers** to pick up the changes
3. **Existing configs will now work** - no need to re-save them
4. **Check logs** - errors that were silent before will now appear

## Support

If workflows still don't start after this fix:

1. Run the diagnostic script: `./scripts/test-workflow-config.sh`
2. Check worker logs: `tail -f api/logs/worker.log`
3. Verify your Temporal/Cadence server is accessible
4. Confirm config is in database with `is_active = true`
