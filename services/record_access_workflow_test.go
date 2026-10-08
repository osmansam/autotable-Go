package services

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func workflowAccessFixture() (*models.ContainerModel, primitive.ObjectID) {
	userID := primitive.NewObjectID()
	container := accessTestContainer()
	caller := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"}}}
	system := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Context: "{{auth.identity.kind}}", Operator: "eq", Value: "system"}}}
	container.Routes.CreateDynamicModelItem.Access = caller
	container.Routes.UpdateDynamicModelItem.Access = caller
	container.Routes.DeleteDynamicModelItem.Access = caller
	container.Routes.GetAllDynamicModelItems.Access = caller
	container.Routes.GetDynamicModelItem.Access = system
	return container, userID
}

func TestWorkflowRecordAccessRouteMapping(t *testing.T) {
	container, _ := workflowAccessFixture()

	for _, operation := range []workflowRecordOperation{workflowRecordCreate, workflowRecordUpdate, workflowRecordDelete, workflowRecordRead} {
		route := workflowRecordAccessRoute(container, operation)
		if route.Access == nil {
			t.Fatalf("workflowRecordAccessRoute(%q) has no access policy", operation)
		}
	}
	if got := workflowRecordAccessRoute(container, workflowRecordRead); got.Access != container.Routes.GetAllDynamicModelItems.Access {
		t.Fatal("workflow reads must use GetAllDynamicModelItems access, not get-one access")
	}
}

func TestWorkflowReadRecordAccessUsesCallerAndSystemIdentity(t *testing.T) {
	container, userID := workflowAccessFixture()
	service := &DynamicService{loadAccessUser: func(context.Context, string, string, string) (map[string]interface{}, error) {
		return map[string]interface{}{"_id": userID}, nil
	}}

	callerPayload := workflowExecutionPayload{IdentityKind: AccessIdentityCaller, UserID: userID.Hex(), UserRole: "member"}
	got, err := service.workflowReadRecordAccessFilter(context.Background(), container, callerPayload, "tenant", "project", "orders", bson.M{"state": "open"})
	if err != nil {
		t.Fatalf("caller workflowReadRecordAccessFilter() error = %v", err)
	}
	want := bson.M{"$and": []bson.M{{"state": "open"}, {"ownerId": userID}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("caller workflow filter = %#v, want %#v", got, want)
	}

	container.Routes.GetAllDynamicModelItems.Access = &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Context: "{{auth.identity.kind}}", Operator: "eq", Value: "system"}}}
	systemPayload := workflowExecutionPayload{IdentityKind: AccessIdentitySystem}
	got, err = service.workflowReadRecordAccessFilter(context.Background(), container, systemPayload, "tenant", "project", "orders", bson.M{"state": "open"})
	if err != nil {
		t.Fatalf("system workflowReadRecordAccessFilter() error = %v", err)
	}
	if !reflect.DeepEqual(got, bson.M{"state": "open"}) {
		t.Fatalf("system workflow filter = %#v, want base filter", got)
	}
}

func TestWorkflowCreateRecordAccessAppliesTrustedAssignmentAndDeniesSystem(t *testing.T) {
	container, userID := workflowAccessFixture()
	container.Routes.CreateDynamicModelItem.Access.Assign = map[string]interface{}{"ownerId": "{{auth.user.id}}"}
	callerCtx := callerAccessContext(userID.Hex())
	document, err := applyCreateRecordAccessAssignments(container, container.Routes.CreateDynamicModelItem.Access, callerCtx, map[string]interface{}{"ownerId": primitive.NewObjectID()})
	if err != nil {
		t.Fatalf("applyCreateRecordAccessAssignments() error = %v", err)
	}
	if document["ownerId"] != userID {
		t.Fatalf("assigned ownerId = %#v, want caller id", document["ownerId"])
	}
	if err := authorizePreparedCreateRecords(container, container.Routes.CreateDynamicModelItem.Access, callerCtx, document); err != nil {
		t.Fatalf("caller create authorization error = %v", err)
	}

	systemCtx := recordAccessContext{IdentityKind: accessIdentitySystem}
	if err := authorizePreparedCreateRecords(container, container.Routes.CreateDynamicModelItem.Access, systemCtx, map[string]interface{}{"ownerId": userID}); err == nil {
		t.Fatal("system identity must be denied when no explicit system rule exists")
	}
}

func TestWorkflowMutationSelectorAndFinalDocumentAuthorization(t *testing.T) {
	container, userID := workflowAccessFixture()
	payload := workflowExecutionPayload{IdentityKind: AccessIdentityCaller, UserID: userID.Hex(), UserRole: "member"}
	ctx := callerAccessContext(userID.Hex())
	selector, err := workflowMutationRecordAccessSelector(container, workflowRecordUpdate, payload, ctx, bson.M{"state": "open"})
	if err != nil {
		t.Fatalf("workflowMutationRecordAccessSelector() error = %v", err)
	}
	want := bson.M{"$and": []bson.M{{"state": "open"}, {"ownerId": userID}}}
	if !reflect.DeepEqual(selector, want) {
		t.Fatalf("workflow mutation selector = %#v, want %#v", selector, want)
	}
	if err := authorizeFinalUpdateRecord(container, container.Routes.UpdateDynamicModelItem.Access, ctx, map[string]interface{}{"ownerId": primitive.NewObjectID()}); err == nil {
		t.Fatal("workflow update must reject a final document outside the policy")
	}
}

func TestWorkflowRecordAccessRejectsOwnershipTransferInUpdateDocument(t *testing.T) {
	container, userID := workflowAccessFixture()
	ctx := callerAccessContext(userID.Hex())
	document := map[string]interface{}{"ownerId": userID, "status": "open"}
	update := bson.M{"$set": bson.M{"ownerId": primitive.NewObjectID()}}

	if err := applyWorkflowUpdateForAccess(document, update); err != nil {
		t.Fatalf("applyWorkflowUpdateForAccess() error = %v", err)
	}
	if err := authorizeFinalUpdateRecord(container, container.Routes.UpdateDynamicModelItem.Access, ctx, document); err == nil {
		t.Fatal("workflow update must reject ownership transfer after applying Mongo operators")
	}
}

func TestCronRecordAccessIdentityIsExplicitSystem(t *testing.T) {
	payload := newCronWorkflowPayload("tenant", "project", "orders", nil, models.DynamicWorkflow{}, time.Now())
	if payload.IdentityKind != AccessIdentitySystem || payload.UserID != "" || len(payload.UserRoles) != 0 {
		t.Fatalf("cron identity = %#v, want explicit system without invented user", payload)
	}
}

func TestWorkflowRecordAccessStepsUseSharedEnforcementHelpers(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "dynamic_workflow.go", nil, 0)
	if err != nil {
		t.Fatalf("parse dynamic_workflow.go: %v", err)
	}
	wantCalls := map[string][]string{
		"workflowCreateRecord": {"workflowMutationRecordAccessContext", "authorizePreparedCreateRecords"},
		"workflowUpdateRecord": {"workflowUpdateRecords"},
		"workflowUnsetRecord":  {"workflowUpdateRecord"},
		"workflowDeleteRecord": {"workflowMutationRecordAccessContext", "workflowMutationRecordAccessSelector"},
		"workflowGetRecord":    {"workflowReadRecordAccessFilter"},
		"workflowFindRecords":  {"workflowReadRecordAccessFilter"},
		"workflowCountRecords": {"workflowReadRecordAccessFilter"},
		"workflowDistinct":     {"workflowReadRecordAccessFilter"},
		"workflowUpdateArray":  {"workflowUpdateRecords"},
	}

	foundFunctions := map[string]bool{}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		required, tracked := wantCalls[function.Name.Name]
		if !tracked {
			continue
		}
		foundFunctions[function.Name.Name] = true
		calls := map[string]bool{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				calls[target.Name] = true
			case *ast.SelectorExpr:
				calls[target.Sel.Name] = true
			}
			return true
		})
		for _, requiredCall := range required {
			if !calls[requiredCall] {
				t.Errorf("%s must call %s", function.Name.Name, requiredCall)
			}
		}
	}
	for functionName := range wantCalls {
		if !foundFunctions[functionName] {
			t.Errorf("workflow function %s was not found", functionName)
		}
	}
}
