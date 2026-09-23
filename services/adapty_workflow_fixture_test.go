package services

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/osmansam/autotableGo/models"
)

func TestAdaptyWebhookWorkflowFixtureIsValid(t *testing.T) {
	data, err := os.ReadFile("../docs/examples/miywo/adapty-webhook.workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	var workflow models.DynamicWorkflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("decode workflow fixture: %v", err)
	}
	if workflow.Name != "adapty" || workflow.Trigger != models.WorkflowTriggerManual {
		t.Fatalf("unexpected identity: name=%q trigger=%q", workflow.Name, workflow.Trigger)
	}
	if workflow.IsAuthenticated || workflow.IsAuthorized {
		t.Fatal("external integration, rather than a user session, must authorize this workflow")
	}
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("invalid workflow fixture: %v", err)
	}
}
