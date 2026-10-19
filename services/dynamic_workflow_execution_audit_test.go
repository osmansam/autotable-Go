package services

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func auditVariableStep(name string, order int, values map[string]interface{}) models.DynamicWorkflowStep {
	return models.DynamicWorkflowStep{Name: name, Order: order, Type: models.WorkflowStepTypeSetVariable, IsActive: true, Config: map[string]interface{}{"values": values}}
}

func TestWorkflowExecutionHonorsCanceledContext(t *testing.T) {
	for _, expired := range []bool{false, true} {
		for _, boundary := range []string{"definition", "step list", "single step", "loop"} {
			t.Run(boundary+map[bool]string{false: "/canceled", true: "/deadline"}[expired], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				wantErr := context.Canceled
				if expired {
					ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
					defer cancel()
					wantErr = context.DeadlineExceeded
				}
				payload := workflowExecutionPayload{Variables: map[string]interface{}{}}
				step := auditVariableStep("mustNotRun", 0, map[string]interface{}{"changed": true})
				service := &DynamicService{}
				var err error
				switch boundary {
				case "definition":
					err = service.runWorkflowDefinition(ctx, &payload, models.DynamicWorkflow{Name: "canceled", Steps: []models.DynamicWorkflowStep{step}})
				case "step list":
					err = service.runWorkflowStepList(ctx, &payload, "canceled", "", models.WorkflowModeTransactional, false, []models.DynamicWorkflowStep{step})
				case "single step":
					_, err = service.processWorkflowStepWithTimeout(ctx, step, &payload)
				case "loop":
					_, err = service.workflowForEach(ctx, models.DynamicWorkflowStep{Config: map[string]interface{}{"items": []interface{}{1, 2}}}, &payload)
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("error = %v, want %v", err, wantErr)
				}
				if payload.Variables["changed"] != nil {
					t.Fatal("canceled workflow mutated variables")
				}
			})
		}
	}
}

func TestWorkflowStepOutputsAreIndependentSnapshots(t *testing.T) {
	payload := workflowExecutionPayload{Record: map[string]interface{}{"customer": map[string]interface{}{"name": "Ada"}}}
	service := &DynamicService{}
	steps := []models.DynamicWorkflowStep{
		{Name: "before", Type: models.WorkflowStepTypeSetRecord, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"state": "new"}}},
		{Name: "after", Order: 1, Type: models.WorkflowStepTypeSetRecord, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"customer.name": "Grace"}}},
	}
	if err := service.runWorkflowStepList(context.Background(), &payload, "snapshots", "", models.WorkflowModeTransactional, true, steps); err != nil {
		t.Fatal(err)
	}
	before, _ := workflowPathValue(payload.StepOutputs, "before.customer.name")
	after, _ := workflowPathValue(payload.StepOutputs, "after.customer.name")
	if before != "Ada" || after != "Grace" {
		t.Fatalf("step snapshots = (%v, %v), want (Ada, Grace)", before, after)
	}
}

func TestWorkflowNestedExecutionPreservesParentRecord(t *testing.T) {
	child := models.DynamicWorkflow{Name: "child", IsActive: true, Steps: []models.DynamicWorkflowStep{{
		Name: "change", Type: models.WorkflowStepTypeSetRecord, IsActive: true,
		Config: map[string]interface{}{"values": map[string]interface{}{"customer.name": "Grace"}},
	}}}
	parent := workflowExecutionPayload{SchemaName: "orders", Container: &models.ContainerModel{SchemaName: "orders", Workflows: []models.DynamicWorkflow{child}}, Record: map[string]interface{}{"customer": map[string]interface{}{"name": "Ada"}}}
	if _, err := (&DynamicService{}).workflowExecuteWorkflow(context.Background(), models.DynamicWorkflowStep{Config: map[string]interface{}{"workflowName": "child"}}, parent); err != nil {
		t.Fatal(err)
	}
	if name, _ := workflowPathValue(parent.Record, "customer.name"); name != "Ada" {
		t.Fatalf("child workflow changed parent record: %#v", parent.Record)
	}
}

func TestExecuteWorkflowDoesNotMutateCallerNestedInput(t *testing.T) {
	record := map[string]interface{}{"customer": map[string]interface{}{"name": "Ada"}}
	container := &models.ContainerModel{Workflows: []models.DynamicWorkflow{{Name: "change", IsActive: true, Steps: []models.DynamicWorkflowStep{{
		Type: models.WorkflowStepTypeSetRecord, IsActive: true, Config: map[string]interface{}{"values": map[string]interface{}{"customer.name": "Grace"}},
	}}}}}
	if _, err := (&DynamicService{}).ExecuteWorkflow(context.Background(), ExecuteWorkflowInput{Container: container, WorkflowName: "change", Record: record}); err != nil {
		t.Fatal(err)
	}
	if name, _ := workflowPathValue(record, "customer.name"); name != "Ada" {
		t.Fatalf("ExecuteWorkflow mutated caller input: %#v", record)
	}
}

func TestWorkflowCloneIsolatesTypedMongoValues(t *testing.T) {
	id := primitive.NewObjectID()
	source := map[string]interface{}{
		"id":      id,
		"maps":    []bson.M{{"nested": bson.M{"value": "original"}}},
		"array":   primitive.A{map[string]interface{}{"value": "original"}},
		"strings": []string{"original"},
		"binary":  primitive.Binary{Subtype: 0x80, Data: []byte{1, 2}},
	}
	copy := cloneWorkflowMap(source)
	copy["maps"].([]bson.M)[0]["nested"].(bson.M)["value"] = "changed"
	copy["array"].(primitive.A)[0].(map[string]interface{})["value"] = "changed"
	copy["strings"].([]string)[0] = "changed"
	copy["binary"].(primitive.Binary).Data[0] = 9
	if !reflect.DeepEqual(source["maps"], []bson.M{{"nested": bson.M{"value": "original"}}}) ||
		!reflect.DeepEqual(source["array"], primitive.A{map[string]interface{}{"value": "original"}}) ||
		!reflect.DeepEqual(source["strings"], []string{"original"}) || source["binary"].(primitive.Binary).Data[0] != 1 || copy["id"] != id {
		t.Fatalf("clone changed source or BSON scalar: %#v", source)
	}
}

func TestWorkflowTemplatesTreatResolvedTextAsLiteral(t *testing.T) {
	payload := workflowExecutionPayload{Record: map[string]interface{}{"note": "{{vars.secret}}", "second": "kept"}, Variables: map[string]interface{}{"secret": "private"}}
	for _, value := range []string{"prefix {{record.note}}", "{{record.note}} / {{record.second}}"} {
		want := "prefix {{vars.secret}}"
		if value[0] == '{' {
			want = "{{vars.secret}} / kept"
		}
		if got := resolveWorkflowTemplateString(value, payload); got != want {
			t.Errorf("resolve(%q) = %#v, want %q", value, got, want)
		}
	}
}

func TestWorkflowTemplatesResolveBSONAndTypedArrays(t *testing.T) {
	payload := workflowExecutionPayload{Record: map[string]interface{}{"name": "Ada"}}
	for _, input := range []interface{}{
		primitive.A{bson.M{"name": "{{record.name}}"}},
		[]bson.M{{"name": "{{record.name}}"}},
		[]map[string]interface{}{{"name": "{{record.name}}"}},
		[]string{"{{record.name}}"},
	} {
		got := resolveWorkflowTemplates(input, payload)
		items, ok := workflowInterfaceSlice(got)
		if !ok || len(items) != 1 {
			t.Fatalf("resolve(%T) = %#v", input, got)
		}
		if _, isString := input.([]string); isString {
			if items[0] != "Ada" {
				t.Errorf("resolve(%T) = %#v, want [Ada]", input, got)
			}
		} else if name, ok := workflowPathValue(items[0], "name"); !ok || name != "Ada" {
			t.Errorf("resolve(%T) = %#v, want name Ada", input, got)
		}
	}
}

func TestWorkflowPathValueReadsBSONArrays(t *testing.T) {
	value := bson.M{"items": primitive.A{bson.M{"name": "Ada"}}, "tags": []string{"first", "second"}}
	for path, want := range map[string]interface{}{"items.0.name": "Ada", "tags.1": "second"} {
		if got, ok := workflowPathValue(value, path); !ok || got != want {
			t.Errorf("path %s = %#v, %v, want %#v", path, got, ok, want)
		}
	}
}

func TestWorkflowDefaultExpressionAcceptsContainers(t *testing.T) {
	payload := workflowExecutionPayload{Record: map[string]interface{}{"items": []interface{}{1}, "object": map[string]interface{}{"key": "value"}}}
	for _, field := range []string{"items", "object"} {
		got := resolveWorkflowTemplateString("{{default(record."+field+", 'fallback')}}", payload)
		if !reflect.DeepEqual(got, payload.Record[field]) {
			t.Errorf("default(%s) = %#v, want %#v", field, got, payload.Record[field])
		}
	}
}

func TestWorkflowQueryReferencesWorkInConditionsAndDefaults(t *testing.T) {
	payload := workflowExecutionPayload{Query: map[string]interface{}{"limit": 12}}
	for _, condition := range []models.WorkflowCondition{{Field: "query.limit", Operator: "=", Value: 12}, {Field: "query.limit", Operator: "<", Value: 15}} {
		if !workflowConditionMatches(condition, payload) {
			t.Errorf("condition did not read request query: %#v", condition)
		}
	}
	if got := resolveWorkflowTemplateString("{{default(query.missing, 5)}}", payload); got != int64(5) {
		t.Fatalf("default for missing query = %#v, want 5", got)
	}
}

func TestWorkflowConditionsDoNotOrderMissingFields(t *testing.T) {
	for _, operator := range []string{">", ">=", "<", "<=", "between", "starts_with", "ends_with"} {
		t.Run(operator, func(t *testing.T) {
			var expected interface{} = 10
			if operator == "between" {
				expected = []interface{}{"", "zzz"}
			}
			if operator == "starts_with" || operator == "ends_with" {
				expected = ""
			}
			if workflowConditionMatches(models.WorkflowCondition{Field: "missing", Operator: operator, Value: expected}, workflowExecutionPayload{}) {
				t.Fatal("missing field satisfied an ordering/string condition")
			}
		})
	}
}

func TestWorkflowChangedConditionsDetectAddedAndRemovedFields(t *testing.T) {
	for _, test := range []struct {
		name              string
		current, previous map[string]interface{}
		operator          string
		expected          interface{}
		want              bool
	}{
		{name: "added", current: map[string]interface{}{"state": "open"}, operator: "changed", want: true},
		{name: "added to", current: map[string]interface{}{"state": "open"}, operator: "changed_to", expected: "open", want: true},
		{name: "removed", previous: map[string]interface{}{"state": "open"}, operator: "changed", want: true},
		{name: "removed from", previous: map[string]interface{}{"state": "open"}, operator: "changed_from", expected: "open", want: true},
		{name: "absent both", operator: "changed"},
		{name: "null added", current: map[string]interface{}{"state": nil}, operator: "changed", want: true},
		{name: "unchanged", current: map[string]interface{}{"state": "open"}, previous: map[string]interface{}{"state": "open"}, operator: "changed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			condition := models.WorkflowCondition{Field: "record.state", Operator: test.operator, Value: test.expected}
			if got := workflowConditionMatches(condition, workflowExecutionPayload{Record: test.current, OldRecord: test.previous}); got != test.want {
				t.Fatalf("condition = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWorkflowResolvedObjectsDetachFromSourceState(t *testing.T) {
	for _, namespace := range []string{"record", "vars", "user"} {
		t.Run(namespace, func(t *testing.T) {
			source := map[string]interface{}{"value": "original"}
			payload := workflowExecutionPayload{Record: map[string]interface{}{"object": source}, Variables: map[string]interface{}{"object": source}, CurrentUser: map[string]interface{}{"object": source}}
			resolved := resolveWorkflowTemplates(map[string]interface{}{"child": "{{" + namespace + ".object}}"}, payload).(map[string]interface{})
			resolved["child"].(map[string]interface{})["value"] = "changed"
			if source["value"] != "original" {
				t.Fatal("resolved object aliases its source; nested assignments could create cycles or mutate trusted user state")
			}
		})
	}
}

func TestWorkflowTemplateResolutionPreservesBinaryScalars(t *testing.T) {
	for _, input := range []interface{}{[]byte{1, 2}, json.RawMessage(`{"value":1}`), bson.Raw{5, 0, 0, 0, 0}} {
		if got := resolveWorkflowTemplates(input, workflowExecutionPayload{}); !reflect.DeepEqual(got, input) {
			t.Errorf("resolve(%T) = %#v, want scalar %#v", input, got, input)
		}
	}
}

func TestWorkflowForEachAcceptsTypedRecordArrays(t *testing.T) {
	for _, items := range []interface{}{[]string{"a", "b"}, []int64{1, 2}, []bson.M{{"id": 1}, {"id": 2}}} {
		payload := workflowExecutionPayload{Record: map[string]interface{}{"items": items}}
		got, err := (&DynamicService{}).workflowForEach(context.Background(), models.DynamicWorkflowStep{Config: map[string]interface{}{"items": "{{record.items}}"}}, &payload)
		if err != nil {
			t.Errorf("for_each(%T): %v", items, err)
			continue
		}
		if got.(map[string]interface{})["count"] != 2 {
			t.Errorf("for_each(%T) = %#v, want 2 items", items, got)
		}
	}
}

func TestWorkflowAggregateCapsResultsAfterExpandingStages(t *testing.T) {
	input := []interface{}{map[string]interface{}{"$limit": 2}, map[string]interface{}{"$unwind": "$items"}}
	got, err := workflowAggregatePipelineWithLimit(input)
	if err != nil {
		t.Fatal(err)
	}
	last, ok := workflowPipelineStage(got[len(got)-1])
	if !ok || last["$limit"] != maxWorkflowAggregateLimit {
		t.Fatalf("aggregate can exceed result cap after unwind: %#v", got)
	}
	if len(input) != 2 {
		t.Fatal("pipeline input was mutated")
	}
}

func TestWorkflowAssigningAnAncestorDoesNotCreateCycles(t *testing.T) {
	payload := workflowExecutionPayload{Record: map[string]interface{}{"object": map[string]interface{}{"value": "before"}}}
	got, err := workflowSetRecord(models.DynamicWorkflowStep{Config: map[string]interface{}{"values": map[string]interface{}{"object.child": "{{record.object}}"}}}, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("ancestor assignment did not produce finite JSON: %v", err)
	}
	if value, ok := workflowPathValue(got, "object.child.value"); !ok || value != "before" {
		t.Fatalf("copied ancestor value = %#v, %v", value, ok)
	}
}

func TestWorkflowStepOrderingAndSkipping(t *testing.T) {
	steps := []models.DynamicWorkflowStep{
		auditVariableStep("last", 10, map[string]interface{}{"value": "last"}),
		auditVariableStep("first", -1, map[string]interface{}{"value": "first"}),
		auditVariableStep("sameOrder", -1, map[string]interface{}{"value": "second"}),
		auditVariableStep("inactive", 11, map[string]interface{}{"value": "inactive"}),
		auditVariableStep("wrongMode", 12, map[string]interface{}{"value": "outbox"}),
		auditVariableStep("unmatched", 13, map[string]interface{}{"value": "unmatched"}),
	}
	steps[3].IsActive = false
	steps[4].ExecutionMode = models.WorkflowModeOutbox
	steps[5].Conditions = []models.WorkflowCondition{{Field: "enabled", Value: true}}
	payload := workflowExecutionPayload{}
	if err := (&DynamicService{}).runWorkflowSteps(context.Background(), &payload, models.DynamicWorkflow{Name: "order", Steps: steps}, models.WorkflowModeTransactional); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"first": "first", "sameOrder": "second", "last": "last"} {
		if got, _ := workflowPathValue(payload.StepOutputs, name+".value"); got != want {
			t.Errorf("output %s = %#v, want %s", name, got, want)
		}
	}
	if payload.Variables["value"] != "last" || len(payload.StepOutputs) != 3 || steps[0].Name != "last" {
		t.Fatalf("ordering/skipping changed definitions or ran excluded steps: %#v", payload)
	}
}

func TestWorkflowErrorContinuationPolicy(t *testing.T) {
	for _, stop := range []bool{false, true} {
		for _, allow := range []bool{false, true} {
			t.Run(map[bool]string{false: "continue", true: "stop"}[stop]+map[bool]string{false: "/normal", true: "/optional"}[allow], func(t *testing.T) {
				payload := workflowExecutionPayload{}
				err := (&DynamicService{}).runWorkflowStepList(context.Background(), &payload, "errors", "", models.WorkflowModeTransactional, stop, []models.DynamicWorkflowStep{
					{Name: "denied", Type: models.WorkflowStepTypeFail, IsActive: true, ContinueOnError: allow, Config: map[string]interface{}{"message": "insufficient stock", "status": 409}},
					auditVariableStep("after", 1, map[string]interface{}{"ran": true}),
				})
				wantStop := stop && !allow
				if (err != nil) != wantStop || (payload.Variables["ran"] == true) == wantStop {
					t.Fatalf("error=%v variables=%#v; stop=%v allow=%v", err, payload.Variables, stop, allow)
				}
				if wantStop {
					var business *workflowBusinessError
					if !errors.As(err, &business) || business.Status != 409 || business.Message != "insufficient stock" {
						t.Fatalf("wrapped workflow business error lost status/message: %v", err)
					}
				}
				if _, exists := payload.StepOutputs["denied"]; exists {
					t.Fatal("failed step published a success output")
				}
			})
		}
	}
}

func TestWorkflowForEachReturnStopsParentAndPreservesOuterLoop(t *testing.T) {
	payload := workflowExecutionPayload{Record: map[string]interface{}{"items": []interface{}{"first", "second"}}, Loop: map[string]interface{}{"item": "outer", "index": 9}}
	steps := []models.DynamicWorkflowStep{
		{Name: "loop", Type: models.WorkflowStepTypeForEach, IsActive: true, Config: map[string]interface{}{"items": "{{record.items}}", "itemName": "entry"}, Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeReturn, IsActive: true, Config: map[string]interface{}{"value": "{{loop.entry}}"}}}},
		auditVariableStep("after", 1, map[string]interface{}{"ran": true}),
	}
	if err := (&DynamicService{}).runWorkflowStepList(context.Background(), &payload, "loop-return", "", models.WorkflowModeTransactional, true, steps); err != nil {
		t.Fatal(err)
	}
	if !payload.HasReturn || payload.ReturnValue != "first" || payload.Variables["ran"] != nil || !reflect.DeepEqual(payload.Loop, map[string]interface{}{"item": "outer", "index": 9}) {
		t.Fatalf("loop return escaped incorrectly: %#v", payload)
	}
}

func FuzzWorkflowTemplatesResolvedTextRemainsLiteral(f *testing.F) {
	for _, seed := range []string{"plain text", "{{record.note}}", "{{vars.secret}}", "{{", "}}", "a {{record.note}} b", "東京", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, note string) {
		payload := workflowExecutionPayload{Record: map[string]interface{}{"note": note}, Variables: map[string]interface{}{"secret": "private"}}
		got := resolveWorkflowTemplateString("prefix:{{record.note}}:suffix", payload)
		if got != "prefix:"+note+":suffix" {
			t.Fatalf("record text was interpreted as template syntax: %#v", got)
		}
	})
}
