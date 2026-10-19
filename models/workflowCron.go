package models

import "time"

// One durable scheduling cursor per workflow scope. Never expires with outbox TTL.
type WorkflowCronSchedule struct {
	ID            string             `bson:"_id"`
	Revision      string             `bson:"revision"`
	NextAt        time.Time          `bson:"nextAt"`
	SkippedBefore time.Time          `bson:"skippedBefore,omitempty"`
	UpdatedAt     time.Time          `bson:"updatedAt"`
	Source        WorkflowCronSource `bson:"source"`
}

type WorkflowCronSource struct {
	TenantID     string            `bson:"tenantId"`
	ProjectID    string            `bson:"projectId"`
	Schema       string            `bson:"schemaName"`
	WorkflowName string            `bson:"workflowName"`
	Workflows    []DynamicWorkflow `bson:"workflows"`
}
