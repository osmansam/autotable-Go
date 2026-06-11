# autotable-Go

## Local Observability

This project includes a simple local Prometheus + Grafana setup for the Go/Fiber API metrics exposed at `/metrics`, plus OpenTelemetry traces through the OpenTelemetry Collector and Grafana Tempo.

### Start

1. Run the Go app on port `3002`.

2. Start Prometheus, Grafana, the OpenTelemetry Collector, and Tempo:

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

Grafana is provisioned with Prometheus and Tempo data sources and a starter dashboard named `Autotable API Metrics`.

### Traces

The Go app exports traces to the local OpenTelemetry Collector at `localhost:4317` by default.

Optional environment variables:

```text
OTEL_SERVICE_NAME=autotable-go
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
OTEL_TRACING_ENABLED=true
```

In Grafana, open `Explore`, select the `Tempo` data source, and use TraceQL queries such as:

```text
{ resource.service.name = "autotable-go" }
{ span.tenantId = "your-tenant-id" }
{ span.projectId = "your-project-id" }
{ span.workflow_name = "your_workflow_name" }
{ span.request_id = "your-request-id" }
```

The `http.route` span attribute still shows Fiber route placeholders such as
`:tenantSlug` and `:projectSlug` because those are the public URL params. Use
the `tenantId` and `projectId` span attributes when correlating traces with
structured logs.

### Endpoints

```text
Go app metrics: http://localhost:3002/metrics
Prometheus:     http://localhost:9090
Grafana:        http://localhost:3000
Tempo:          http://localhost:3200
OTLP gRPC:      localhost:4317
OTLP HTTP:      localhost:4318
```
