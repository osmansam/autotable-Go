package observability

import (
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	Registry = prometheus.NewRegistry()

	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	workflowExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflow_executions_total",
			Help: "Total number of workflow executions.",
		},
		[]string{"status"},
	)

	workflowExecutionDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "workflow_execution_duration_seconds",
			Help:    "Workflow execution duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	workflowStepExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflow_step_executions_total",
			Help: "Total number of workflow step executions.",
		},
		[]string{"step_type", "status"},
	)

	workflowStepDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "workflow_step_duration_seconds",
			Help:    "Workflow step execution duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"step_type", "status"},
	)

	pipelineExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_executions_total",
			Help: "Total number of pipeline executions.",
		},
		[]string{"pipeline_name", "schema_name", "status"},
	)

	pipelineExecutionDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pipeline_execution_duration_seconds",
			Help:    "Pipeline execution duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"pipeline_name", "schema_name", "status"},
	)

	cacheRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_requests_total",
			Help: "Total number of cache operations.",
		},
		[]string{"operation", "schema_name", "result"},
	)

	outboxJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "outbox_jobs_total",
			Help: "Total number of outbox jobs processed.",
		},
		[]string{"operation", "status"},
	)

	websocketClientsConnected = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_clients_connected",
			Help: "Current number of connected websocket clients.",
		},
	)
)

func init() {
	Registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		httpRequestsTotal,
		httpRequestDurationSeconds,
		workflowExecutionsTotal,
		workflowExecutionDurationSeconds,
		workflowStepExecutionsTotal,
		workflowStepDurationSeconds,
		pipelineExecutionsTotal,
		pipelineExecutionDurationSeconds,
		cacheRequestsTotal,
		outboxJobsTotal,
		websocketClientsConnected,
	)
}

func RecordHTTPRequest(method, route string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	httpRequestsTotal.WithLabelValues(method, normalizeMetricLabel(route), status).Inc()
	httpRequestDurationSeconds.WithLabelValues(method, normalizeMetricLabel(route), status).Observe(duration.Seconds())
}

// RecordWorkflowExecution records workflow health with intentionally low-cardinality
// labels. Tenant/project/schema/workflow names belong in logs, not Prometheus labels.
func RecordWorkflowExecution(tenantID, projectID, workflowName, schemaName, status string, duration time.Duration) {
	workflowExecutionsTotal.WithLabelValues(normalizeStatus(status)).Inc()
	workflowExecutionDurationSeconds.WithLabelValues(normalizeStatus(status)).Observe(duration.Seconds())
}

// RecordWorkflowStepExecution records workflow step execution without using
// step IDs, workflow names, tenant IDs, or user-specific values as labels.
func RecordWorkflowStepExecution(tenantID, projectID, workflowName, stepType, status string, duration time.Duration) {
	workflowStepExecutionsTotal.WithLabelValues(normalizeMetricLabel(stepType), normalizeStatus(status)).Inc()
	workflowStepDurationSeconds.WithLabelValues(normalizeMetricLabel(stepType), normalizeStatus(status)).Observe(duration.Seconds())
}

// RecordPipelineExecution is intended for dynamic pipeline execution paths.
func RecordPipelineExecution(pipelineName, schemaName, status string, duration time.Duration) {
	pipelineExecutionsTotal.WithLabelValues(normalizeMetricLabel(pipelineName), normalizeMetricLabel(schemaName), normalizeStatus(status)).Inc()
	pipelineExecutionDurationSeconds.WithLabelValues(normalizeMetricLabel(pipelineName), normalizeMetricLabel(schemaName), normalizeStatus(status)).Observe(duration.Seconds())
}

// RecordCacheRequest records cache get/set/invalidate behavior. Do not pass
// exact Redis keys because they are high-cardinality and may contain user data.
func RecordCacheRequest(operation, schemaName, result string) {
	cacheRequestsTotal.WithLabelValues(normalizeMetricLabel(operation), normalizeMetricLabel(schemaName), normalizeMetricLabel(result)).Inc()
}

// RecordOutboxJob records background outbox processing results.
func RecordOutboxJob(operation, status string) {
	outboxJobsTotal.WithLabelValues(normalizeMetricLabel(operation), normalizeStatus(status)).Inc()
}

func SetWebsocketClientsConnected(count int) {
	websocketClientsConnected.Set(float64(count))
}

func normalizeStatus(value string) string {
	if value == "" {
		return "unknown"
	}
	return normalizeMetricLabel(value)
}

func normalizeMetricLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
