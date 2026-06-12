package observability

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials/insecure"
)

const instrumentationName = "github.com/osmansam/autotableGo"

var tracer = otel.Tracer(instrumentationName)

func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	if strings.EqualFold(os.Getenv("OTEL_TRACING_ENABLED"), "false") {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "autotable-go"
	}

	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			attribute.String("deployment.environment", firstNonEmpty(os.Getenv("NODE_ENV"), "local")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	tracer = provider.Tracer(instrumentationName)

	return provider.Shutdown, nil
}

func TracingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if shouldSkipTrace(c) {
			return c.Next()
		}

		baseCtx := c.UserContext()
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		baseCtx = otel.GetTextMapPropagator().Extract(baseCtx, fiberHeaderCarrier{c: c})

		method := strings.Clone(c.Method())
		path := strings.Clone(c.Path())
		ctx, span := tracer.Start(baseCtx, method+" "+path, trace.WithSpanKind(trace.SpanKindServer))
		c.SetUserContext(ctx)

		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		statusCode := fiberStatusCode(c, err)
		route := fiberRoute(c)

		span.SetName(method + " " + route)
		span.SetAttributes(
			attribute.String("http.request.method", method),
			attribute.String("url.path", path),
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", statusCode),
			attribute.Float64(FieldDurationMS, float64(duration.Microseconds())/1000),
		)
		span.SetAttributes(RequestTraceAttrs(c)...)

		if err != nil {
			RecordSpanError(span, err)
		} else if statusCode >= fiber.StatusInternalServerError {
			span.SetStatus(codes.Error, "http server error")
		}
		span.End()

		return err
	}
}

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func RecordSpanError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(attribute.String(FieldStatus, "error"))
}

func EndSpan(span trace.Span, status string, err error) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.String(FieldStatus, firstNonEmpty(status, "success")))
	if err != nil {
		RecordSpanError(span, err)
	}
	span.End()
}

func RequestTraceAttrs(c *fiber.Ctx) []attribute.KeyValue {
	if c == nil {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, 6)
	attrs = appendTraceStringAttr(attrs, FieldRequestID, RequestID(c))
	attrs = appendTraceStringAttr(attrs, FieldTenantID, localString(c, "tenantID"))
	attrs = appendTraceStringAttr(attrs, FieldProjectID, localString(c, "projectID"))
	attrs = appendTraceStringAttr(attrs, FieldSchemaName, firstNonEmpty(localString(c, "schemaName"), c.Query("schemaName")))
	attrs = appendTraceStringAttr(attrs, FieldWorkflowName, firstNonEmpty(localString(c, "workflowName"), c.Params("workflowName"), c.Query("workflowName")))
	attrs = appendTraceStringAttr(attrs, FieldPipelineName, firstNonEmpty(localString(c, "pipelineName"), c.Query("pipelineName")))
	return attrs
}

func WorkflowTraceAttrs(tenantID, projectID, schemaName, workflowName string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 4)
	attrs = appendTraceStringAttr(attrs, FieldTenantID, tenantID)
	attrs = appendTraceStringAttr(attrs, FieldProjectID, projectID)
	attrs = appendTraceStringAttr(attrs, FieldSchemaName, schemaName)
	attrs = appendTraceStringAttr(attrs, FieldWorkflowName, workflowName)
	return attrs
}

func OperationTraceAttrs(operation, status string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	attrs = appendTraceStringAttr(attrs, FieldOperation, operation)
	attrs = appendTraceStringAttr(attrs, FieldStatus, status)
	return attrs
}

func CacheTraceAttrs(operation, schemaName string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	attrs = appendTraceStringAttr(attrs, FieldOperation, operation)
	attrs = appendTraceStringAttr(attrs, FieldSchemaName, schemaName)
	return attrs
}

func MongoTraceAttrs(operation, schemaName string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = append(attrs, attribute.String("db.system", "mongodb"))
	attrs = appendTraceStringAttr(attrs, FieldOperation, operation)
	attrs = appendTraceStringAttr(attrs, FieldSchemaName, schemaName)
	return attrs
}

func appendTraceStringAttr(attrs []attribute.KeyValue, key, value string) []attribute.KeyValue {
	value = strings.TrimSpace(value)
	if value == "" {
		return attrs
	}
	return append(attrs, attribute.String(key, strings.Clone(value)))
}

type fiberHeaderCarrier struct {
	c *fiber.Ctx
}

func (h fiberHeaderCarrier) Get(key string) string {
	return h.c.Get(key)
}

func (h fiberHeaderCarrier) Set(key, value string) {
	h.c.Set(key, value)
}

func (h fiberHeaderCarrier) Keys() []string {
	keys := make([]string, 0)
	h.c.Request().Header.VisitAll(func(key, _ []byte) {
		keys = append(keys, string(key))
	})
	return keys
}

func shouldSkipTrace(c *fiber.Ctx) bool {
	switch c.Path() {
	case "/metrics", "/favicon.ico":
		return true
	default:
		return false
	}
}

func fiberRoute(c *fiber.Ctx) string {
	if c == nil || c.Route() == nil || c.Route().Path == "" {
		return "unknown"
	}
	return strings.Clone(c.Route().Path)
}

func fiberStatusCode(c *fiber.Ctx, err error) int {
	statusCode := c.Response().StatusCode()
	if err == nil {
		return statusCode
	}
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return fiberErr.Code
	}
	if statusCode >= fiber.StatusBadRequest {
		return statusCode
	}
	return fiber.StatusInternalServerError
}
