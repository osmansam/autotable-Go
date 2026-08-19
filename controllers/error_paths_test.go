package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestWorkflowRequestContextAllowsLongRunningWorkflow(t *testing.T) {
	ctx, cancel := workflowRequestContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("workflow request context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 299*time.Second || remaining > 300*time.Second {
		t.Fatalf("workflow request context timeout = %v, want approximately 300s", remaining)
	}
}

func TestDynamicHandlersRejectMissingProjectContext(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler fiber.Handler
	}{
		{name: "create item", method: http.MethodPost, path: "/", handler: CreateDynamicModelItem},
		{name: "create items", method: http.MethodPost, path: "/", handler: CreateMultipleDynamicModelItem},
		{name: "get all", method: http.MethodGet, path: "/", handler: GetAllDynamicModelItems},
		{name: "selection", method: http.MethodGet, path: "/", handler: GetItemsForSelection},
		{name: "delete item", method: http.MethodDelete, path: "/id", handler: DeleteDynamicModelItem},
		{name: "delete items", method: http.MethodDelete, path: "/", handler: DeleteMultipleDynamicModelItem},
		{name: "update item", method: http.MethodPatch, path: "/id", handler: UpdateDynamicModelItem},
		{name: "update items", method: http.MethodPatch, path: "/", handler: UpdateMultipleDynamicModelItem},
		{name: "get item", method: http.MethodGet, path: "/id", handler: GetDynamicModelItem},
		{name: "search", method: http.MethodGet, path: "/", handler: HandleSearchDynamicModelItem},
		{name: "filter", method: http.MethodGet, path: "/", handler: HandleFilterDynamicModelItem},
		{name: "pipeline", method: http.MethodGet, path: "/", handler: GetPipeline},
		{name: "pagination", method: http.MethodGet, path: "/", handler: GetAllDynamicModelItemsWithPagination},
		{name: "execute code", method: http.MethodPost, path: "/", handler: ExecuteDynamicCode},
		{name: "test pipeline", method: http.MethodPost, path: "/", handler: TestPipeline},
		{name: "execute api", method: http.MethodPost, path: "/", handler: ExecuteDynamicAPI},
		{name: "execute workflow", method: http.MethodPost, path: "/", handler: ExecuteWorkflow},
		{name: "export", method: http.MethodPost, path: "/", handler: ExportDynamicModelItems},
		{name: "add array row", method: http.MethodPost, path: "/checklist/id/array/duties", handler: AddDynamicArrayRow},
		{name: "update array row", method: http.MethodPatch, path: "/checklist/id/array/duties/Open", handler: UpdateDynamicArrayRow},
		{name: "delete array row", method: http.MethodDelete, path: "/checklist/id/array/duties/Open", handler: DeleteDynamicArrayRow},
		{name: "reorder array rows", method: http.MethodPatch, path: "/checklist/id/array/duties/reorder", handler: ReorderDynamicArrayRows},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Add(tt.method, tt.path, tt.handler)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			if tt.name == "logout" {
				req.Header.Set("Origin", "null")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode < http.StatusBadRequest {
				t.Fatalf("status = %d, want error status", resp.StatusCode)
			}
		})
	}
}

func TestDynamicArrayHandlerRejectsMalformedBody(t *testing.T) {
	app := fiber.New()
	app.Post("/:schema/:id/array/:field", AddDynamicArrayRow)
	req := httptest.NewRequest(
		http.MethodPost,
		"/checklist/6a7d31f77024197f5f2a25c4/array/duties?tenantID=tenant&projectID=project",
		strings.NewReader(`{"rowIdentityField":"duty","unknown":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestParseWorkflowRequestBodyAcceptsBulkArray(t *testing.T) {
	body := []byte(`[
		{"productId":"6a56770742b009c32a92d202","quantity":1},
		{"productId":"6a56770742b009c32a92d1f8","quantity":2}
	]`)

	record, oldRecord, stepOutputs, err := parseWorkflowRequestBody(body)
	if err != nil {
		t.Fatalf("parseWorkflowRequestBody() error = %v", err)
	}

	wantProductIDs := []interface{}{"6a56770742b009c32a92d202", "6a56770742b009c32a92d1f8"}
	if !reflect.DeepEqual(record["productIds"], wantProductIDs) {
		t.Fatalf("record.productIds = %#v, want %#v", record["productIds"], wantProductIDs)
	}
	items, ok := record["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("record.items = %#v, want two array items", record["items"])
	}
	if oldRecord == nil || len(oldRecord) != 0 {
		t.Fatalf("oldRecord = %#v, want empty map", oldRecord)
	}
	if stepOutputs == nil || len(stepOutputs) != 0 {
		t.Fatalf("stepOutputs = %#v, want empty map", stepOutputs)
	}
}

func TestParseWorkflowRequestBodyAddsProductIDsForWrappedItems(t *testing.T) {
	body := []byte(`{
		"record": {
			"items": [
				{"productId":"6a486f0faadf8857d624d25e","quantity":1},
				{"productId":"6a486f0faadf8857d624d26d","quantity":1}
			]
		}
	}`)

	record, _, _, err := parseWorkflowRequestBody(body)
	if err != nil {
		t.Fatalf("parseWorkflowRequestBody() error = %v", err)
	}

	wantProductIDs := []interface{}{"6a486f0faadf8857d624d25e", "6a486f0faadf8857d624d26d"}
	if !reflect.DeepEqual(record["productIds"], wantProductIDs) {
		t.Fatalf("record.productIds = %#v, want %#v", record["productIds"], wantProductIDs)
	}
}

func TestParseWorkflowFormConfigReference(t *testing.T) {
	ref, err := parseWorkflowFormConfigReference([]byte(`{
		"record":{"items":[]},
		"formConfigRef":{"pageId":"68a4f56da3c241f066ff1142","componentId":"cmp-order"}
	}`))
	if err != nil {
		t.Fatalf("parseWorkflowFormConfigReference() error = %v", err)
	}
	if ref == nil || ref.PageID != "68a4f56da3c241f066ff1142" || ref.ComponentID != "cmp-order" {
		t.Fatalf("reference = %#v", ref)
	}
	record, _, _, err := parseWorkflowRequestBody([]byte(`{
		"record":{"items":[]},
		"formConfigRef":{"pageId":"68a4f56da3c241f066ff1142","componentId":"cmp-order"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := record["formConfigRef"]; exists {
		t.Fatal("formConfigRef leaked into workflow record")
	}
}

func TestParseWorkflowFormConfigReferenceRejectsPartialReference(t *testing.T) {
	if _, err := parseWorkflowFormConfigReference([]byte(`{"formConfigRef":{"pageId":"page"}}`)); err == nil {
		t.Fatal("partial formConfigRef was accepted")
	}
	ref, err := parseWorkflowFormConfigReference([]byte(`{"record":{}}`))
	if err != nil || ref != nil {
		t.Fatalf("missing reference = %#v, %v", ref, err)
	}
}

func TestContainerHandlersRejectMissingProjectContext(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler fiber.Handler
	}{
		{name: "create", method: http.MethodPost, path: "/", handler: CreateContainer},
		{name: "list", method: http.MethodGet, path: "/", handler: GetAllContainers},
		{name: "delete", method: http.MethodDelete, path: "/id", handler: DeleteContainer},
		{name: "update", method: http.MethodPatch, path: "/id", handler: UpdateContainer},
		{name: "pipelines", method: http.MethodPatch, path: "/id", handler: UpdatePipelines},
		{name: "workflows", method: http.MethodPatch, path: "/id", handler: UpdateWorkflows},
		{name: "functions", method: http.MethodPatch, path: "/id", handler: UpdateDynamicFunctions},
		{name: "dynamic apis", method: http.MethodPatch, path: "/id", handler: UpdateDynamicApis},
		{name: "get", method: http.MethodGet, path: "/id", handler: GetContainer},
		{name: "types", method: http.MethodGet, path: "/", handler: GetAllContainerTypes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Add(tt.method, tt.path, tt.handler)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			if tt.name == "logout" {
				req.Header.Set("Origin", "null")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode < http.StatusBadRequest {
				t.Fatalf("status = %d, want error status", resp.StatusCode)
			}
		})
	}
}

func TestControllerInputErrorsWithoutDatabase(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		handler     fiber.Handler
	}{
		{name: "register missing schema", method: http.MethodPost, path: "/", body: `{}`, contentType: "application/json", handler: Register},
		{name: "tenant register malformed body", method: http.MethodPost, path: "/", body: `{`, contentType: "application/json", handler: TenantRegister},
		{name: "tenant register validation", method: http.MethodPost, path: "/", body: `{}`, contentType: "application/json", handler: TenantRegister},
		{name: "project malformed body", method: http.MethodPost, path: "/", body: `{`, contentType: "application/json", handler: CreateProject},
		{name: "project invalid slug", method: http.MethodPost, path: "/", body: `{"name":"Project","slug":"INVALID"}`, contentType: "application/json", handler: CreateProject},
		{name: "excel missing file", method: http.MethodPost, path: "/", handler: UploadExcel},
		{name: "bulk excel invalid form", method: http.MethodPost, path: "/", handler: UploadMultipleExcel},
		{name: "audit invalid user id", method: http.MethodGet, path: "/?tenantID=tenant&projectID=project&userId=invalid", handler: GetAuditLogs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Add(tt.method, "/", tt.handler)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode < http.StatusBadRequest {
				t.Fatalf("status = %d, want error status", resp.StatusCode)
			}
		})
	}
}

func TestPageAndAuthHandlersRejectMissingContext(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler fiber.Handler
	}{
		{name: "create page", method: http.MethodPost, path: "/", handler: CreatePage},
		{name: "list pages", method: http.MethodGet, path: "/", handler: GetAllPages},
		{name: "list public pages", method: http.MethodGet, path: "/", handler: GetAllPagesPublic},
		{name: "get page", method: http.MethodGet, path: "/id", handler: GetPage},
		{name: "update page", method: http.MethodPatch, path: "/id", handler: UpdatePage},
		{name: "delete page", method: http.MethodDelete, path: "/id", handler: DeletePage},
		{name: "login", method: http.MethodPost, path: "/", handler: Login},
		{name: "google login", method: http.MethodGet, path: "/", handler: GoogleLogin},
		{name: "logout", method: http.MethodPost, path: "/", handler: Logout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Add(tt.method, tt.path, tt.handler)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			if tt.name == "logout" {
				req.Header.Set("Origin", "null")
			}
			resp, err := app.Test(req)
			if tt.name == "logout" && err == nil && resp.StatusCode == http.StatusOK {
				return
			}
			if err != nil || resp.StatusCode < http.StatusBadRequest {
				t.Fatalf("response = %#v, error = %v", resp, err)
			}
		})
	}
}

func TestAuthHandlersRejectInvalidInputWithoutDatabase(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler fiber.Handler
	}{
		{name: "google callback missing state", method: http.MethodGet, path: "/", handler: GoogleCallback},
		{name: "tenant login malformed body", method: http.MethodPost, path: "/", body: `{`, handler: TenantLogin},
		{name: "switch project malformed body", method: http.MethodPost, path: "/", body: `{`, handler: SwitchToProject},
		{name: "refresh malformed body", method: http.MethodPost, path: "/", body: `{`, handler: TenantRefreshToken},
		{name: "refresh invalid token", method: http.MethodPost, path: "/", body: `{"refreshToken":"invalid"}`, handler: TenantRefreshToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Add(tt.method, tt.path, tt.handler)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := app.Test(req)
			if err != nil || resp.StatusCode < http.StatusBadRequest {
				t.Fatalf("response = %#v, error = %v", resp, err)
			}
		})
	}
}

func TestTenantAndRoleHandlersRejectInvalidIDs(t *testing.T) {
	app := fiber.New()
	app.Get("/user", func(c *fiber.Ctx) error {
		c.Locals("tenantUserID", "invalid")
		return GetCurrentUser(c)
	})
	app.Get("/role", func(c *fiber.Ctx) error {
		c.Locals("roles", []string{"admin"})
		c.Locals("tenantID", "tenant")
		c.Locals("projectID", "project")
		return GetRoleItemById(c)
	})
	for _, path := range []string{"/user", "/role"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s response = %#v, error = %v", path, resp, err)
		}
	}
}
