package services

import (
	"crypto/sha256"
	"net/url"
	"strconv"
	"strings"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func workflowJoinIdempotencyParts(parts ...string) string {
	escaped := make([]string, len(parts))
	for index, part := range parts {
		escaped[index] = url.QueryEscape(part)
	}
	return strings.Join(escaped, ":")
}

// A captured group shares workflow state, while each nested operation owns a
// separate replay identity. Position also disambiguates unnamed/repeated steps.
func workflowScopedStepIdempotencyKey(parent, workflowName string, step models.DynamicWorkflowStep, position int) string {
	if parent == "" {
		return ""
	}
	return parent + ":step:" + strconv.Itoa(position) + ":" + url.QueryEscape(workflowName) + ":" + url.QueryEscape(workflowStepIdentifier(step))
}

func workflowLoopIdempotencyKey(parent string, index int) string {
	if parent == "" {
		return ""
	}
	return parent + ":loop:" + strconv.Itoa(index)
}

func workflowAuditEventID(payload workflowExecutionPayload) primitive.ObjectID {
	if payload.OutboxEventID.IsZero() || payload.IdempotencyKey == "" {
		return payload.OutboxEventID
	}
	digest := sha256.Sum256([]byte("workflow-audit:" + payload.IdempotencyKey))
	var id primitive.ObjectID
	copy(id[:], digest[:12])
	return id
}

func workflowBranchIdempotencyKey(parent string, index int, name string) string {
	if parent == "" {
		return ""
	}
	return parent + ":branch:" + strconv.Itoa(index) + ":" + url.QueryEscape(name)
}
