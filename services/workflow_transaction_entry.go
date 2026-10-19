package services

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/mongo"
)

// Scheduled and manual entry points share the transaction decision. Nested
// execution reuses an enclosing session instead of starting another transaction.
func (s *DynamicService) runWorkflowWithTransaction(ctx context.Context, payload *workflowExecutionPayload, workflow models.DynamicWorkflow) error {
	if !workflowRequiresTransaction(workflow) || mongo.SessionFromContext(ctx) != nil {
		return s.runWorkflowDefinition(ctx, payload, workflow)
	}
	run := s.runTransaction
	if run == nil {
		run = s.repository.WithTransaction
	}
	return run(ctx, func(tx mongo.SessionContext) error { return s.runWorkflowDefinition(tx, payload, workflow) })
}
