#!/bin/bash
set -e

# Test script for workflow orchestration configuration
# This script helps diagnose why workflows aren't starting

echo "=== Workflow Orchestration Diagnostic Tool ==="
echo ""

# Check if API is running
if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "❌ API is not running on port 8080"
    echo "   Start it with: make api"
    exit 1
fi
echo "✓ API is running"

# Check if Worker is running
if ! curl -s http://localhost:8081/health > /dev/null 2>&1; then
    echo "❌ Worker is not running on port 8081"
    echo "   Start it with: make worker"
    exit 1
fi
echo "✓ Worker is running"

# Check worker state
echo ""
echo "=== Worker State ==="
WORKER_STATE=$(curl -s http://localhost:8081/workers)
echo "$WORKER_STATE" | jq '.'

TOTAL_WORKERS=$(echo "$WORKER_STATE" | jq -r '.total // 0')
if [ "$TOTAL_WORKERS" -eq 0 ]; then
    echo ""
    echo "⚠️  No workers are running!"
    echo ""
    echo "Possible reasons:"
    echo "1. No workflow orchestration configs are configured"
    echo "2. Workflow engine creation is failing (check worker logs)"
    echo "3. System is in standalone mode but Temporal/Cadence is not running"
    echo ""
    echo "Check worker logs for errors:"
    echo "  grep 'workflow engine' api/logs/worker.log"
    echo "  grep 'failed to create' api/logs/worker.log"
    echo ""
else
    echo "✓ $TOTAL_WORKERS workflow workers are running"
fi

echo ""
echo "=== Next Steps ==="
echo "1. Check the worker logs for 'failed to create workflow engine' errors"
echo "2. Verify your Temporal/Cadence server is running and accessible"
echo "3. Test connectivity to your workflow server:"
echo "   nc -zv <host> <port>"
echo "4. Check the workflow orchestration config in client settings"
echo "5. Wait 30 seconds for the next reconcile cycle after saving config"
echo ""
