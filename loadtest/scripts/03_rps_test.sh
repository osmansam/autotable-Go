#!/bin/bash

# RPS Test - Find maximum sustainable throughput
# Tests with increasing requests per second

set -e

SERVER_URL="${SERVER_URL:-http://localhost:8080}"
ENDPOINT="${ENDPOINT:-/api/v1/container}"
RESULTS_DIR="loadtest/results/rps_$(date +%Y%m%d_%H%M%S)"

mkdir -p "$RESULTS_DIR"

echo "📊 Running RPS Test..."
echo "Server: $SERVER_URL"
echo "Endpoint: $ENDPOINT"
echo "Results: $RESULTS_DIR"
echo ""

# Test different RPS levels
RPS_LEVELS=(100 250 500 1000 2000 5000)
DURATION=60  # seconds

for rps in "${RPS_LEVELS[@]}"; do
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Testing at $rps requests/second for ${DURATION}s..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    echo "GET $SERVER_URL$ENDPOINT" | \
        vegeta attack -rate="$rps" -duration="${DURATION}s" | \
        tee "$RESULTS_DIR/rps_${rps}.bin" | \
        vegeta report | \
        tee "$RESULTS_DIR/rps_${rps}.txt"
    
    # Generate plot
    vegeta plot "$RESULTS_DIR/rps_${rps}.bin" > "$RESULTS_DIR/rps_${rps}.html"
    
    echo ""
    echo "Waiting 10 seconds before next test..."
    sleep 10
done

echo ""
echo "✅ RPS test complete!"
echo "Results saved to $RESULTS_DIR/"
echo ""
echo "Summary of success rates:"
grep "Success" "$RESULTS_DIR"/*.txt
