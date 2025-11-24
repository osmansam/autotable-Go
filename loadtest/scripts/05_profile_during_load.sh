#!/bin/bash

# Profile During Load - Collect pprof data while running load test
# This script runs a load test and collects CPU, memory, and goroutine profiles

set -e

SERVER_URL="${SERVER_URL:-http://localhost:8080}"
ENDPOINT="${ENDPOINT:-/api/v1/container}"
RESULTS_DIR="loadtest/results/profile_$(date +%Y%m%d_%H%M%S)"

mkdir -p "$RESULTS_DIR"

echo "🔬 Running Load Test with Profiling..."
echo "Server: $SERVER_URL"
echo "Endpoint: $ENDPOINT"
echo "Results: $RESULTS_DIR"
echo ""

# Check if pprof endpoints are available
echo "Checking pprof endpoints..."
if ! curl -s "$SERVER_URL/debug/pprof/" > /dev/null; then
    echo "❌ pprof endpoints not available. Make sure your server has pprof enabled."
    exit 1
fi
echo "✅ pprof endpoints are available"
echo ""

# Start load test in background
echo "Starting load test (500 concurrent, 50000 requests)..."
hey -c 500 -n 50000 "$SERVER_URL$ENDPOINT" > "$RESULTS_DIR/load_test.txt" &
LOAD_PID=$!

# Wait for load to ramp up
echo "Waiting 5 seconds for load to ramp up..."
sleep 5

# Collect CPU profile
echo "Collecting CPU profile (30 seconds)..."
curl -s "$SERVER_URL/debug/pprof/profile?seconds=30" > "$RESULTS_DIR/cpu.prof"
echo "✅ CPU profile collected"

# Collect heap profile
echo "Collecting heap profile..."
curl -s "$SERVER_URL/debug/pprof/heap" > "$RESULTS_DIR/heap.prof"
echo "✅ Heap profile collected"

# Collect goroutine profile
echo "Collecting goroutine profile..."
curl -s "$SERVER_URL/debug/pprof/goroutine" > "$RESULTS_DIR/goroutine.prof"
echo "✅ Goroutine profile collected"

# Collect allocs profile
echo "Collecting allocs profile..."
curl -s "$SERVER_URL/debug/pprof/allocs" > "$RESULTS_DIR/allocs.prof"
echo "✅ Allocs profile collected"

# Collect runtime metrics
echo "Collecting runtime metrics..."
curl -s "$SERVER_URL/debug/vars" > "$RESULTS_DIR/metrics.json"
echo "✅ Runtime metrics collected"

# Wait for load test to complete
echo "Waiting for load test to complete..."
wait $LOAD_PID

echo ""
echo "✅ Profiling complete!"
echo "Results saved to $RESULTS_DIR/"
echo ""
echo "Analyze profiles with:"
echo "  go tool pprof -http=:8081 $RESULTS_DIR/cpu.prof"
echo "  go tool pprof -http=:8081 $RESULTS_DIR/heap.prof"
echo "  go tool pprof -http=:8081 $RESULTS_DIR/goroutine.prof"
echo ""
echo "View metrics:"
echo "  cat $RESULTS_DIR/metrics.json | jq"
