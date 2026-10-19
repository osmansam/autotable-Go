package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/textproto"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// Provider idempotency is opt-in because remote APIs define their own header
// and deduplication contract. The key is stable for retries of one scoped step.
func workflowProviderIdempotencyHeader(config map[string]interface{}) (string, error) {
	configured, exists := config["idempotencyHeader"]
	if !exists {
		return "", nil
	}
	name, ok := configured.(string)
	if !ok {
		return "", fmt.Errorf("call_api idempotencyHeader must be a header name")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	if len(name) > 128 || !httpguts.ValidHeaderFieldName(name) {
		return "", fmt.Errorf("call_api idempotencyHeader must be a valid header name")
	}
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "host", "cookie", "set-cookie", "content-type", "content-length", "content-encoding", "connection", "transfer-encoding", "upgrade", "trailer", "te", "keep-alive":
		return "", fmt.Errorf("call_api idempotencyHeader cannot replace authentication or transport headers")
	}
	return textproto.CanonicalMIMEHeaderKey(name), nil
}

func workflowProviderIdempotencyHeaders(config map[string]interface{}, payload workflowExecutionPayload, protected map[string]string) (map[string]string, error) {
	name, err := workflowProviderIdempotencyHeader(config)
	if err != nil || name == "" {
		return protected, err
	}
	if payload.OutboxEventID.IsZero() || payload.IdempotencyKey == "" {
		return nil, fmt.Errorf("call_api idempotencyHeader requires a durable outbox execution identity")
	}
	for existing := range protected {
		if strings.EqualFold(existing, name) {
			return nil, fmt.Errorf("call_api idempotencyHeader conflicts with a protected credential header")
		}
	}
	result := make(map[string]string, len(protected)+1)
	for key, value := range protected {
		result[key] = value
	}
	// Preserve boundaries even for old colon-delimited stored keys. Encoding a
	// tuple prevents one tenant/project/workflow scope from resembling another.
	scope, _ := json.Marshal([]string{"workflow-http:v1", payload.TenantID, payload.ProjectID, payload.SchemaName, payload.WorkflowName, payload.IdempotencyKey})
	digest := sha256.Sum256(scope)
	result[name] = hex.EncodeToString(digest[:])
	return result, nil
}
