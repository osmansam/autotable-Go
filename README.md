# autotable-Go

## Local Observability

This project includes a simple local Prometheus + Grafana setup for the Go/Fiber API metrics exposed at `/metrics`, OpenTelemetry traces through the OpenTelemetry Collector and Grafana Tempo, and structured JSON logs shipped to Grafana Loki by Grafana Alloy.

### Start

1. Run the Go app on port `3002`.

2. Start Prometheus, Grafana, the OpenTelemetry Collector, Tempo, Loki, and Alloy:

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

Grafana is provisioned with Prometheus, Tempo, and Loki data sources and a starter dashboard named `Autotable API Metrics`.

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

### Logs

The Go app writes structured JSON logs to stdout and to `logs/autotable.log` by default. Grafana Alloy tails that file and sends logs to Loki.

Optional environment variables:

```text
LOG_LEVEL=info
LOG_FILE=logs/autotable.log
```

In Grafana, open `Explore`, select the `Loki` data source, and use LogQL queries such as:

```text
{service_name="autotable-go"}
{service_name="autotable-go", level="ERROR"}
{service_name="autotable-go"} |= "project_id"
{service_name="autotable-go"} |= "your-trace-id"
{service_name="autotable-go"} | json | request_id = "your-request-id"
```

From a Tempo trace view, use the logs link to jump to matching Loki entries by `trace_id`.

Alloy live debugging is enabled for local development. After changing `observability/alloy.yml` or Alloy command flags, recreate Alloy:

```bash
docker compose -f observability/docker-compose.observability.yml up -d --force-recreate alloy
```

In the Alloy UI, use live debugging on `loki.process.autotable`. Components such as `loki.source.file.autotable` and `loki.write.local` are healthy but do not support live debugging.

If Grafana Explore shows no Loki results, set the time range wide enough to include existing app logs or generate a fresh API request. You can verify Loki directly:

```bash
curl -G 'http://localhost:3100/loki/api/v1/query_range' \
  --data-urlencode 'query={service_name="autotable-go"}' \
  --data-urlencode 'limit=5'
```

### Endpoints

```text
Go app metrics: http://localhost:3002/metrics
Prometheus:     http://localhost:9090
Grafana:        http://localhost:3000
Tempo:          http://localhost:3200
Loki:           http://localhost:3100
Alloy:          http://localhost:12345
OTLP gRPC:      localhost:4317
OTLP HTTP:      localhost:4318
```
