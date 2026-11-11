package controllers

import (
	"context"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type visualDesignMemoryStore struct {
	values          []models.VisualComponentDesign
	tenant, project string
	conflict        bool
}

func (s *visualDesignMemoryStore) List(_ context.Context, t, p string) ([]models.VisualComponentDesign, error) {
	s.tenant = t
	s.project = p
	return s.values, nil
}
func (s *visualDesignMemoryStore) Create(_ context.Context, t, p string, v models.VisualComponentDesign) (models.VisualComponentDesign, error) {
	s.tenant = t
	s.project = p
	v.ID = primitive.NewObjectID()
	v.Revision = 1
	s.values = append(s.values, v)
	return v, nil
}
func (s *visualDesignMemoryStore) Update(_ context.Context, t, p string, v models.VisualComponentDesign) (models.VisualComponentDesign, error) {
	s.tenant = t
	s.project = p
	if s.conflict {
		return v, repositories.ErrVisualComponentConflict
	}
	v.Revision++
	return v, nil
}
func TestVisualComponentHTTPCreateAndConflict(t *testing.T) {
	store := &visualDesignMemoryStore{}
	handler := VisualComponentController{Store: store}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenantID", "tenant-a")
		c.Locals("projectID", "project-a")
		return c.Next()
	})
	app.Post("/designs", handler.Save)
	app.Put("/designs/:id", handler.Save)
	value := `{"document":{"version":2,"id":"doc","name":"Card","root":{"id":"root","type":"section","children":[]}},"theme":{"palettes":{"light":{"primary-foreground":"#fff"}}}}`
	request := httptest.NewRequest("POST", "/designs", strings.NewReader(`{"name":" Card ","value":`+value+`}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d %s", response.StatusCode, body)
	}
	var saved models.VisualComponentDesign
	if err = json.NewDecoder(response.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Card" || saved.Revision != 1 || store.tenant != "tenant-a" || store.project != "project-a" {
		t.Fatalf("saved=%+v scope=%s/%s", saved, store.tenant, store.project)
	}
	store.conflict = true
	request = httptest.NewRequest("PUT", "/designs/"+saved.ID.Hex(), strings.NewReader(`{"name":"Card","revision":1,"value":`+value+`}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 409 {
		t.Fatalf("status=%d", response.StatusCode)
	}
	request = httptest.NewRequest("POST", "/designs", strings.NewReader(`{"name":"Card","value":{}}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 400 || len(store.values) != 1 {
		t.Fatalf("invalid saved status=%d count=%d", response.StatusCode, len(store.values))
	}
}
