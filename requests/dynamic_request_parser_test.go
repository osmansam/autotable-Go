package requests

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func TestParseLimitedItems(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		max       int
		operation string
		want      []map[string]interface{}
		wantErr   string
		wantLimit bool
	}{
		{name: "empty array", data: `[]`, max: 2, want: []map[string]interface{}{}},
		{name: "within limit", data: `[{"name":"one"},{"name":"two"}]`, max: 2, want: []map[string]interface{}{{"name": "one"}, {"name": "two"}}},
		{name: "non array", data: `{"name":"one"}`, max: 2, wantErr: "expected JSON array"},
		{name: "invalid json", data: `[{"name":`, max: 2, wantErr: "unexpected EOF"},
		{name: "over limit", data: `[{},{}]`, max: 1, operation: "bulk write", wantErr: "bulk write limit exceeded", wantLimit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLimitedItems([]byte(tt.data), tt.max, tt.operation)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("parseLimitedItems() error = %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("parseLimitedItems() = %#v, want %#v", got, tt.want)
				}
				return
			}
			if err == nil || !contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseLimitedItems() error = %v, want containing %q", err, tt.wantErr)
			}
			var limitErr *BatchLimitError
			if errors.As(err, &limitErr) != tt.wantLimit {
				t.Fatalf("BatchLimitError = %v, want %v", errors.As(err, &limitErr), tt.wantLimit)
			}
		})
	}
}

func TestConvertFormFieldTypes(t *testing.T) {
	item := map[string]interface{}{
		"enabled":  "true",
		"count":    "42",
		"ratio":    "2.5",
		"tags":     " red, blue ",
		"scores":   "1, 2.5, bad",
		"invalid":  "not-an-int",
		"metadata": map[string]interface{}{"active": "false", "rank": "7"},
		"rows": []interface{}{
			map[string]interface{}{"amount": "3.5"},
		},
	}
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "enabled", Type: "bool"},
		{Name: "count", Type: "int"},
		{Name: "ratio", Type: "decimal"},
		{Name: "tags", Type: "stringArray"},
		{Name: "scores", Type: "numberArray"},
		{Name: "invalid", Type: "int"},
		{Name: "metadata", Type: "object", Children: []models.Field{{Name: "active", Type: "boolean"}, {Name: "rank", Type: "int"}}},
		{Name: "rows", Type: "array", Children: []models.Field{{Name: "amount", Type: "float"}}},
	}}

	ConvertFormFieldTypes(item, container)

	want := map[string]interface{}{
		"enabled":  true,
		"count":    42,
		"ratio":    2.5,
		"tags":     []interface{}{"red", "blue"},
		"scores":   []interface{}{1, 2.5},
		"invalid":  "not-an-int",
		"metadata": map[string]interface{}{"active": false, "rank": 7},
		"rows": []interface{}{
			map[string]interface{}{"amount": 3.5},
		},
	}
	if !reflect.DeepEqual(item, want) {
		t.Fatalf("ConvertFormFieldTypes() = %#v, want %#v", item, want)
	}
}

func TestSplitArrays(t *testing.T) {
	if got := splitStringArray(""); !reflect.DeepEqual(got, []interface{}{}) {
		t.Fatalf("splitStringArray(empty) = %#v", got)
	}
	if got := splitNumberArray(""); !reflect.DeepEqual(got, []interface{}{}) {
		t.Fatalf("splitNumberArray(empty) = %#v", got)
	}
}

func TestHasImageField(t *testing.T) {
	tests := []struct {
		name      string
		container *models.ContainerModel
		want      bool
	}{
		{name: "no fields", container: &models.ContainerModel{}},
		{name: "non image field", container: &models.ContainerModel{Fields: []models.Field{{Type: "string"}}}},
		{name: "image field", container: &models.ContainerModel{Fields: []models.Field{{Type: "image"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasImageField(tt.container); got != tt.want {
				t.Fatalf("hasImageField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBatchLimitError(t *testing.T) {
	err := (&BatchLimitError{Operation: "bulk delete", Max: 10}).Error()
	if err != "bulk delete limit exceeded. Maximum allowed items is 10." {
		t.Fatalf("Error() = %q", err)
	}
}

func TestDynamicRequestParserJSONPaths(t *testing.T) {
	parser := NewDynamicRequestParser(nil)
	container := &models.ContainerModel{}
	app := fiber.New()
	app.Post("/item", func(c *fiber.Ctx) error {
		item, err := parser.ParseCreateItem(c, container)
		if err != nil || !reflect.DeepEqual(item, map[string]interface{}{"name": "Ada"}) {
			t.Fatalf("ParseCreateItem() = %#v, %v", item, err)
		}
		return nil
	})
	app.Post("/items", func(c *fiber.Ctx) error {
		items, err := parser.ParseCreateItems(c, container, 2)
		if err != nil || len(items) != 1 {
			t.Fatalf("ParseCreateItems() = %#v, %v", items, err)
		}
		return nil
	})
	app.Post("/update", func(c *fiber.Ctx) error {
		item, err := parser.ParseUpdateItem(c, container)
		if err != nil || !reflect.DeepEqual(item, map[string]interface{}{"name": "Lin"}) {
			t.Fatalf("ParseUpdateItem() = %#v, %v", item, err)
		}
		return nil
	})
	app.Post("/delete", func(c *fiber.Ctx) error {
		items, err := parser.ParseDeleteItems(c, 2)
		if err != nil || len(items) != 1 {
			t.Fatalf("ParseDeleteItems() = %#v, %v", items, err)
		}
		return nil
	})
	app.Post("/api", func(c *fiber.Ctx) error {
		body, err := parser.ParseDynamicAPIRequest(c)
		if err != nil || body == nil {
			t.Fatalf("ParseDynamicAPIRequest() = %#v, %v", body, err)
		}
		return nil
	})
	for path, body := range map[string]string{
		"/item":   `{"name":"Ada"}`,
		"/items":  `[{"name":"Ada"}]`,
		"/update": `{"name":"Lin"}`,
		"/delete": `[{"_id":"1"}]`,
		"/api":    `null`,
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if _, err := app.Test(req); err != nil {
			t.Fatalf("app.Test(%s) error = %v", path, err)
		}
	}
}

func TestDynamicRequestParserQueryPaths(t *testing.T) {
	parser := NewDynamicRequestParser(nil)
	container := &models.ContainerModel{Fields: []models.Field{{Name: "age", Type: "int"}}}
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		search, err := parser.ParseSearchParams(c)
		if err != nil || search.SearchKey != "Ada" || search.Pager.Page != 2 {
			t.Fatalf("ParseSearchParams() = %#v, %v", search, err)
		}
		filter, err := parser.ParseFilterParams(c, container)
		if err != nil || !reflect.DeepEqual(filter.Filter, bson.M{"age": 42}) {
			t.Fatalf("ParseFilterParams() = %#v, %v", filter, err)
		}
		paginated, err := parser.ParsePaginatedItemsParams(c, container)
		if err != nil || paginated.QueryString == "" {
			t.Fatalf("ParsePaginatedItemsParams() = %#v, %v", paginated, err)
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/?search=Ada&page=2&limit=5&age=42", nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestDynamicRequestParserMultipartPaths(t *testing.T) {
	parser := NewDynamicRequestParser(nil)
	container := &models.ContainerModel{}
	app := fiber.New()
	app.Post("/create", func(c *fiber.Ctx) error {
		items, err := parser.ParseCreateItems(c, container, 2)
		if err != nil || !reflect.DeepEqual(items, []map[string]interface{}{{"name": "Ada"}}) {
			t.Fatalf("ParseCreateItems() = %#v, %v", items, err)
		}
		return nil
	})
	app.Post("/update", func(c *fiber.Ctx) error {
		items, err := parser.ParseUpdateItems(c, container, 2)
		if err != nil || !reflect.DeepEqual(items, []map[string]interface{}{{"name": "Lin"}}) {
			t.Fatalf("ParseUpdateItems() = %#v, %v", items, err)
		}
		return nil
	})
	app.Post("/missing", func(c *fiber.Ctx) error {
		if _, err := parser.ParseCreateItems(c, container, 2); err == nil {
			t.Fatal("ParseCreateItems(missing items) error = nil")
		}
		return nil
	})
	app.Post("/mismatch", func(c *fiber.Ctx) error {
		imageContainer := &models.ContainerModel{Fields: []models.Field{{Name: "image", Type: "image"}}}
		if _, err := parser.ParseUpdateItems(c, imageContainer, 2); err == nil || !strings.Contains(err.Error(), "Expected 2 files") {
			t.Fatalf("ParseUpdateItems(mismatched files) error = %v", err)
		}
		return nil
	})

	for _, tt := range []struct {
		path   string
		fields map[string]string
		files  map[string][]string
	}{
		{path: "/create", fields: map[string]string{"items": `[{"name":"Ada"}]`}},
		{path: "/update", fields: map[string]string{"items": `[{"name":"Lin"}]`}},
		{path: "/missing"},
		{path: "/mismatch", fields: map[string]string{"items": `[{},{}]`}, files: map[string][]string{"image": {"image.txt"}}},
	} {
		body, contentType := multipartBody(t, tt.fields, tt.files)
		req := httptest.NewRequest(http.MethodPost, tt.path, body)
		req.Header.Set("Content-Type", contentType)
		if _, err := app.Test(req); err != nil {
			t.Fatalf("app.Test(%q) error = %v", tt.path, err)
		}
	}
}

func TestDynamicRequestParserAdditionalWrappers(t *testing.T) {
	parser := NewDynamicRequestParser(nil)
	app := fiber.New()
	app.Get("/pipeline", func(c *fiber.Ctx) error {
		got := parser.ParsePipelineParams(c)
		if got.SchemaName != "orders" || got.PipelineName != "summary" || got.CurrentQuery == "" {
			t.Fatalf("ParsePipelineParams() = %#v", got)
		}
		return nil
	})
	app.Post("/test", func(c *fiber.Ctx) error {
		got, err := parser.ParseTestPipeline(c)
		if err != nil || got.PipelineStage.Name != "summary" {
			t.Fatalf("ParseTestPipeline() = %#v, %v", got, err)
		}
		return nil
	})
	app.Post("/export", func(c *fiber.Ctx) error {
		got, err := parser.ParseExportRequest(c)
		if err != nil || got.SchemaName != "orders" || !reflect.DeepEqual(got.Fields, []string{"name"}) {
			t.Fatalf("ParseExportRequest() = %#v, %v", got, err)
		}
		return nil
	})
	for _, tt := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/pipeline?schemaName=orders&pipelineName=summary"},
		{method: http.MethodPost, path: "/test", body: `{"pipelineStage":{"name":"summary"}}`},
		{method: http.MethodPost, path: "/export", body: `{"schemaName":"orders","fields":["name"]}`},
	} {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		if tt.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if _, err := app.Test(req); err != nil {
			t.Fatalf("app.Test(%q) error = %v", tt.path, err)
		}
	}
}

func TestDynamicRequestParserQueryErrors(t *testing.T) {
	parser := NewDynamicRequestParser(nil)
	container := &models.ContainerModel{Fields: []models.Field{{Name: "age", Type: "int"}}}
	app := fiber.New()
	app.Get("/search", func(c *fiber.Ctx) error {
		if _, err := parser.ParseSearchParams(c); err == nil {
			t.Fatal("ParseSearchParams() error = nil")
		}
		return nil
	})
	app.Get("/filter", func(c *fiber.Ctx) error {
		if _, err := parser.ParseFilterParams(c, container); err == nil {
			t.Fatal("ParseFilterParams() error = nil")
		}
		return nil
	})
	app.Get("/page", func(c *fiber.Ctx) error {
		if _, err := parser.ParsePaginatedItemsParams(c, container); err == nil {
			t.Fatal("ParsePaginatedItemsParams() error = nil")
		}
		return nil
	})
	for _, path := range []string{"/search?sort=age&asc=invalid", "/filter?age=invalid", "/page?page=invalid"} {
		if _, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil)); err != nil {
			t.Fatalf("app.Test(%q) error = %v", path, err)
		}
	}
}

func multipartBody(t *testing.T, fields map[string]string, files map[string][]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
	}
	for fieldName, names := range files {
		for _, name := range names {
			part, err := writer.CreateFormFile(fieldName, name)
			if err != nil {
				t.Fatalf("CreateFormFile() error = %v", err)
			}
			if _, err := part.Write([]byte("content")); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return body, writer.FormDataContentType()
}

func contains(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
