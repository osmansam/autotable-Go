package services

import (
	"reflect"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func accessTestContainer() *models.ContainerModel {
	return &models.ContainerModel{Fields: []models.Field{
		{Name: "ownerId", Type: "objectId"},
		{Name: "status", Type: "string"},
		{Name: "tags", Type: "array"},
	}}
}

func callerAccessContext(userID string) recordAccessContext {
	return recordAccessContext{
		TenantID:     "tenant-1",
		ProjectID:    "project-1",
		Schema:       "orders",
		Operation:    "read",
		IdentityKind: accessIdentityCaller,
		UserID:       userID,
		UserRole:     "member",
		UserRoles:    []string{"member", "buyer"},
		User: map[string]interface{}{
			"_id":      userID,
			"email":    "ada@example.com",
			"password": "must-never-be-copied",
			"profile": map[string]interface{}{
				"departmentId": "dept-1",
			},
		},
	}
}

func TestBuildAccessAuthContextUsesTrustedIdentityValues(t *testing.T) {
	ctx := callerAccessContext("user-1")

	auth := buildAccessAuthContext(ctx)
	if got := authPathValue(auth, "tenant.id"); got != "tenant-1" {
		t.Fatalf("auth tenant.id = %#v, want tenant-1", got)
	}
	if got := authPathValue(auth, "project.id"); got != "project-1" {
		t.Fatalf("auth project.id = %#v, want project-1", got)
	}
	if got := authPathValue(auth, "schema"); got != "orders" {
		t.Fatalf("auth schema = %#v, want orders", got)
	}
	if got := authPathValue(auth, "operation"); got != "read" {
		t.Fatalf("auth operation = %#v, want read", got)
	}
	if got := authPathValue(auth, "identity.kind"); got != "caller" {
		t.Fatalf("auth identity.kind = %#v, want caller", got)
	}
	if got := authPathValue(auth, "user.id"); got != "user-1" {
		t.Fatalf("auth user.id = %#v, want user-1", got)
	}
	if got := authPathValue(auth, "user._id"); got != "user-1" {
		t.Fatalf("auth user._id = %#v, want user-1", got)
	}
	if got := authPathValue(auth, "user.role"); got != "member" {
		t.Fatalf("auth user.role = %#v, want member", got)
	}
	if got := authPathValue(auth, "user.roles"); !reflect.DeepEqual(got, []string{"member", "buyer"}) {
		t.Fatalf("auth user.roles = %#v", got)
	}
	if got := authPathValue(auth, "user.profile.departmentId"); got != "dept-1" {
		t.Fatalf("auth nested user field = %#v, want dept-1", got)
	}
}

func TestBuildAccessAuthContextForSystemHasNoUser(t *testing.T) {
	auth := buildAccessAuthContext(recordAccessContext{
		TenantID: "tenant-1", ProjectID: "project-1", Schema: "orders",
		Operation: "create", IdentityKind: accessIdentitySystem,
	})

	if got := authPathValue(auth, "identity.kind"); got != "system" {
		t.Fatalf("auth identity.kind = %#v, want system", got)
	}
	if got := authPathValue(auth, "user.id"); got != nil {
		t.Fatalf("auth user.id = %#v, want nil", got)
	}
}

func TestRecordAccessRuleOperators(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		left     interface{}
		right    interface{}
		want     bool
	}{
		{name: "eq", operator: "eq", left: "open", right: "open", want: true},
		{name: "ne", operator: "ne", left: "open", right: "closed", want: true},
		{name: "in", operator: "in", left: "open", right: []interface{}{"closed", "open"}, want: true},
		{name: "nin", operator: "nin", left: "open", right: []string{"closed"}, want: true},
		{name: "in miss", operator: "in", left: "open", right: []string{"closed"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recordAccessValuesMatch(tt.operator, tt.left, tt.right); got != tt.want {
				t.Fatalf("recordAccessValuesMatch(%q, %#v, %#v) = %v, want %v", tt.operator, tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestRecordAccessCompileFieldRuleNormalizesObjectID(t *testing.T) {
	userID := primitive.NewObjectID()
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}",
	}}}

	filter, unrestricted, err := compileRecordAccessFilter(accessTestContainer(), policy, callerAccessContext(userID.Hex()))
	if err != nil {
		t.Fatalf("compileRecordAccessFilter() error = %v", err)
	}
	if unrestricted {
		t.Fatal("compileRecordAccessFilter() unrestricted = true, want false")
	}
	want := bson.M{"ownerId": userID}
	if !reflect.DeepEqual(filter, want) {
		t.Fatalf("compileRecordAccessFilter() = %#v, want %#v", filter, want)
	}
}

func TestRecordAccessCompileMultipleFieldRulesUsesOr(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{
		{Field: "status", Operator: "eq", Value: "open"},
		{Field: "status", Operator: "ne", Value: "archived"},
	}}

	filter, unrestricted, err := compileRecordAccessFilter(accessTestContainer(), policy, callerAccessContext("user-1"))
	if err != nil {
		t.Fatalf("compileRecordAccessFilter() error = %v", err)
	}
	if unrestricted {
		t.Fatal("compileRecordAccessFilter() unrestricted = true, want false")
	}
	want := bson.M{"$or": []bson.M{
		{"status": "open"},
		{"status": bson.M{"$ne": "archived"}},
	}}
	if !reflect.DeepEqual(filter, want) {
		t.Fatalf("compileRecordAccessFilter() = %#v, want %#v", filter, want)
	}
}

func TestRecordAccessTrueContextRuleMakesPolicyUnrestricted(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{
		{Field: "status", Operator: "eq", Value: "open"},
		{Context: "{{auth.user.role}}", Operator: "eq", Value: "admin"},
	}}
	ctx := callerAccessContext("user-1")
	ctx.UserRole = "admin"

	filter, unrestricted, err := compileRecordAccessFilter(accessTestContainer(), policy, ctx)
	if err != nil {
		t.Fatalf("compileRecordAccessFilter() error = %v", err)
	}
	if !unrestricted || filter != nil {
		t.Fatalf("compileRecordAccessFilter() = %#v, unrestricted %v; want nil, true", filter, unrestricted)
	}
}

func TestRecordAccessMissingContextKeepsUsableFieldRule(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{
		{Context: "{{auth.user.profile.missing}}", Operator: "eq", Value: "x"},
		{Field: "status", Operator: "eq", Value: "open"},
	}}

	filter, unrestricted, err := compileRecordAccessFilter(accessTestContainer(), policy, recordAccessContext{IdentityKind: accessIdentitySystem})
	if err != nil {
		t.Fatalf("compileRecordAccessFilter() error = %v", err)
	}
	if unrestricted || !reflect.DeepEqual(filter, bson.M{"status": "open"}) {
		t.Fatalf("compileRecordAccessFilter() = %#v, unrestricted %v", filter, unrestricted)
	}
}

func TestRecordAccessUnusableRulesCompileDenyAll(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}",
	}}}

	filter, unrestricted, err := compileRecordAccessFilter(accessTestContainer(), policy, recordAccessContext{IdentityKind: accessIdentitySystem})
	if err != nil {
		t.Fatalf("compileRecordAccessFilter() error = %v", err)
	}
	want := recordAccessDenyAllFilter()
	if unrestricted || !reflect.DeepEqual(filter, want) {
		t.Fatalf("compileRecordAccessFilter() = %#v, unrestricted %v; want %#v, false", filter, unrestricted, want)
	}
}

func TestRecordAccessInvalidObjectIDFailsClosed(t *testing.T) {
	policy := &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{
		Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}",
	}}}

	filter, unrestricted, err := compileRecordAccessFilter(accessTestContainer(), policy, callerAccessContext("not-an-object-id"))
	if err != nil {
		t.Fatalf("compileRecordAccessFilter() error = %v", err)
	}
	if unrestricted || !reflect.DeepEqual(filter, recordAccessDenyAllFilter()) {
		t.Fatalf("compileRecordAccessFilter() = %#v, unrestricted %v; want deny-all", filter, unrestricted)
	}
}

func TestRecordAccessEvaluateAnyRules(t *testing.T) {
	userID := primitive.NewObjectID()
	ctx := callerAccessContext(userID.Hex())
	record := map[string]interface{}{"ownerId": userID, "status": "open"}

	tests := []struct {
		name   string
		policy *models.RecordAccessPolicy
		want   bool
	}{
		{name: "eq", policy: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "status", Operator: "eq", Value: "open"}}}, want: true},
		{name: "ne", policy: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "status", Operator: "ne", Value: "open"}}}, want: false},
		{name: "in", policy: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "status", Operator: "in", Value: []interface{}{"open", "closed"}}}}, want: true},
		{name: "nin", policy: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "status", Operator: "nin", Value: []interface{}{"open"}}}}, want: false},
		{name: "object id", policy: &models.RecordAccessPolicy{Any: []models.RecordAccessRule{{Field: "ownerId", Operator: "eq", Value: "{{auth.user.id}}"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := recordAccessAllows(accessTestContainer(), tt.policy, ctx, record)
			if err != nil {
				t.Fatalf("recordAccessAllows() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("recordAccessAllows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordAccessAssignmentsOverwriteSpoofedValuesWithoutMutatingInput(t *testing.T) {
	userID := primitive.NewObjectID()
	input := map[string]interface{}{"ownerId": primitive.NewObjectID(), "status": "open"}
	policy := &models.RecordAccessPolicy{Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"}}

	got, err := applyRecordAccessAssignments(accessTestContainer(), policy, callerAccessContext(userID.Hex()), input)
	if err != nil {
		t.Fatalf("applyRecordAccessAssignments() error = %v", err)
	}
	if got["ownerId"] != userID {
		t.Fatalf("assigned ownerId = %#v, want %s", got["ownerId"], userID.Hex())
	}
	if reflect.DeepEqual(input["ownerId"], userID) {
		t.Fatal("applyRecordAccessAssignments() mutated caller input")
	}
}

func TestRecordAccessAssignmentMissingContextReturnsError(t *testing.T) {
	policy := &models.RecordAccessPolicy{Assign: map[string]interface{}{"ownerId": "{{auth.user.id}}"}}

	if _, err := applyRecordAccessAssignments(accessTestContainer(), policy, recordAccessContext{IdentityKind: accessIdentitySystem}, map[string]interface{}{}); err == nil {
		t.Fatal("applyRecordAccessAssignments() error = nil, want unresolved context error")
	}
}

func TestRecordAccessCombineFiltersPreservesEveryConstraint(t *testing.T) {
	got := combineRecordAccessFilters(
		bson.M{"status": "open"},
		bson.M{"$or": []bson.M{{"ownerId": "one"}, {"ownerId": "two"}}},
		bson.M{"tenantId": "tenant-1"},
	)
	want := bson.M{"$and": []bson.M{
		{"status": "open"},
		{"$or": []bson.M{{"ownerId": "one"}, {"ownerId": "two"}}},
		{"tenantId": "tenant-1"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("combineRecordAccessFilters() = %#v, want %#v", got, want)
	}
}

func authPathValue(auth map[string]interface{}, path string) interface{} {
	current := interface{}(auth)
	for _, part := range splitAccessPath(path) {
		mapped, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = mapped[part]
		if !ok {
			return nil
		}
	}
	return current
}
