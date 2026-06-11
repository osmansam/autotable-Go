# Load Testing Suite for autotable-Go

Complete load testing infrastructure for performance testing and capacity planning.

## Quick Start

### 1. Install Tools
```bash
chmod +x loadtest/install_tools.sh
./loadtest/install_tools.sh
```

### 2. Make Scripts Executable
```bash
chmod +x loadtest/scripts/*.sh
chmod +x loadtest/run_all_tests.sh
```

### 3. Run All Tests
```bash
# Make sure your server is running first!
./loadtest/run_all_tests.sh
```

## Individual Tests

### Smoke Test (Quick Validation)
```bash
./loadtest/scripts/01_smoke_test.sh
```

### Concurrency Test (Find Limits)
```bash
./loadtest/scripts/02_concurrency_test.sh
```

### RPS Test (Find Throughput)
```bash
./loadtest/scripts/03_rps_test.sh
```

### Stress Test (Find Breaking Point)
```bash
./loadtest/scripts/04_stress_test.sh
```

### Profile During Load
```bash
./loadtest/scripts/05_profile_during_load.sh
```

## Custom Configuration

Set environment variables to customize tests:

```bash
export SERVER_URL="http://localhost:8080"
export ENDPOINT="/api/v1/container"
./loadtest/run_all_tests.sh
```

## Analyzing Results

### View pprof Profiles
```bash
# CPU profile
go tool pprof -http=:8081 loadtest/results/profile_*/cpu.prof

# Memory profile
go tool pprof -http=:8081 loadtest/results/profile_*/heap.prof

# Goroutine profile
go tool pprof -http=:8081 loadtest/results/profile_*/goroutine.prof
```

### View Runtime Metrics
```bash
# While server is running
curl http://localhost:8080/metrics

# From saved results
cat loadtest/results/profile_*/metrics.prom
```

### View Test Results
```bash
# Latest concurrency test
cat loadtest/results/concurrency_*/concurrency_500.txt

# Latest RPS test
cat loadtest/results/rps_*/rps_1000.txt

# View HTML plots (RPS tests)
open loadtest/results/rps_*/rps_1000.html
```

## Results Directory Structure

```
loadtest/
├── install_tools.sh          # Tool installer
├── run_all_tests.sh          # Master test runner
├── scripts/
│   ├── 01_smoke_test.sh
│   ├── 02_concurrency_test.sh
│   ├── 03_rps_test.sh
│   ├── 04_stress_test.sh
│   └── 05_profile_during_load.sh
└── results/
    ├── master_run_*/         # Full test suite results
    ├── concurrency_*/        # Concurrency test results
    ├── rps_*/                # RPS test results
    ├── stress_*/             # Stress test results
    └── profile_*/            # Profiling data
```

## Performance Endpoints

Your server now exposes these endpoints:

- **pprof:** `http://localhost:8080/debug/pprof/`
- **Prometheus metrics:** `http://localhost:8080/metrics`

## Troubleshooting

### Server not responding
```bash
# Check if server is running
curl http://localhost:8080/api/v1/container

# Check server logs
# Look for "pprof available at..." message
```

### Tools not found
```bash
# Re-run installer
./loadtest/install_tools.sh

# Or install manually
go install github.com/rakyll/hey@latest
brew install vegeta k6
```

### Permission denied
```bash
# Make scripts executable
chmod +x loadtest/*.sh
chmod +x loadtest/scripts/*.sh
```

## Next Steps

1. ✅ Run smoke test to verify setup
2. ✅ Run full test suite
3. 📊 Analyze results and identify bottlenecks
4. 🔧 Optimize code based on profiling data
5. 🔄 Re-test to measure improvements
6. 📈 Set up continuous monitoring

## See Also

- [Complete Load Testing Plan](../.gemini/antigravity/brain/245de07b-3875-470c-a15c-e2fc8ef42078/load_testing_plan.md)
- [pprof Documentation](https://pkg.go.dev/net/http/pprof)
- [hey Documentation](https://github.com/rakyll/hey)
- [vegeta Documentation](https://github.com/tsenart/vegeta)
- [k6 Documentation](https://k6.io/docs/)
