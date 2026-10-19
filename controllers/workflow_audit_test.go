package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
)

func TestWorkflowHandlerAcceptsBulkBody(t *testing.T) {
	app := fiber.New()
	app.Post("/workflow/:workflowName", func(c *fiber.Ctx) error {
		c.Locals("tenantID", "tenant")
		c.Locals("projectID", "project")
		c.Locals("containerModel", &models.ContainerModel{SchemaName: "orders", Workflows: []models.DynamicWorkflow{{
			Name: "bulk", IsActive: true, Steps: []models.DynamicWorkflowStep{{Type: models.WorkflowStepTypeReturn, IsActive: true, Config: map[string]interface{}{"value": "{{record.productIds}}"}}},
		}}})
		return c.Next()
	}, ExecuteWorkflow)
	req := httptest.NewRequest(http.MethodPost, "/workflow/bulk?schemaName=orders", strings.NewReader(`[{"productId":"first"},{"productId":"second"}]`))
	req.Header.Set("Content-Type", "application/json")
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bulk workflow status = %d, body = %s", response.StatusCode, data)
	}
	var envelope struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !reflect.DeepEqual(envelope.Data, []string{"first", "second"}) {
		t.Fatalf("bulk response = %s, error = %v", data, err)
	}
}

func TestWorkflowRequestBodyRejectsMalformedShapes(t *testing.T) {
	for _, body := range []string{`{`, `null`, `true`, `1`, `"record"`, `{}` + `{}`} {
		t.Run(body, func(t *testing.T) {
			if _, _, _, err := parseWorkflowRequestBody([]byte(body)); err == nil {
				t.Fatalf("invalid workflow body %s accepted", body)
			}
		})
	}
}

func TestWorkflowRequestBodyPreservesExplicitProductIDsAndEnvelope(t *testing.T) {
	for _, body := range []string{
		`{"items":[{"productId":"derived"}],"productIds":["explicit"]}`,
		`{"record":{"items":[{"productId":"derived"}],"productIds":["explicit"]},"oldRecord":{"state":"old"},"stepOutputs":{"before":7},"formConfigRef":{"pageId":"page","componentId":"component"}}`,
	} {
		record, old, outputs, err := parseWorkflowRequestBody([]byte(body))
		if err != nil || !reflect.DeepEqual(record["productIds"], []interface{}{"explicit"}) {
			t.Fatalf("explicit IDs replaced: %#v, %v", record, err)
		}
		if _, has := record["formConfigRef"]; has {
			t.Fatal("form configuration metadata leaked into record")
		}
		if strings.Contains(body, "oldRecord") && (old["state"] != "old" || outputs["before"] != float64(7)) {
			t.Fatalf("envelope lost old state/outputs: %#v %#v", old, outputs)
		}
	}
}

func TestWorkflowRequestBodySkipsItemsWithoutProductIDs(t *testing.T) {
	record, _, _, err := parseWorkflowRequestBody([]byte(`[null,1,"text",{}, {"productId":null},{"productId":""},{"productId":"valid"},{"productId":12}]`))
	if err != nil || !reflect.DeepEqual(record["productIds"], []interface{}{"valid", float64(12)}) {
		t.Fatalf("productIds = %#v, error = %v", record["productIds"], err)
	}
}

func TestWorkflowFormReferenceBodyShapes(t *testing.T) {
	for _, body := range []string{"", "  ", `{}`, `[]`, `[{"productId":"one"}]`, `{"formConfigRef":null}`} {
		t.Run(body, func(t *testing.T) {
			ref, err := parseWorkflowFormConfigReference([]byte(body))
			if err != nil || ref != nil {
				t.Fatalf("optional reference for %q = %#v, %v", body, ref, err)
			}
		})
	}
	for _, body := range []string{`{`, `{"formConfigRef":"bad"}`, `{"formConfigRef":{"componentId":"only"}}`} {
		if _, err := parseWorkflowFormConfigReference([]byte(body)); err == nil {
			t.Errorf("invalid reference accepted: %s", body)
		}
	}
}

func TestWorkflowRequestDeadlinePreservesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, cleanup := workflowRequestContext(parent)
	defer cleanup()
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("parent cancellation = %v", ctx.Err())
	}
	parent, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx, cleanup = workflowRequestContext(parent)
	defer cleanup()
	want, _ := parent.Deadline()
	got, _ := ctx.Deadline()
	if !got.Equal(want) {
		t.Fatalf("workflow extended parent deadline: %s, want %s", got, want)
	}
	ctx, cleanup = workflowRequestContext(nil)
	defer cleanup()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("nil parent did not receive a bounded context")
	}
}
