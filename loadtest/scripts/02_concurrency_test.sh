#!/bin/bash

# Concurrency Test - Find the concurrency limit
# Tests with increasing concurrent connections

set -e

SERVER_URL="${SERVER_URL:-http://localhost:8080}"
ENDPOINT="${ENDPOINT:-/api/v1/container}"
RESULTS_DIR="loadtest/results/concurrency_$(date +%Y%m%d_%H%M%S)"

mkdir -p "$RESULTS_DIR"

echo "🔄 Running Concurrency Test..."
echo "Server: $SERVER_URL"
echo "Endpoint: $ENDPOINT"
echo "Results: $RESULTS_DIR"
echo ""

# Test different concurrency levels
CONCURRENCY_LEVELS=(50 100 250 500 1000 2000)

for c in "${CONCURRENCY_LEVELS[@]}"; do
    n=$((c * 100))  # Total requests = concurrency * 100
    
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Testing with $c concurrent connections ($n total requests)..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    hey -c "$c" -n "$n" "$SERVER_URL$ENDPOINT" | tee "$RESULTS_DIR/concurrency_${c}.txt"
    
    echo ""
    echo "Waiting 5 seconds before next test..."
    sleep 5
done

echo ""
echo "✅ Concurrency test complete!"
echo "Results saved to $RESULTS_DIR/"
echo ""
echo "Summary of p95 latencies:"
grep "95%" "$RESULTS_DIR"/*.txt | awk '{print $1, $3, $4}'
