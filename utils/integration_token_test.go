package utils

import (
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestIntegrationTokenHashIsStableAndDoesNotExposeToken(t *testing.T) {
	token := "ati_example_secret"

	first := HashIntegrationToken(token)
	second := HashIntegrationToken(token)

	if first == "" {
		t.Fatal("HashIntegrationToken() returned empty hash")
	}
	if first != second {
		t.Fatalf("HashIntegrationToken() is not stable: %q != %q", first, second)
	}
	if first == token {
		t.Fatal("HashIntegrationToken() returned the raw token")
	}
}

func TestIntegrationPermissionMatchesOnlyExactRequest(t *testing.T) {
	permissions := []models.IntegrationPermission{
		{Kind: models.IntegrationPermissionKindWorkflow, SchemaName: "orders", Name: "confirmDelivery", Method: "POST"},
		{Kind: models.IntegrationPermissionKindDynamicRoute, SchemaName: "products", Route: "GetAllDynamicModelItems", Method: "GET"},
	}

	allowed := models.IntegrationPermission{Kind: models.IntegrationPermissionKindWorkflow, SchemaName: "orders", Name: "confirmDelivery", Method: "post"}
	if !IntegrationPermissionAllowed(permissions, allowed) {
		t.Fatal("IntegrationPermissionAllowed() rejected matching workflow permission")
	}

	denied := models.IntegrationPermission{Kind: models.IntegrationPermissionKindWorkflow, SchemaName: "orders", Name: "cancelDelivery", Method: "POST"}
	if IntegrationPermissionAllowed(permissions, denied) {
		t.Fatal("IntegrationPermissionAllowed() allowed different workflow name")
	}

	wrongMethod := models.IntegrationPermission{Kind: models.IntegrationPermissionKindDynamicRoute, SchemaName: "products", Route: "GetAllDynamicModelItems", Method: "POST"}
	if IntegrationPermissionAllowed(permissions, wrongMethod) {
		t.Fatal("IntegrationPermissionAllowed() allowed different method")
	}
}

func TestIntegrationPermissionForDynamicRoute(t *testing.T) {
	got := IntegrationPermissionForDynamicRoute("ExecuteWorkflow", "orders", "POST", "confirmDelivery", "", "")
	want := models.IntegrationPermission{Kind: models.IntegrationPermissionKindWorkflow, SchemaName: "orders", Name: "confirmDelivery", Method: "POST"}
	if got != want {
		t.Fatalf("workflow permission = %#v, want %#v", got, want)
	}

	got = IntegrationPermissionForDynamicRoute("ExecuteDynamicAPI", "stock", "GET", "", "syncRetailerStock", "")
	want = models.IntegrationPermission{Kind: models.IntegrationPermissionKindAPI, SchemaName: "stock", Name: "syncRetailerStock", Method: "GET"}
	if got != want {
		t.Fatalf("api permission = %#v, want %#v", got, want)
	}

	got = IntegrationPermissionForDynamicRoute("GetPipeline", "orders", "GET", "", "", "summary")
	want = models.IntegrationPermission{Kind: models.IntegrationPermissionKindPipeline, SchemaName: "orders", Name: "summary", Method: "GET"}
	if got != want {
		t.Fatalf("pipeline permission = %#v, want %#v", got, want)
	}

	got = IntegrationPermissionForDynamicRoute("CreateDynamicModelItem", "orders", "POST", "", "", "")
	want = models.IntegrationPermission{Kind: models.IntegrationPermissionKindDynamicRoute, SchemaName: "orders", Route: "CreateDynamicModelItem", Method: "POST"}
	if got != want {
		t.Fatalf("dynamic route permission = %#v, want %#v", got, want)
	}
}

func TestValidateIntegrationCredentialAccess(t *testing.T) {
	tenantID := primitive.NewObjectID()
	projectID := primitive.NewObjectID()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	required := models.IntegrationPermission{Kind: models.IntegrationPermissionKindDynamicRoute, SchemaName: "orders", Route: "CreateDynamicModelItem", Method: "POST"}
	credential := models.IntegrationCredential{
		TenantID:  tenantID,
		ProjectID: projectID,
		ExpiresAt: now.Add(time.Hour),
		Permissions: []models.IntegrationPermission{
			required,
		},
	}

	if err := ValidateIntegrationCredentialAccess(credential, tenantID.Hex(), projectID.Hex(), required, now); err != nil {
		t.Fatalf("ValidateIntegrationCredentialAccess() error = %v", err)
	}

	if err := ValidateIntegrationCredentialAccess(credential, primitive.NewObjectID().Hex(), projectID.Hex(), required, now); err != ErrIntegrationTenantProjectMismatch {
		t.Fatalf("tenant mismatch error = %v, want %v", err, ErrIntegrationTenantProjectMismatch)
	}

	expired := credential
	expired.ExpiresAt = now.Add(-time.Second)
	if err := ValidateIntegrationCredentialAccess(expired, tenantID.Hex(), projectID.Hex(), required, now); err != ErrIntegrationTokenExpired {
		t.Fatalf("expired error = %v, want %v", err, ErrIntegrationTokenExpired)
	}

	revokedAt := now.Add(-time.Minute)
	revoked := credential
	revoked.RevokedAt = &revokedAt
	if err := ValidateIntegrationCredentialAccess(revoked, tenantID.Hex(), projectID.Hex(), required, now); err != ErrIntegrationTokenRevoked {
		t.Fatalf("revoked error = %v, want %v", err, ErrIntegrationTokenRevoked)
	}

	missingPermission := models.IntegrationPermission{Kind: models.IntegrationPermissionKindDynamicRoute, SchemaName: "orders", Route: "DeleteDynamicModelItem", Method: "DELETE"}
	if err := ValidateIntegrationCredentialAccess(credential, tenantID.Hex(), projectID.Hex(), missingPermission, now); err != ErrIntegrationPermissionDenied {
		t.Fatalf("permission error = %v, want %v", err, ErrIntegrationPermissionDenied)
	}
}
