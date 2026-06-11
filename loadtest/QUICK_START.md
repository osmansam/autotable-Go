# 🚀 Quick Start - Load Testing Your Server

Your server is now running with performance monitoring enabled!

## ✅ What's Ready

- ✅ Server running on port **3002**
- ✅ pprof profiling: `http://localhost:3002/debug/pprof/`
- ✅ Prometheus metrics: `http://localhost:3002/metrics`
- ✅ Load testing tools installed (vegeta, k6)
- ✅ Test scripts ready to run

## 🎯 Quick Tests (Run These Now!)

### 1. Quick Smoke Test (30 seconds)
```bash
cd /Users/osmansamilerdogan/Desktop/autotable-Go
~/go/bin/hey -c 10 -n 1000 http://localhost:3002/api/v1/container
```

### 2. View Live Metrics
```bash
# See Prometheus metrics, including Go runtime/process stats
curl http://localhost:3002/metrics
```

### 3. Medium Load Test (1 minute)
```bash
# 100 concurrent connections, 10,000 requests
~/go/bin/hey -c 100 -n 10000 http://localhost:3002/api/v1/container
```

### 4. RPS Test with vegeta (1 minute)
```bash
# 500 requests/second for 60 seconds
echo "GET http://localhost:3002/api/v1/container" | \
  vegeta attack -rate=500 -duration=60s | \
  vegeta report
```

### 5. Profile During Load
```bash
# Run this in one terminal
~/go/bin/hey -c 500 -n 50000 http://localhost:3002/api/v1/container &

# Immediately in another terminal, collect CPU profile
curl http://localhost:3002/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze it
go tool pprof -http=:8081 cpu.prof
```

## 🎬 Full Test Suite

Run all tests automatically:
```bash
cd /Users/osmansamilerdogan/Desktop/autotable-Go

# Update scripts to use correct port
export SERVER_URL=http://localhost:3002
export ENDPOINT=/api/v1/container

# Run all tests (takes ~15 minutes)
./loadtest/run_all_tests.sh
```

## 📊 Understanding Results

### Good Performance Indicators
- ✅ Average latency < 100ms
- ✅ p95 latency < 500ms
- ✅ p99 latency < 1000ms
- ✅ Error rate < 1%
- ✅ Throughput > 500 RPS

### What to Watch
- ⚠️ Increasing latency = approaching capacity
- ⚠️ Growing goroutines = potential leak
- ⚠️ Growing memory = memory leak
- ⚠️ High error rate = system overloaded

## 🔍 Quick Commands

```bash
# Check if server is responding
curl http://localhost:3002/api/v1/container

# View pprof index
curl http://localhost:3002/debug/pprof/

# Get current goroutine metric
curl -s http://localhost:3002/metrics | grep '^go_goroutines'

# Get memory metrics
curl -s http://localhost:3002/metrics | grep '^go_memstats'

# Quick 100 request test
~/go/bin/hey -n 100 http://localhost:3002/api/v1/container
```

## 🎯 Recommended First Test

Start with this simple test:
```bash
~/go/bin/hey -c 50 -n 5000 http://localhost:3002/api/v1/container
```

This will:
- Send 5000 requests
- With 50 concurrent connections
- Take about 10-30 seconds
- Give you baseline performance metrics

## 📈 Next Steps

1. Run the quick smoke test above
2. Check the metrics at `/metrics`
3. Run a medium load test
4. Collect a CPU profile
5. Run the full test suite when ready

## 🆘 Troubleshooting

**Command not found: hey**
```bash
# Use full path
~/go/bin/hey -n 100 http://localhost:3002/api/v1/container
```

**Connection refused**
```bash
# Check if server is running
curl http://localhost:3002/api/v1/container
```

**Want to see real-time updates**
```bash
# Watch metrics update
watch -n 1 'curl -s http://localhost:3002/metrics | grep -E "^(go_goroutines|go_memstats_alloc_bytes)"'
```
