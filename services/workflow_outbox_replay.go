package services

import "github.com/osmansam/autotableGo/models"

func workflowStepFromOutboxEvent(event *models.DynamicOutboxEvent) models.DynamicWorkflowStep {
	step := cloneWorkflowSnapshot(models.DynamicWorkflowStep{
		ID: event.Payload.StepID, Name: event.Payload.StepName, Type: event.Payload.StepType, IsActive: true,
		TargetSchema: event.Payload.TargetSchema, Config: event.Payload.Config, Steps: event.Payload.Steps,
		Conditions: event.Payload.Conditions, ElseSteps: event.Payload.ElseSteps, Branches: event.Payload.Branches, TimeoutSec: event.Payload.StepTimeoutSec,
	})
	normalizeCapturedWorkflowStepModes(&step)
	return step
}

// Captured children belong to this claimed event. Replaying them locally avoids
// dropping explicitly outbox-mode children or enqueueing the same tree forever.
func normalizeCapturedWorkflowStepModes(step *models.DynamicWorkflowStep) {
	step.ExecutionMode = models.WorkflowModeTransactional
	for index := range step.Steps {
		normalizeCapturedWorkflowStepModes(&step.Steps[index])
	}
	for index := range step.ElseSteps {
		normalizeCapturedWorkflowStepModes(&step.ElseSteps[index])
	}
	for branch := range step.Branches {
		for index := range step.Branches[branch].Steps {
			normalizeCapturedWorkflowStepModes(&step.Branches[branch].Steps[index])
		}
	}
}

// Captured control-flow groups can commit their database mutations and the
// completion marker together, provided retrying the group has no external effect.
func workflowReplayRequiresTransaction(step models.DynamicWorkflowStep) bool {
	if workflowStepWritesTransactionally(step.Type) {
		return true
	}
	if step.Type != models.WorkflowStepTypeIf && step.Type != models.WorkflowStepTypeForEach {
		return false
	}
	return workflowStepsRequireTransaction([]models.DynamicWorkflowStep{step}, models.WorkflowModeTransactional) &&
		!workflowReplayTreeHasExternalEffects(step)
}

func workflowReplayTreeHasExternalEffects(step models.DynamicWorkflowStep) bool {
	if !step.IsActive {
		return false
	}
	// Dynamically loaded code and workflows are not a captured subtree: their
	// effects cannot be classified from this event's snapshot.
	if workflowStepHasNonTransactionalSideEffect(step) || step.Type == models.WorkflowStepTypeDynamicFunction || step.Type == models.WorkflowStepTypeExecuteWorkflow {
		return true
	}
	for _, children := range [][]models.DynamicWorkflowStep{step.Steps, step.ElseSteps} {
		for _, child := range children {
			if workflowReplayTreeHasExternalEffects(child) {
				return true
			}
		}
	}
	for _, branch := range step.Branches {
		for _, child := range branch.Steps {
			if workflowReplayTreeHasExternalEffects(child) {
				return true
			}
		}
	}
	return false
}
