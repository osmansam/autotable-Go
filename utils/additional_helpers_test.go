package utils

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCreateDynamicPipeline(t *testing.T) {
	got, err := CreateDynamicPipeline(models.PipelineStage{PipelineJSON: `[{"$match":{"status":"open"}},{"$limit":2}]`})
	if err != nil || len(got) != 2 || got[0][0].Key != "$match" || got[1][0].Key != "$limit" {
		t.Fatalf("CreateDynamicPipeline() = %#v, %v", got, err)
	}
	if _, err := CreateDynamicPipeline(models.PipelineStage{PipelineJSON: `invalid`}); err == nil {
		t.Fatal("CreateDynamicPipeline(invalid) error = nil")
	}
}

func TestReplacePlaceholdersWithQueryParams(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		got := ReplacePlaceholdersWithQueryParams(`[{"$match":{"age":"{{age}}","name":"{{name}}","missing":"{{missing}}"}}]`, c)
		want := `[{"$match":{"age":42,"name":"Ada","missing":"{{missing}}"}}]`
		if got != want {
			t.Fatalf("ReplacePlaceholdersWithQueryParams() = %q, want %q", got, want)
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/?age=42&name=Ada", nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestReplacePlaceholdersWithQueryParamsTrimsPlaceholderNames(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		got := ReplacePlaceholdersWithQueryParams(`[{"$match":{"period":"{{ filter }}"}}]`, c)
		want := `[{"$match":{"period":"07-2026"}}]`
		if got != want {
			t.Fatalf("ReplacePlaceholdersWithQueryParams() = %q, want %q", got, want)
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/?filter=07-2026", nil)); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestReplacePlaceholdersWithProjectContext(t *testing.T) {
	got := ReplacePlaceholdersWithProjectContext(
		`[{"$lookup":{"from":"{{projectCollection:product}}"}},{"$match":{"tenant":"{{tenantID}}","project":"{{projectID}}"}}]`,
		"tenant1",
		"project1",
	)
	want := `[{"$lookup":{"from":"tenant_tenant1_project_project1_product"}},{"$match":{"tenant":"tenant1","project":"project1"}}]`
	if got != want {
		t.Fatalf("ReplacePlaceholdersWithProjectContext() = %q, want %q", got, want)
	}
}

func TestBuildFilterFromQuery(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "age", Type: "int"},
		{Name: "email", Type: "string"},
		{Name: "password", Type: "string", IsHashed: true},
	}}
	tests := []struct {
		name    string
		path    string
		want    bson.M
		wantErr bool
	}{
		{name: "single value", path: "/?age=42&email=a%40example.com&password=hidden", want: bson.M{"age": 42, "email": "a@example.com"}},
		{name: "multiple simple values", path: "/?age=1&age=2", want: bson.M{"age": bson.M{"$in": []interface{}{1, 2}}}},
		{name: "range", path: "/?age=gte-10&age=lt-20", want: bson.M{"age": bson.M{"$gte": 10, "$lt": 20}}},
		{name: "wraparound range becomes or", path: "/?age=gte-20&age=lte-10", want: bson.M{"$or": []bson.M{{"age": bson.M{"$lte": 10}}, {"age": bson.M{"$gte": 20}}}}},
		{name: "invalid int", path: "/?age=bad", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				got, err := BuildFilterFromQuery(c, container)
				if (err != nil) != tt.wantErr {
					t.Fatalf("BuildFilterFromQuery() error = %v, wantErr %v", err, tt.wantErr)
				}
				if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("BuildFilterFromQuery() = %#v, want %#v", got, tt.want)
				}
				return nil
			})
			if _, err := app.Test(httptest.NewRequest(http.MethodGet, tt.path, nil)); err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
		})
	}
}

func TestBuildSelectionFilterFromQuery(t *testing.T) {
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(c)

	c.Request().URI().SetQueryString("schemaName=products&fieldName=name&filter.active=true&filter.category=featured")
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "schemaName", Type: "string"},
		{Name: "active", Type: "boolean"},
		{Name: "category", Type: "string"},
	}}

	got, err := BuildSelectionFilterFromQuery(c, container)
	if err != nil {
		t.Fatalf("BuildSelectionFilterFromQuery() error = %v", err)
	}

	want := bson.M{"active": true, "category": "featured"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildSelectionFilterFromQuery() = %#v, want %#v", got, want)
	}
}

func TestIdempotencyKeyAndHash(t *testing.T) {
	var firstKey, secondKey, firstHash, secondHash string
	app := fiber.New()
	app.Post("/items", func(c *fiber.Ctx) error {
		key := BuildIdempotencyRedisKey("tenant", "project", "", c)
		hash := BuildIdempotencyRequestHash(c)
		if firstKey == "" {
			firstKey, firstHash = key, hash
		} else {
			secondKey, secondHash = key, hash
		}
		return nil
	})
	for _, path := range []string{"/items?b=2&a=1", "/items?a=1&b=2"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"name":"Ada"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(idempotencyHeaderName, " key ")
		if _, err := app.Test(req); err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
	}
	if firstKey == "" || firstKey != secondKey || firstHash != secondHash {
		t.Fatalf("idempotency values = (%q, %q), (%q, %q)", firstKey, firstHash, secondKey, secondHash)
	}
}

func TestExecuteAPIRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer server.Close()

	got, err := ExecuteApiRequest(context.Background(), http.MethodPost, server.URL, map[string]interface{}{"name": "Ada"})
	if err != nil || string(got) != `{"name":"Ada"}` {
		t.Fatalf("ExecuteApiRequest() = %q, %v", got, err)
	}

	multiMegabyteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 2<<20))
	}))
	defer multiMegabyteServer.Close()
	got, err = ExecuteApiRequest(context.Background(), http.MethodGet, multiMegabyteServer.URL, nil)
	if err != nil || len(got) != 2<<20 {
		t.Fatalf("ExecuteApiRequest(2 MiB response) bytes = %d, error = %v", len(got), err)
	}

	largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxExecuteAPIResponseBytes+1))
	}))
	defer largeServer.Close()
	if _, err := ExecuteApiRequest(context.Background(), http.MethodGet, largeServer.URL, nil); err == nil {
		t.Fatal("ExecuteApiRequest(large response) error = nil")
	}
}

func TestExecuteAPIRequestWithStatusReturnsStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	}))
	defer server.Close()

	got, status, err := ExecuteApiRequestWithStatus(context.Background(), http.MethodPost, server.URL, map[string]interface{}{"name": "Ada"})
	if err != nil {
		t.Fatalf("ExecuteApiRequestWithStatus() error = %v", err)
	}
	if status != http.StatusBadRequest || string(got) != `{"message":"bad request"}` {
		t.Fatalf("ExecuteApiRequestWithStatus() = (%q, %d), want bad request body and %d", got, status, http.StatusBadRequest)
	}
}

func TestExecuteAPIRequestWithStatusAndHeadersProtectsAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer protected" {
			t.Fatalf("Authorization = %q, want protected header", got)
		}
		if got := r.Header.Get("X-Custom"); got != "allowed" {
			t.Fatalf("X-Custom = %q, want user header", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	body, status, err := ExecuteApiRequestWithStatusAndHeaders(
		context.Background(),
		http.MethodGet,
		server.URL,
		nil,
		map[string]string{"Authorization": "Bearer attacker", "X-Custom": "allowed"},
		map[string]string{"Authorization": "Bearer protected"},
		nil,
	)
	if err != nil {
		t.Fatalf("ExecuteApiRequestWithStatusAndHeaders() error = %v", err)
	}
	if status != http.StatusOK || string(body) != `{"ok":true}` {
		t.Fatalf("ExecuteApiRequestWithStatusAndHeaders() = (%q, %d), want ok", body, status)
	}
}

func TestExecuteAPIRequestWithStatusAndHeadersBlocksRedirectToDisallowedHost(t *testing.T) {
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://disallowed.example.invalid/secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer allowed.Close()

	allowedURL, err := url.Parse(allowed.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	_, _, err = ExecuteApiRequestWithStatusAndHeaders(
		context.Background(),
		http.MethodGet,
		allowed.URL,
		nil,
		nil,
		nil,
		func(host string) bool { return host == allowedURL.Hostname() },
	)
	if err == nil {
		t.Fatal("ExecuteApiRequestWithStatusAndHeaders(redirect) error = nil")
	}
	if strings.Contains(err.Error(), "disallowed.example.invalid") {
		t.Fatalf("redirect error leaked target host: %v", err)
	}
}

func TestSendResponseAndSendErrorResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/success", func(c *fiber.Ctx) error {
		return SendResponse(c, http.StatusCreated, "created", map[string]string{"id": "1"})
	})
	app.Get("/error", func(c *fiber.Ctx) error { return SendErrorResponse(c, context.DeadlineExceeded, "failed") })

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/success", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	var body responses.GeneralResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Status != http.StatusCreated || body.Message != "created" {
		t.Fatalf("response = %#v, %v", body, err)
	}
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/error", nil))
	if err != nil || resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("error response = %#v, %v", resp, err)
	}
}

func TestValidateStructAndCreditCard(t *testing.T) {
	type input struct {
		Name string `validate:"required"`
	}
	if err := ValidateStruct(input{Name: "Ada"}); err != nil {
		t.Fatalf("ValidateStruct() error = %v", err)
	}
	if err := ValidateStruct(input{}); err == nil {
		t.Fatal("ValidateStruct(empty) error = nil")
	}
	if !isValidCreditCard("4111-1111-1111-1111") || isValidCreditCard("4111") || isValidCreditCard("4111-1111-1111-abcd") {
		t.Fatal("isValidCreditCard() returned incorrect result")
	}
}

func TestExtractObjectIDs(t *testing.T) {
	first := primitive.NewObjectID()
	second := primitive.NewObjectID()
	tests := []struct {
		name  string
		value interface{}
		want  []primitive.ObjectID
	}{
		{name: "primitive array", value: primitive.A{first, second.Hex(), "invalid"}, want: []primitive.ObjectID{first, second}},
		{name: "interface array", value: []interface{}{first, second.Hex()}, want: []primitive.ObjectID{first, second}},
		{name: "string array", value: []string{first.Hex(), "invalid"}, want: []primitive.ObjectID{first}},
		{name: "object id array", value: []primitive.ObjectID{first}, want: []primitive.ObjectID{first}},
		{name: "single object id", value: first, want: []primitive.ObjectID{first}},
		{name: "single string", value: second.Hex(), want: []primitive.ObjectID{second}},
		{name: "unsupported", value: 1, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractObjectIDs(tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("extractObjectIDs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestIndexErrorHelpers(t *testing.T) {
	if isIndexExistsError(nil) {
		t.Fatal("isIndexExistsError(nil) = true")
	}
	if !isIndexExistsError(assertError("all indexes already exist")) {
		t.Fatal("isIndexExistsError() = false")
	}
	if isIndexExistsError(assertError("index conflict")) {
		t.Fatal("isIndexExistsError(conflict) = true")
	}
	if !containsAny("abcdef", []string{"xy", "cd"}) || containsAny("abcdef", []string{"xy"}) {
		t.Fatal("containsAny() returned incorrect result")
	}
}

func TestBuildSearchWithReferencesDirectFields(t *testing.T) {
	if _, err := BuildSearchWithReferences(context.Background(), nil, "Ada"); err == nil {
		t.Fatal("BuildSearchWithReferences(nil) error = nil")
	}
	if got, err := BuildSearchWithReferences(context.Background(), &models.ContainerModel{}, ""); err != nil || got != nil {
		t.Fatalf("BuildSearchWithReferences(empty) = %#v, %v", got, err)
	}
	got, err := BuildSearchWithReferences(context.Background(), &models.ContainerModel{
		Fields: []models.Field{{Name: "name", Type: "string", IsSearchable: true}},
	}, "Ada")
	if err != nil || len(got) != 1 {
		t.Fatalf("BuildSearchWithReferences() = %#v, %v", got, err)
	}
}

func TestIdempotencyNoKeyAndResponseHelpers(t *testing.T) {
	if result, err := BeginIdempotentRequest(context.Background(), "", "hash"); err != nil || result.Status != IdempotencyOwned {
		t.Fatalf("BeginIdempotentRequest() = %#v, %v", result, err)
	}
	if result, err := GetIdempotentResult(context.Background(), "", "hash"); err != nil || result != nil {
		t.Fatalf("GetIdempotentResult() = %#v, %v", result, err)
	}
	if err := StoreIdempotentResult(context.Background(), "", IdempotencyResult{}); err != nil {
		t.Fatalf("StoreIdempotentResult() error = %v", err)
	}
	if result, err := WaitForIdempotentResult(context.Background(), "", "hash"); err != nil || result != nil {
		t.Fatalf("WaitForIdempotentResult() = %#v, %v", result, err)
	}

	app := fiber.New()
	app.Get("/mismatch", SendIdempotencyRequestMismatch)
	app.Get("/progress", SendIdempotencyInProgress)
	for path, want := range map[string]int{"/mismatch": http.StatusBadRequest, "/progress": http.StatusConflict} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil || resp.StatusCode != want {
			t.Fatalf("%s status = %v, error = %v", path, resp, err)
		}
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
