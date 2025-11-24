#!/bin/bash

# Smoke Test - Quick validation that the server is working
# This runs a minimal load test to verify basic functionality

set -e

SERVER_URL="${SERVER_URL:-http://localhost:8080}"
ENDPOINT="${ENDPOINT:-/api/v1/container}"

echo "🔍 Running Smoke Test..."
echo "Server: $SERVER_URL"
echo "Endpoint: $ENDPOINT"
echo ""

# Check if server is responding
echo "Checking if server is up..."
if ! curl -s -o /dev/null -w "%{http_code}" "$SERVER_URL$ENDPOINT" | grep -q "200\|401"; then
    echo "❌ Server is not responding at $SERVER_URL$ENDPOINT"
    exit 1
fi
echo "✅ Server is responding"
echo ""

# Run smoke test with hey
echo "Running smoke test (10 concurrent, 1000 requests)..."
hey -c 10 -n 1000 "$SERVER_URL$ENDPOINT" | tee loadtest/results/smoke_test_$(date +%Y%m%d_%H%M%S).txt

echo ""
echo "✅ Smoke test complete!"
echo "Results saved to loadtest/results/"
