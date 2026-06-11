# autotable-Go

## Local Observability

This project includes a simple local Prometheus + Grafana setup for the Go/Fiber API metrics exposed at `/metrics`.

### Start

1. Run the Go app on port `3002`.

2. Start Prometheus and Grafana:

```bash
docker compose -f observability/docker-compose.observability.yml up
```

3. Open Prometheus:

```text
http://localhost:9090
```

4. Open Grafana:

```text
http://localhost:3000
```

5. Log in to Grafana:

```text
username: admin
password: admin
```

Grafana is provisioned with a Prometheus data source and a starter dashboard named `Autotable API Metrics`.

### Endpoints

```text
Go app metrics: http://localhost:3002/metrics
Prometheus:     http://localhost:9090
Grafana:        http://localhost:3000
```
