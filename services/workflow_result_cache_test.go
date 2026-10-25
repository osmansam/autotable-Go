package services

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/mongo"
)

func workflowResultCacheFixture(t *testing.T) (*DynamicService, ExecuteWorkflowInput, *miniredis.Miniredis, *atomic.Int32) {
	t.Helper()
	server := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() {
		_ = configs.RedisClient.Close()
		configs.RedisClient = previous
		configs.RedisCircuitRecordSuccess()
	})
	var workflow models.DynamicWorkflow
	if err := json.Unmarshal([]byte(`{"name":"loadPublicPage","trigger":"manual","mode":"transactional","isActive":true,"isRedisCached":true,"cacheTime":1,"runInTransaction":true,"stopOnError":true,"steps":[{"name":"page","type":"return","isActive":true,"config":{"value":"{{record.result}}"}}]}`), &workflow); err != nil {
		t.Fatal(err)
	}
	container := &models.ContainerModel{SchemaName: "siteConfig", Workflows: []models.DynamicWorkflow{workflow}}
	service := NewDynamicService()
	calls := &atomic.Int32{}
	service.runTransaction = func(ctx context.Context, fn func(mongo.SessionContext) error) error {
		calls.Add(1)
		return fn(mongo.NewSessionContext(ctx, nil))
	}
	return service, ExecuteWorkflowInput{AccessIdentityInput: AccessIdentityInput{IdentityKind: AccessIdentityCaller}, TenantID: "tenant", ProjectID: "project", Schema: "siteConfig", WorkflowName: "loadPublicPage", Record: map[string]interface{}{"result": map[string]interface{}{"title": "Home", "number": int64(9007199254740993)}}, Container: container}, server, calls
}

func TestWorkflowResultCacheHitExpiryAndNumericPrecision(t *testing.T) {
	service, input, server, calls := workflowResultCacheFixture(t)
	first, err := service.ExecuteWorkflow(context.Background(), input)
	if err != nil || first.Source != "workflow" {
		t.Fatalf("initial execution = %#v, %v", first, err)
	}
	second, err := service.ExecuteWorkflow(context.Background(), input)
	if err != nil || second.Source != "cache" {
		t.Fatalf("repeat execution = %#v, %v; want cache hit", second, err)
	}
	a, _ := json.Marshal(first.Data)
	b, _ := json.Marshal(second.Data)
	if string(a) != string(b) {
		t.Fatalf("cached data changed: %s -> %s", a, b)
	}
	if calls.Load() != 1 {
		t.Fatalf("workflow executed %d times for identical requests", calls.Load())
	}
	server.FastForward(61 * time.Second)
	third, err := service.ExecuteWorkflow(context.Background(), input)
	if err != nil || third.Source != "workflow" || calls.Load() != 2 {
		t.Fatalf("expired execution = %#v, %v; calls=%d", third, err, calls.Load())
	}
}

func TestWorkflowResultCacheSeparatesExecutionInputs(t *testing.T) {
	for _, name := range []string{"tenant", "project", "record", "query", "oldRecord", "stepOutputs", "pagination", "definition", "access-metadata"} {
		t.Run(name, func(t *testing.T) {
			service, input, _, calls := workflowResultCacheFixture(t)
			if _, err := service.ExecuteWorkflow(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "tenant":
				input.TenantID = "another-tenant"
			case "project":
				input.ProjectID = "another-project"
			case "record":
				input.Record = map[string]interface{}{"result": "Other page"}
			case "query":
				input.Query = map[string]interface{}{"locale": "tr"}
			case "oldRecord":
				input.OldRecord = map[string]interface{}{"value": "old"}
			case "stepOutputs":
				input.StepOutputs = map[string]interface{}{"before": "changed"}
			case "pagination":
				input.Pager = &utils.Pager{Enabled: true, Page: 2, Limit: 10}
			case "definition":
				input.Container.Workflows[0].Steps[0].Config["value"] = "Updated page"
			case "access-metadata":
				input.Container.Routes.GetAllDynamicModelItems.IsAuthenticated = true
			}
			got, err := service.ExecuteWorkflow(context.Background(), input)
			if err != nil || got.Source != "workflow" || calls.Load() != 2 {
				t.Fatalf("%s reused another result: %#v, %v; calls=%d", name, got, err, calls.Load())
			}
		})
	}
}

func TestWorkflowResultCacheInvalidatesOnOtherCollectionWrite(t *testing.T) {
	service, input, _, calls := workflowResultCacheFixture(t)
	ctx := context.Background()
	if _, err := service.ExecuteWorkflow(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := utils.IncrementSchemaCacheVersion(ctx, input.TenantID, input.ProjectID, "visualBlocks"); err != nil {
		t.Fatal(err)
	}
	got, err := service.ExecuteWorkflow(ctx, input)
	if err != nil || got.Source != "workflow" || calls.Load() != 2 {
		t.Fatalf("old page reused after visual block update: %#v, %v", got, err)
	}
}

func TestWorkflowResultCacheBypassesAuthenticatedAndUnsafeExecution(t *testing.T) {
	for _, name := range []string{"disabled-cache", "authenticated-workflow", "authorized-workflow", "caller-role", "system", "unknown-identity", "missing-identity", "user", "form", "nested-write", "error", "partial-error"} {
		t.Run(name, func(t *testing.T) {
			service, input, _, calls := workflowResultCacheFixture(t)
			w := &input.Container.Workflows[0]
			switch name {
			case "disabled-cache":
				var replacement models.DynamicWorkflow
				raw, _ := json.Marshal(w)
				var body map[string]interface{}
				_ = json.Unmarshal(raw, &body)
				body["isRedisCached"] = false
				raw, _ = json.Marshal(body)
				_ = json.Unmarshal(raw, &replacement)
				*w = replacement
			case "authenticated-workflow":
				w.IsAuthenticated = true
			case "authorized-workflow":
				w.IsAuthorized = true
			case "caller-role":
				input.UserRole = "admin"
			case "system":
				input.IdentityKind = AccessIdentitySystem
			case "unknown-identity":
				input.IdentityKind = "tenant"
			case "missing-identity":
				input.IdentityKind = ""
			case "user":
				input.UserID = "user"
				service.loadAccessUser = func(context.Context, string, string, string) (map[string]interface{}, error) {
					return map[string]interface{}{"id": "user"}, nil
				}
			case "form":
				input.FormConfigRef = &FormConfigReference{PageID: "page", ComponentID: "form"}
			case "nested-write":
				w.Steps = append([]models.DynamicWorkflowStep{{Name: "branch", Type: models.WorkflowStepTypeIf, IsActive: true, Conditions: []models.WorkflowCondition{{Field: "record.never", Operator: models.WorkflowConditionEqual, Value: true}}, Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeCreateRecord, IsActive: true}}}}, w.Steps...)
			case "error", "partial-error":
				w.StopOnError = name == "error"
				w.Steps = append([]models.DynamicWorkflowStep{{Name: "failed", Type: models.WorkflowStepTypeFail, IsActive: true, Config: map[string]interface{}{"message": "temporary failure"}}}, w.Steps...)
			}
			for i := 0; i < 2; i++ {
				got, err := service.ExecuteWorkflow(context.Background(), input)
				if got.Source == "cache" {
					t.Fatalf("%s returned shared cache", name)
				}
				if name != "error" && name != "form" && err != nil {
					t.Fatal(err)
				}
			}
			if name != "form" && calls.Load() != 2 {
				t.Fatalf("%s executed %d times; want 2", name, calls.Load())
			}
		})
	}
}

func TestWorkflowResultCacheConcurrentFillAndRedisFailure(t *testing.T) {
	service, input, server, calls := workflowResultCacheFixture(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := service.ExecuteWorkflow(context.Background(), input); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent requests executed workflow %d times; want 1", calls.Load())
	}
	server.Close()
	got, err := service.ExecuteWorkflow(context.Background(), input)
	if err != nil || got.Source != "workflow" {
		t.Fatalf("Redis failure blocked page execution: %#v, %v", got, err)
	}
}

func TestWorkflowResultCacheNeverSkipsDatabaseArrayWrites(t *testing.T) {
	for _, kind := range []string{"append_array", "remove_array", "add_to_set", "push", "pull", "pull_all", "set_array"} {
		t.Run(kind, func(t *testing.T) {
			steps := []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeIf, IsActive: true, ElseSteps: []models.DynamicWorkflowStep{{Type: kind, IsActive: true}}}}
			if workflowResultCacheReadOnly(steps) {
				t.Fatalf("%s is a database write and must make the whole workflow ineligible", kind)
			}
		})
	}
}

func TestWorkflowResultCacheGenericHTTPCallsBypassSharing(t *testing.T) {
	steps:=[]models.DynamicWorkflowStep{{Type:models.WorkflowStepTypeCallAPI,IsActive:true,Config:map[string]interface{}{"method":"GET","url":"https://example.com"}}}
	if workflowResultCacheReadOnly(steps) {t.Fatal("generic call_api must bypass sharing; query_dynamic_api is the supported read-only HTTP operation")}
}

func TestWorkflowResultCachePartialLoopFailuresRemainRetryable(t *testing.T) {
	service, input, _, calls := workflowResultCacheFixture(t)
	w := &input.Container.Workflows[0]
	w.StopOnError = false
	w.Steps = append([]models.DynamicWorkflowStep{{Name: "loop", Type: models.WorkflowStepTypeForEach, IsActive: true, Config: map[string]interface{}{"items": []interface{}{1, 2}}, Steps: []models.DynamicWorkflowStep{{Name: "failed", Type: models.WorkflowStepTypeFail, IsActive: true, Config: map[string]interface{}{"message": "retry"}}}}}, w.Steps...)
	for i := 0; i < 2; i++ {
		got, err := service.ExecuteWorkflow(context.Background(), input)
		if err != nil || got.Source != "workflow" {
			t.Fatalf("partial loop failure was cached: %#v, %v", got, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("failed loop ran %d times", calls.Load())
	}
}

func TestWorkflowResultCacheCanceledOrDisabledRequestsCannotReuseResults(t *testing.T) {
	service, input, _, _ := workflowResultCacheFixture(t)
	if _, err := service.ExecuteWorkflow(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := service.ExecuteWorkflow(ctx, input); err == nil || got.Source == "cache" {
		t.Fatalf("canceled request reused cached data: %#v %v", got, err)
	}
	input.Container.Workflows[0].IsActive = false
	if got, err := service.ExecuteWorkflow(context.Background(), input); serviceErrorStatus(err) != 403 || got.Source == "cache" {
		t.Fatalf("disabled workflow reused cached data: %#v %v", got, err)
	}
}

func TestWorkflowResultCachePreservesNullAndScalarOutput(t *testing.T) {
	for _, value := range []interface{}{nil, "text", false, []interface{}{}, map[string]interface{}{}} {
		t.Run(string(func() []byte { data, _ := json.Marshal(value); return data }()), func(t *testing.T) {
			service, input, _, _ := workflowResultCacheFixture(t)
			input.Record["result"] = value
			a, err := service.ExecuteWorkflow(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			b, err := service.ExecuteWorkflow(context.Background(), input)
			if err != nil || b.Source != "cache" {
				t.Fatalf("scalar cache miss: %#v %v", b, err)
			}
			before, _ := json.Marshal(a.Data)
			after, _ := json.Marshal(b.Data)
			if string(before) != string(after) {
				t.Fatalf("output changed: %s -> %s", before, after)
			}
		})
	}
}
