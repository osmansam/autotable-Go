package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestBulkContainersContinueAfterFailureAndUseResolvedScope(t *testing.T) {
	id := primitive.NewObjectID()
	var names []string
	app := fiber.New()
	app.Post("/", newCreateMultipleContainersHandler(
		func(c *fiber.Ctx) (string, string, error) { return "tenant-url", "project-url", nil },
		func(ctx context.Context, container models.ContainerModel, tenantID, projectID string) (*mongo.InsertOneResult, error) {
			if tenantID != "tenant-url" || projectID != "project-url" {
				t.Errorf("wrong scope: %s/%s", tenantID, projectID)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("missing per-container timeout")
			}
			names = append(names, container.SchemaName)
			if container.SchemaName == "existing" {
				return nil, fiber.NewError(409, "already exists")
			}
			return &mongo.InsertOneResult{InsertedID: id}, nil
		},
	))
	req := httptest.NewRequest(http.MethodPost, "/?tenantID=ignored&projectID=ignored", strings.NewReader(`[{"schemaName":"first"},{"schemaName":"existing"},{"schemaName":"last"}]`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 207 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body struct {
		Created int `json:"created"`
		Failed  int `json:"failed"`
		Results []struct {
			SchemaName string `json:"schemaName"`
			Status     int    `json:"status"`
			ID         string `json:"id"`
			Error      string `json:"error"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Created != 2 || body.Failed != 1 || len(body.Results) != 3 {
		t.Fatalf("unexpected response: %+v", body)
	}
	if strings.Join(names, ",") != "first,existing,last" {
		t.Fatalf("creation order: %v", names)
	}
	if body.Results[0].ID != id.Hex() || body.Results[1].Status != 409 || body.Results[1].Error == "" || body.Results[2].Status != 201 {
		t.Fatalf("unexpected results: %+v", body.Results)
	}
}

func TestBulkContainersRejectInvalidBatchBeforeResolvingProject(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `[]`, `[`, `[null]`, `[{}]`, `[{"schemaName":"   "}]`, `[{"schemaName":"same"},{"schemaName":"same"}]`, "[" + strings.Repeat(`{"schemaName":"x"},`, 100) + `{"schemaName":"last"}]`} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			app := fiber.New()
			app.Post("/", newCreateMultipleContainersHandler(func(c *fiber.Ctx) (string, string, error) {
				t.Error("invalid batch reached resolver")
				return "", "", fiber.ErrBadRequest
			}, nil))
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Fatalf("status=%d", resp.StatusCode)
			}
		})
	}
}

func TestBulkContainersAllCreatedAndMissingProject(t *testing.T) {
	for _, missing := range []bool{false, true} {
		app := fiber.New()
		app.Post("/", newCreateMultipleContainersHandler(func(c *fiber.Ctx) (string, string, error) {
			if missing {
				return "", "", fiber.ErrNotFound
			}
			return "tenant", "project", nil
		}, func(ctx context.Context, c models.ContainerModel, t, p string) (*mongo.InsertOneResult, error) {
			return &mongo.InsertOneResult{InsertedID: primitive.NewObjectID()}, nil
		}))
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`[{"schemaName":"pets"}]`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := 201
		if missing {
			want = 404
		}
		if resp.StatusCode != want {
			t.Errorf("missing=%v: status=%d want=%d", missing, resp.StatusCode, want)
		}
	}
}

func TestContainerValidationMessagesSurviveProductionErrorHandler(t *testing.T) {
	for _, single := range []bool{true, false} {
		app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error { return utils.SendErrorResponse(c, err, "Internal server error") }})
		body := `[]`
		want := "array"
		if single {
			body = `{"schemaName":"containers"}`
			want = "restricted"
			app.Post("/", func(c *fiber.Ctx) error {
				c.Locals("tenantID", "tenant")
				c.Locals("projectID", "project")
				return CreateContainer(c)
			})
		} else {
			app.Post("/", CreateMultipleContainers)
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var got struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 400 || !strings.Contains(got.Message, want) {
			t.Errorf("single=%v: status=%d message=%q", single, resp.StatusCode, got.Message)
		}
	}
}

func TestBulkContainersReportPersistedItemWithPostCreationError(t *testing.T) {
	id := primitive.NewObjectID()
	app := fiber.New()
	app.Post("/", newCreateMultipleContainersHandler(func(c *fiber.Ctx) (string, string, error) { return "tenant", "project", nil }, func(ctx context.Context, c models.ContainerModel, t, p string) (*mongo.InsertOneResult, error) {
		return &mongo.InsertOneResult{InsertedID: id}, fiber.NewError(500, "Container created but invalidation failed")
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`[{"schemaName":"pets"}]`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Created int `json:"created"`
		Failed  int `json:"failed"`
		Results []struct {
			ID      string `json:"id"`
			Created bool   `json:"created"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 207 || got.Created != 1 || got.Failed != 1 || len(got.Results) != 1 || !got.Results[0].Created || got.Results[0].ID != id.Hex() || got.Results[0].Error == "" {
		t.Fatalf("status=%d body=%+v", resp.StatusCode, got)
	}
}
