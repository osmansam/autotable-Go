package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/mongo"
	"testing"
	"time"
)

func TestCronHonorsExplicitTransaction(t *testing.T) {
	sentinel := errors.New("transaction unavailable")
	calls := 0
	service := &DynamicService{runTransaction: func(context.Context, func(mongo.SessionContext) error) error { calls++; return sentinel }}
	workflow := models.DynamicWorkflow{Name: "scheduled", Trigger: models.WorkflowTriggerCron, Schedule: "* * * * *", Mode: models.WorkflowModeTransactional, RunInTransaction: true, IsActive: true, Steps: []models.DynamicWorkflowStep{{Name: "result", Type: models.WorkflowStepTypeReturn, IsActive: true, Config: map[string]interface{}{"value": "done"}}}}
	err := service.RunCronWorkflow(context.Background(), "t", "p", "records", &models.ContainerModel{SchemaName: "records"}, workflow, time.Now())
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("transaction calls=%d error=%v", calls, err)
	}
}
