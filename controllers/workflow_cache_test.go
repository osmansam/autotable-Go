package controllers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

func TestWorkflowHTTPResponseCachesPublicPageAndPreservesData(t *testing.T) {
	server := miniredis.RunT(t)
	previous := configs.RedisClient
	configs.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = configs.RedisClient.Close(); configs.RedisClient = previous })
	var workflow models.DynamicWorkflow
	if err := json.Unmarshal([]byte(`{"name":"loadPublicPage","isActive":true,"isRedisCached":true,"cacheTime":1,"steps":[{"type":"return","isActive":true,"config":{"value":"{{record}}"}}]}`), &workflow); err != nil {
		t.Fatal(err)
	}
	container := &models.ContainerModel{SchemaName: "siteConfig", Workflows: []models.DynamicWorkflow{workflow}}
	app := fiber.New()
	app.Post("/workflow/:workflowName", func(c *fiber.Ctx) error {
		c.Locals("tenantID", "tenant")
		c.Locals("projectID", "project")
		c.Locals("containerModel", container)
		return c.Next()
	}, ExecuteWorkflow)
	var original json.RawMessage
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest("POST", "/workflow/loadPublicPage?schemaName=siteConfig", strings.NewReader(`{"record":{"sitePath":["contact"],"locale":"en","number":9007199254740993}}`))
		request.Header.Set("content-type", "application/json")
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("status=%d: %s", response.StatusCode, body)
		}
		var envelope struct {
			Status int             `json:"status"`
			Source string          `json:"source"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != 200 {
			t.Fatalf("wrong envelope: %s", body)
		}
		if i == 0 {
			original = envelope.Data
		} else if envelope.Source != "cache" || string(envelope.Data) != string(original) {
			t.Fatalf("cache changed HTTP data: %s -> %s", original, body)
		}
	}
}
