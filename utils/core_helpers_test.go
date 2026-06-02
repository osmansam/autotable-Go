package utils

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func TestFilterFieldsByRole(t *testing.T) {
	fields := []models.Field{
		{Name: "public"},
		{Name: "admin", IsAuthorized: true, AuthorizeRole: []string{"admin"}},
		{Name: "profile", Children: []models.Field{{Name: "visible"}, {Name: "secret", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}},
	}

	got := FilterFieldsByRole(fields, "viewer")
	want := []models.Field{
		{Name: "public"},
		{Name: "profile", Children: []models.Field{{Name: "visible"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterFieldsByRole() = %#v, want %#v", got, want)
	}
}

func TestFilterDocumentFields(t *testing.T) {
	fields := []models.Field{
		{Name: "public"},
		{Name: "private", IsAuthorized: true, AuthorizeRole: []string{"admin"}},
		{Name: "profile", Children: []models.Field{{Name: "name"}, {Name: "secret", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}},
	}
	doc := bson.M{
		"_id":     "id",
		"public":  "visible",
		"private": "hidden",
		"unknown": "discarded",
		"profile": bson.M{"name": "Ada", "secret": "hidden", "unknown": "discarded"},
	}

	got := FilterDocumentFields(doc, fields, "viewer")
	want := bson.M{"_id": "id", "public": "visible", "profile": bson.M{"name": "Ada"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterDocumentFields() = %#v, want %#v", got, want)
	}

	relaxed := FilterDocumentFieldsRelaxed(doc, fields, "viewer")
	wantRelaxed := bson.M{"_id": "id", "public": "visible", "unknown": "discarded", "profile": bson.M{"name": "Ada", "unknown": "discarded"}}
	if !reflect.DeepEqual(relaxed, wantRelaxed) {
		t.Fatalf("FilterDocumentFieldsRelaxed() = %#v, want %#v", relaxed, wantRelaxed)
	}
}

func TestFilterDocuments(t *testing.T) {
	docs := []map[string]interface{}{{"_id": "id", "public": true, "secret": true}}
	fields := []models.Field{{Name: "public"}, {Name: "secret", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}
	if got := FilterDocuments(docs, fields, "viewer"); !reflect.DeepEqual(got, []map[string]interface{}{{"_id": "id", "public": true}}) {
		t.Fatalf("FilterDocuments() = %#v", got)
	}
	if got := FilterDocumentsRelaxed(docs, fields, "viewer"); !reflect.DeepEqual(got, []map[string]interface{}{{"_id": "id", "public": true}}) {
		t.Fatalf("FilterDocumentsRelaxed() = %#v", got)
	}
}

func TestRelaxedFieldFilteringNestedMap(t *testing.T) {
	fields := []models.Field{
		{Name: "profile", Children: []models.Field{
			{Name: "visible"},
			{Name: "secret", IsAuthorized: true, AuthorizeRole: []string{"admin"}},
		}},
	}
	doc := bson.M{"profile": map[string]interface{}{"visible": "yes", "secret": "hidden", "unknown": "kept"}}
	got := FilterDocumentFieldsRelaxed(doc, fields, "viewer")
	want := bson.M{"profile": bson.M{"visible": "yes", "unknown": "kept"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterDocumentFieldsRelaxed() = %#v, want %#v", got, want)
	}
}

func TestGetRowAccessFilter(t *testing.T) {
	tests := []struct {
		name      string
		container *models.ContainerModel
		role      string
		user      map[string]interface{}
		want      bson.M
		wantErr   bool
	}{
		{name: "no rules", container: &models.ContainerModel{}},
		{
			name:      "no matching role",
			container: &models.ContainerModel{RowAccess: &models.RowAccessRule{Conditions: []models.Condition{{Roles: []string{"admin"}}}}},
			role:      "viewer",
		},
		{
			name: "placeholder falls back to underscore id",
			container: &models.ContainerModel{RowAccess: &models.RowAccessRule{Conditions: []models.Condition{{
				Field: "ownerId", Operator: "=", Value: "{{user.id}}", Roles: []string{"viewer"},
			}}}},
			role: "viewer",
			user: map[string]interface{}{"_id": "user-id"},
			want: bson.M{"ownerId": "user-id"},
		},
		{
			name: "multiple matching rules combine with and",
			container: &models.ContainerModel{RowAccess: &models.RowAccessRule{Conditions: []models.Condition{
				{Field: "amount", Operator: ">", Value: 10, Roles: []string{"viewer"}},
				{Field: "state", Operator: "!=", Value: "closed", Roles: []string{"viewer"}},
			}}},
			role: "viewer",
			want: bson.M{"$and": []bson.M{{"amount": bson.M{"$gt": 10}}, {"state": bson.M{"$ne": "closed"}}}},
		},
		{
			name: "unresolved placeholder",
			container: &models.ContainerModel{RowAccess: &models.RowAccessRule{Conditions: []models.Condition{{
				Field: "ownerId", Operator: "=", Value: "{{user.id}}", Roles: []string{"viewer"},
			}}}},
			role:    "viewer",
			user:    map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetRowAccessFilter(tt.container, tt.role, tt.user)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetRowAccessFilter() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("GetRowAccessFilter() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBuildConditionFilterOperators(t *testing.T) {
	tests := []struct {
		operator string
		want     bson.M
	}{
		{operator: "=", want: bson.M{"field": 1}},
		{operator: "!=", want: bson.M{"field": bson.M{"$ne": 1}}},
		{operator: ">", want: bson.M{"field": bson.M{"$gt": 1}}},
		{operator: ">=", want: bson.M{"field": bson.M{"$gte": 1}}},
		{operator: "<", want: bson.M{"field": bson.M{"$lt": 1}}},
		{operator: "<=", want: bson.M{"field": bson.M{"$lte": 1}}},
		{operator: "in", want: bson.M{"field": bson.M{"$in": 1}}},
		{operator: "nin", want: bson.M{"field": bson.M{"$nin": 1}}},
		{operator: "unknown", want: bson.M{"field": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.operator, func(t *testing.T) {
			got, err := buildConditionFilter(models.Condition{Field: "field", Operator: tt.operator, Value: 1}, nil)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildConditionFilter() = %#v, %v; want %#v", got, err, tt.want)
			}
		})
	}
}

func TestGenerateRedisKeys(t *testing.T) {
	container := &models.ContainerModel{
		Redis:            models.Redis{IsRedisCached: true},
		Pipelines:        []models.PipelineStage{{Name: "summary", IsRedisCached: true}},
		DynamicFunctions: []models.DynamicFunction{{Name: "total", IsRedisCached: true}},
		DynamicApis:      []models.DynamicApiModel{{Name: "status", IsRedisCached: true}},
	}
	tests := []struct {
		name string
		run  func() (string, bool)
		want string
	}{
		{name: "route scoped", run: func() (string, bool) {
			return GenerateRedisKey("GetDynamicModelItem", "t", "p", "orders", container, "42")
		}, want: "tenant_t_project_p_schema_orders_route_GetDynamicModelItem_42"},
		{name: "route legacy", run: func() (string, bool) { return GenerateRedisKey("GetAllDynamicModelItems", "", "", "orders", container) }, want: "schema_orders_route_GetAllDynamicModelItems"},
		{name: "pipeline", run: func() (string, bool) { return GeneratePipelineRedisKey("t", "p", "orders", "summary", container) }, want: "tenant_t_project_p_pipeline_summary_schema_orders"},
		{name: "function", run: func() (string, bool) { return GenerateDynamicFunctionRedisKey("t", "p", "orders", "total", container) }, want: "tenant_t_project_p_function_total_schema_orders"},
		{name: "api", run: func() (string, bool) { return GenerateDynamicApiRedisKey("t", "p", "orders", "status", container) }, want: "tenant_t_project_p_api_status_schema_orders"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, cache := tt.run()
			if !cache || got != tt.want {
				t.Fatalf("key = %q, cache = %v; want %q, true", got, cache, tt.want)
			}
		})
	}
	if key, cache := GenerateRedisKey("unsupported", "t", "p", "orders", container); cache || key != "" {
		t.Fatalf("unsupported route = %q, %v; want empty, false", key, cache)
	}
}

func TestCacheKeyHelpers(t *testing.T) {
	if got := BuildSchemaCacheVersionKey("t", "p", "orders"); got != "tenant:t:project:p:schema:orders:version" {
		t.Fatalf("BuildSchemaCacheVersionKey() = %q", got)
	}
	if got := BuildVersionedCacheKey("t", "p", "orders", 3, "list", "hash"); got != "tenant:t:project:p:schema:orders:v3:route:list:query:hash" {
		t.Fatalf("BuildVersionedCacheKey() = %q", got)
	}
	if HashCacheQuery("same") != HashCacheQuery("same") || HashCacheQuery("same") == HashCacheQuery("different") {
		t.Fatal("HashCacheQuery() must be deterministic and distinguish inputs")
	}
	if got := BuildCacheFillLockKey("key"); got != "lock:cache-fill:"+HashCacheQuery("key") {
		t.Fatalf("BuildCacheFillLockKey() = %q", got)
	}
}

func TestProjectCollectionName(t *testing.T) {
	if got := GetProjectCollectionName("tenant", "project", "orders"); got != "tenant_tenant_project_project_orders" {
		t.Fatalf("GetProjectCollectionName() = %q", got)
	}
}

func TestPointerToString(t *testing.T) {
	if got := PointerToString("value"); got == nil || *got != "value" {
		t.Fatalf("PointerToString() = %#v", got)
	}
}

func TestSplitCachedValue(t *testing.T) {
	tests := map[string][]string{
		"":          {},
		"tenant":    {"tenant"},
		"t|p":       {"t", "p"},
		"t|p|extra": {"t", "p", "extra"},
		"t|":        {"t"},
	}
	for input, want := range tests {
		if got := splitCachedValue(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("splitCachedValue(%q) = %#v, want %#v", input, got, want)
		}
	}
}

func TestParsePagerAndSort(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantPager Pager
		wantSort  bson.D
		wantErr   bool
	}{
		{name: "no params", path: "/", wantPager: Pager{}},
		{name: "page and limit", path: "/?page=3&limit=5&sort=name&asc=false", wantPager: Pager{Enabled: true, Page: 3, Limit: 5, Skip: 10}, wantSort: bson.D{{Key: "name", Value: int32(-1)}}},
		{name: "invalid page", path: "/?page=bad", wantErr: true},
		{name: "invalid sort", path: "/?sort=name&asc=bad", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				pager, pagerErr := ParsePager(c)
				sort, sortErr := ParseSort(c)
				if (pagerErr != nil || sortErr != nil) != tt.wantErr {
					t.Fatalf("errors = (%v, %v), wantErr %v", pagerErr, sortErr, tt.wantErr)
				}
				if !tt.wantErr && (!reflect.DeepEqual(pager, tt.wantPager) || !reflect.DeepEqual(sort, tt.wantSort)) {
					t.Fatalf("ParsePager/ParseSort = (%#v, %#v), want (%#v, %#v)", pager, sort, tt.wantPager, tt.wantSort)
				}
				return nil
			})
			if _, err := app.Test(newRequest(tt.path)); err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
		})
	}
}

func TestBuildFindOptions(t *testing.T) {
	sort := bson.D{{Key: "name", Value: int32(-1)}}
	opts := BuildFindOptions(sort, Pager{Enabled: true, Skip: 10, Limit: 5})
	if !reflect.DeepEqual(opts.Sort, sort) || opts.Skip == nil || *opts.Skip != 10 || opts.Limit == nil || *opts.Limit != 5 {
		t.Fatalf("BuildFindOptions() = %#v", opts)
	}
	if opts.MaxTime == nil || *opts.MaxTime != 10*time.Second {
		t.Fatalf("MaxTime = %#v, want 10s", opts.MaxTime)
	}
}

func TestStripHashed(t *testing.T) {
	items := []map[string]interface{}{{"name": "Ada", "password": "hashed"}}
	StripHashed([]models.Field{{Name: "password", IsHashed: true}}, items)
	if !reflect.DeepEqual(items, []map[string]interface{}{{"name": "Ada"}}) {
		t.Fatalf("StripHashed() = %#v", items)
	}
}

func TestRequestContextWithTimeout(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		ctx, cancel := RequestContextWithTimeout(c, 50*time.Millisecond)
		defer cancel()
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("RequestContextWithTimeout() context has no deadline")
		}
		return nil
	})
	if _, err := app.Test(newRequest("/")); err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 || boolToInt(false) != 0 {
		t.Fatal("boolToInt() returned incorrect value")
	}
}

func TestBuildFilterInternalHelpers(t *testing.T) {
	if got := splitQuery("a=1&&b=2&"); !reflect.DeepEqual(got, []string{"a=1", "b=2"}) {
		t.Fatalf("splitQuery() = %#v", got)
	}
	tests := []struct {
		value interface{}
		want  float64
	}{
		{value: int(1), want: 1},
		{value: int32(2), want: 2},
		{value: int64(3), want: 3},
		{value: float32(4), want: 4},
		{value: float64(5), want: 5},
		{value: "unsupported", want: 0},
	}
	for _, tt := range tests {
		if got := toFloat64(tt.value); got != tt.want {
			t.Fatalf("toFloat64(%T) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func newRequest(path string) *http.Request {
	return httptest.NewRequest(http.MethodGet, path, nil)
}
