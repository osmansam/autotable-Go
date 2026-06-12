package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

const (
	LocalRequestID = "requestID"

	FieldRequestID    = "request_id"
	FieldTenantID     = "tenant_id"
	FieldProjectID    = "project_id"
	FieldSchemaName   = "schema_name"
	FieldWorkflowName = "workflow_name"
	FieldPipelineName = "pipeline_name"
	FieldOperation    = "operation"
	FieldStepType     = "step_type"
	FieldStatus       = "status"
	// user_id is allowed in logs for debugging, but never use it as a Prometheus label.
	FieldUserID     = "user_id"
	FieldDurationMS = "duration_ms"
	FieldError      = "error"
	FieldTraceID    = "trace_id"
	FieldSpanID     = "span_id"
)

var logger = newJSONLogger()

func newJSONLogger() *slog.Logger {
	level := new(slog.LevelVar)
	level.Set(parseLogLevel(os.Getenv("LOG_LEVEL")))

	output := io.Writer(os.Stdout)
	if logFile := strings.TrimSpace(os.Getenv("LOG_FILE")); logFile != "" {
		if file, err := openLogFile(logFile); err == nil {
			output = io.MultiWriter(os.Stdout, file)
		}
	} else if file, err := openLogFile("logs/autotable.log"); err == nil {
		output = io.MultiWriter(os.Stdout, file)
	}

	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: level,
	}))
}

func openLogFile(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Logger() *slog.Logger {
	return logger
}

func Debug(c *fiber.Ctx, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(c.UserContext(), slog.LevelDebug, msg, append(RequestAttrs(c), attrs...)...)
}

func Info(c *fiber.Ctx, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(c.UserContext(), slog.LevelInfo, msg, append(RequestAttrs(c), attrs...)...)
}

func Warn(c *fiber.Ctx, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(c.UserContext(), slog.LevelWarn, msg, append(RequestAttrs(c), attrs...)...)
}

func Error(c *fiber.Ctx, msg string, err error, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, slog.String(FieldError, err.Error()))
	}
	logger.LogAttrs(c.UserContext(), slog.LevelError, msg, append(RequestAttrs(c), attrs...)...)
}

func DebugCtx(ctx context.Context, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

func InfoCtx(ctx context.Context, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func WarnCtx(ctx context.Context, msg string, attrs ...slog.Attr) {
	logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

func ErrorCtx(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, slog.String(FieldError, err.Error()))
	}
	logger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

func TenantProjectAttrs(tenantID, projectID string) []slog.Attr {
	attrs := make([]slog.Attr, 0, 2)
	attrs = appendTrimmedStringAttr(attrs, FieldTenantID, tenantID)
	attrs = appendTrimmedStringAttr(attrs, FieldProjectID, projectID)
	return attrs
}

func WorkflowAttrs(tenantID, projectID, schemaName, workflowName string) []slog.Attr {
	attrs := TenantProjectAttrs(tenantID, projectID)
	attrs = appendTrimmedStringAttr(attrs, FieldSchemaName, schemaName)
	attrs = appendTrimmedStringAttr(attrs, FieldWorkflowName, workflowName)
	return attrs
}

func PipelineAttrs(tenantID, projectID, schemaName, pipelineName string) []slog.Attr {
	attrs := TenantProjectAttrs(tenantID, projectID)
	attrs = appendTrimmedStringAttr(attrs, FieldSchemaName, schemaName)
	attrs = appendTrimmedStringAttr(attrs, FieldPipelineName, pipelineName)
	return attrs
}

func OperationAttrs(operation, status string, duration time.Duration) []slog.Attr {
	attrs := make([]slog.Attr, 0, 3)
	attrs = appendTrimmedStringAttr(attrs, FieldOperation, operation)
	attrs = appendTrimmedStringAttr(attrs, FieldStatus, status)
	attrs = append(attrs, slog.Float64(FieldDurationMS, float64(duration.Microseconds())/1000))
	return attrs
}

// RequestAttrs extracts stable request context fields. Do not add request bodies,
// tokens, emails, passwords, API keys, or exact cache keys here.
func RequestAttrs(c *fiber.Ctx) []slog.Attr {
	if c == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, 7)
	attrs = appendTrimmedStringAttr(attrs, FieldRequestID, RequestID(c))
	attrs = appendTrimmedStringAttr(attrs, FieldTenantID, localString(c, "tenantID"))
	attrs = appendTrimmedStringAttr(attrs, FieldProjectID, localString(c, "projectID"))
	attrs = appendTrimmedStringAttr(attrs, FieldSchemaName, firstNonEmpty(localString(c, "schemaName"), c.Query("schemaName")))
	attrs = appendTrimmedStringAttr(attrs, FieldWorkflowName, firstNonEmpty(localString(c, "workflowName"), c.Params("workflowName"), c.Query("workflowName")))
	attrs = appendTrimmedStringAttr(attrs, FieldPipelineName, firstNonEmpty(localString(c, "pipelineName"), c.Query("pipelineName")))
	attrs = appendTrimmedStringAttr(attrs, FieldUserID, firstNonEmpty(localString(c, "userID"), localString(c, "tenantUserID")))
	attrs = appendTraceAttrs(attrs, c.UserContext())

	return attrs
}

func appendTraceAttrs(attrs []slog.Attr, ctx context.Context) []slog.Attr {
	if ctx == nil {
		return attrs
	}
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return attrs
	}
	attrs = append(attrs, slog.String(FieldTraceID, spanContext.TraceID().String()))
	attrs = append(attrs, slog.String(FieldSpanID, spanContext.SpanID().String()))
	return attrs
}

func RequestID(c *fiber.Ctx) string {
	if c == nil {
		return ""
	}
	return localString(c, LocalRequestID)
}

func DurationMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}

func localString(c *fiber.Ctx, key string) string {
	value, _ := c.Locals(key).(string)
	return value
}

func appendTrimmedStringAttr(attrs []slog.Attr, key, value string) []slog.Attr {
	value = strings.TrimSpace(value)
	if value == "" {
		return attrs
	}
	return append(attrs, slog.String(key, strings.Clone(value)))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
