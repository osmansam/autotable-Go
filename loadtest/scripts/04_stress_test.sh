#!/bin/bash

# Stress Test - Push the system until it breaks
# Gradually increases load to find breaking point

set -e

SERVER_URL="${SERVER_URL:-http://localhost:8080}"
ENDPOINT="${ENDPOINT:-/api/v1/container}"
RESULTS_DIR="loadtest/results/stress_$(date +%Y%m%d_%H%M%S)"

mkdir -p "$RESULTS_DIR"

echo "💥 Running Stress Test..."
echo "Server: $SERVER_URL"
echo "Endpoint: $ENDPOINT"
echo "Results: $RESULTS_DIR"
echo ""
echo "⚠️  This test will push your server to its limits!"
echo "Press Ctrl+C to stop at any time."
echo ""

# Create k6 stress test script
cat > "$RESULTS_DIR/stress_test.js" << 'EOF'
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '2m', target: 100 },   // Warm up
    { duration: '3m', target: 500 },   // Increase load
    { duration: '3m', target: 1000 },  // Push harder
    { duration: '3m', target: 2000 },  // Push to limits
    { duration: '3m', target: 3000 },  // Break it
    { duration: '2m', target: 0 },     // Recovery
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'], // 95% < 2s
    errors: ['rate<0.1'], // Error rate < 10%
  },
};

const SERVER_URL = __ENV.SERVER_URL || 'http://localhost:8080';
const ENDPOINT = __ENV.ENDPOINT || '/api/v1/container';

export default function () {
  const res = http.get(`${SERVER_URL}${ENDPOINT}`);
  
  check(res, {
    'status 200': (r) => r.status === 200,
  }) || errorRate.add(1);
  
  sleep(0.1);
}
EOF

# Run k6 stress test
k6 run \
    --out json="$RESULTS_DIR/stress_test.json" \
    -e SERVER_URL="$SERVER_URL" \
    -e ENDPOINT="$ENDPOINT" \
    "$RESULTS_DIR/stress_test.js" | tee "$RESULTS_DIR/stress_test.log"

echo ""
echo "✅ Stress test complete!"
echo "Results saved to $RESULTS_DIR/"
echo ""
echo "Check the logs to see at what point the system started failing."
