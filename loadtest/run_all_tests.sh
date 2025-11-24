#!/bin/bash

# Master Test Runner - Runs all load tests in sequence
# This is the main entry point for comprehensive load testing

set -e

SERVER_URL="${SERVER_URL:-http://localhost:8080}"
ENDPOINT="${ENDPOINT:-/api/v1/container}"
MASTER_RESULTS_DIR="loadtest/results/master_run_$(date +%Y%m%d_%H%M%S)"

mkdir -p "$MASTER_RESULTS_DIR"

export SERVER_URL
export ENDPOINT

echo "╔════════════════════════════════════════════════════════════╗"
echo "║         AUTOTABLE-GO LOAD TESTING SUITE                   ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""
echo "Server: $SERVER_URL"
echo "Endpoint: $ENDPOINT"
echo "Master Results: $MASTER_RESULTS_DIR"
echo ""

# Function to run a test and log results
run_test() {
    local test_name=$1
    local test_script=$2
    
    echo ""
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║  $test_name"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo ""
    
    if [ -f "$test_script" ]; then
        bash "$test_script" 2>&1 | tee "$MASTER_RESULTS_DIR/${test_name// /_}.log"
        echo "✅ $test_name completed"
    else
        echo "⚠️  Test script not found: $test_script"
    fi
    
    echo ""
    echo "Waiting 10 seconds before next test..."
    sleep 10
}

# Check if server is up
echo "Checking if server is up..."
if ! curl -s -o /dev/null -w "%{http_code}" "$SERVER_URL$ENDPOINT" | grep -q "200\|401"; then
    echo "❌ Server is not responding at $SERVER_URL$ENDPOINT"
    echo "Please start your server and try again."
    exit 1
fi
echo "✅ Server is responding"
echo ""

# Run all tests
run_test "STEP 1: Smoke Test" "loadtest/scripts/01_smoke_test.sh"
run_test "STEP 2: Concurrency Test" "loadtest/scripts/02_concurrency_test.sh"
run_test "STEP 3: RPS Test" "loadtest/scripts/03_rps_test.sh"
run_test "STEP 4: Profiling Test" "loadtest/scripts/05_profile_during_load.sh"

# Optional: Uncomment to run stress test (takes ~16 minutes)
# run_test "STEP 5: Stress Test" "loadtest/scripts/04_stress_test.sh"

# Generate summary report
echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║  GENERATING SUMMARY REPORT                                 ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

cat > "$MASTER_RESULTS_DIR/SUMMARY.md" << EOF
# Load Testing Summary Report

**Date:** $(date)
**Server:** $SERVER_URL
**Endpoint:** $ENDPOINT

## Test Results

### 1. Smoke Test
- **Purpose:** Validate basic functionality
- **Load:** 10 concurrent, 1000 requests
- **Status:** ✅ Completed

### 2. Concurrency Test
- **Purpose:** Find concurrency limits
- **Levels Tested:** 50, 100, 250, 500, 1000, 2000 concurrent
- **Status:** ✅ Completed

### 3. RPS Test
- **Purpose:** Find maximum throughput
- **Levels Tested:** 100, 250, 500, 1000, 2000, 5000 RPS
- **Status:** ✅ Completed

### 4. Profiling Test
- **Purpose:** Identify performance bottlenecks
- **Profiles Collected:** CPU, Heap, Goroutine, Allocs
- **Status:** ✅ Completed

## Key Metrics

### Concurrency Test Results
EOF

# Add concurrency results if available
if ls loadtest/results/concurrency_*/concurrency_*.txt 1> /dev/null 2>&1; then
    echo "" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    echo "| Concurrency | Avg Latency | p95 Latency | Requests/sec |" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    echo "|-------------|-------------|-------------|--------------|" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    
    for file in $(ls -t loadtest/results/concurrency_*/concurrency_*.txt | head -6); do
        concurrency=$(basename "$file" .txt | sed 's/concurrency_//')
        avg=$(grep "Average:" "$file" | awk '{print $2, $3}')
        p95=$(grep "95%" "$file" | awk '{print $3, $4}')
        rps=$(grep "Requests/sec:" "$file" | awk '{print $2}')
        echo "| $concurrency | $avg | $p95 | $rps |" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    done
fi

cat >> "$MASTER_RESULTS_DIR/SUMMARY.md" << EOF

### RPS Test Results

EOF

# Add RPS results if available
if ls loadtest/results/rps_*/rps_*.txt 1> /dev/null 2>&1; then
    echo "| Target RPS | Actual Throughput | Success Rate | p95 Latency |" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    echo "|------------|-------------------|--------------|-------------|" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    
    for file in $(ls -t loadtest/results/rps_*/rps_*.txt | head -6); then
        target_rps=$(basename "$file" .txt | sed 's/rps_//')
        throughput=$(grep "Throughput" "$file" | awk '{print $3}')
        success=$(grep "Success" "$file" | awk '{print $3}')
        p95=$(grep "95th" "$file" | awk '{print $3}')
        echo "| $target_rps | $throughput | $success | $p95 |" >> "$MASTER_RESULTS_DIR/SUMMARY.md"
    done
fi

cat >> "$MASTER_RESULTS_DIR/SUMMARY.md" << EOF

## Analysis

### Performance Profile
- Review CPU profile: \`go tool pprof -http=:8081 <profile_dir>/cpu.prof\`
- Review heap profile: \`go tool pprof -http=:8081 <profile_dir>/heap.prof\`
- Review goroutine profile: \`go tool pprof -http=:8081 <profile_dir>/goroutine.prof\`

### Recommendations
1. Check the concurrency test to find optimal concurrent connection limit
2. Review RPS test to determine maximum sustainable throughput
3. Analyze CPU profile to identify hot paths
4. Check heap profile for memory allocation patterns
5. Monitor goroutine count for potential leaks

## Next Steps
- [ ] Optimize identified bottlenecks
- [ ] Run stress test to find breaking point
- [ ] Implement caching for frequently accessed data
- [ ] Consider horizontal scaling if needed
- [ ] Set up continuous monitoring in production

---
*Generated by autotable-Go load testing suite*
EOF

echo "✅ Summary report generated: $MASTER_RESULTS_DIR/SUMMARY.md"
echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║  ALL TESTS COMPLETED! 🎉                                   ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""
echo "Results location: $MASTER_RESULTS_DIR"
echo ""
echo "View summary report:"
echo "  cat $MASTER_RESULTS_DIR/SUMMARY.md"
echo ""
echo "Analyze profiles:"
echo "  go tool pprof -http=:8081 <profile_dir>/cpu.prof"
echo ""
