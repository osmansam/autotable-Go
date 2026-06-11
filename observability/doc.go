// Package observability provides the basic logging and metrics surface for the
// API.
//
// Use the logging helpers from request handlers when a controller or service has
// useful business context to add:
//
//	observability.Info(c, "workflow execution started",
//		slog.String(observability.FieldWorkflowName, workflowName))
//
// Use the metrics helpers at operation boundaries, not inside tight loops. Pass
// stable names such as schema_name, workflow_name, pipeline_name, operation, and
// status. Do not pass request IDs, user IDs, document IDs, email addresses,
// exact Redis keys, request bodies, tokens, API keys, or passwords as metric
// labels.
package observability
