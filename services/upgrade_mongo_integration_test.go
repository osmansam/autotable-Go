//go:build integration

package services

import (
	"encoding/json"
	"github.com/osmansam/autotableGo/internal/upgrade"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func legacyUpgradeFixture(t *testing.T) (*workflowMongoFixture, upgrade.Scope, primitive.ObjectID) {
	t.Helper()
	f := newWorkflowMongoFixture(t)
	scope := upgrade.Scope{TenantID: "111111111111111111111111", ProjectID: "222222222222222222222222", Schema: "auth"}
	prefix := "tenant_" + scope.TenantID + "_project_" + scope.ProjectID + "_"
	_, err := f.db.Collection(prefix+"containers").InsertOne(f.ctx, models.ContainerModel{ID: primitive.NewObjectID(), SchemaName: "auth", IsAuthContainer: true, IsRegisterActive: true, Fields: []models.Field{{Name: "email", Type: "string", IsLoginCredential: true}}})
	if err != nil {
		t.Fatal(err)
	}
	user := primitive.NewObjectID()
	if _, err := f.db.Collection(prefix+"auth").InsertOne(f.ctx, bson.M{"_id": user, "email": "member@example.com", "privateNote": "never-print-account-secret"}); err != nil {
		t.Fatal(err)
	}
	return f, scope, user
}
func TestWorkflowMongoUpgradeAuditReadsOnlyAndRedactsAccountSecrets(t *testing.T) {
	f, scope, user := legacyUpgradeFixture(t)
	prefix := "tenant_" + scope.TenantID + "_project_" + scope.ProjectID + "_"
	for range 2 {
		if _, err := f.db.Collection("auth_sessions").InsertOne(f.ctx, models.AuthSession{UserID: user.Hex(), TenantID: scope.TenantID, ProjectID: scope.ProjectID, Scope: "project", FamilyID: "never-print-family-secret", CurrentJTIHash: "never-print-token-hash", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := f.db.Collection("project_translations").InsertOne(f.ctx, bson.M{"tenantId": scope.TenantID, "projectId": scope.ProjectID, "locale": "tr", "translationKey": "title", "translatedText": "never-print-translation"}); err != nil {
			t.Fatal(err)
		}
	}
	var before, after bson.Raw
	if err := f.db.Collection(prefix+"containers").FindOne(f.ctx, bson.M{"schemaName": "auth"}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	report, err := upgrade.Audit(f.ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	if !report.TransactionCapable || len(report.Auth) != 1 || report.Auth[0].PasswordLoginConfigured || report.Auth[0].AccountsNeedingEnrollment != 1 || report.LegacyRefreshSessions != 2 || report.DuplicateSessionFamilies != 1 || report.DuplicateTranslationIdentities != 1 {
		t.Fatalf("unexpected audit: %#v", report)
	}
	output, _ := json.Marshal(report)
	if strings.Contains(string(output), "never-print") {
		t.Fatal("audit leaked a user value, token hash or family id")
	}
	if err := f.db.Collection(prefix+"containers").FindOne(f.ctx, bson.M{"schemaName": "auth"}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read-only audit changed metadata")
	}
}
func TestWorkflowMongoUpgradeEnrollmentIsScopedAndDoesNotInventPasswords(t *testing.T) {
	f, scope, user := legacyUpgradeFixture(t)
	prefix := "tenant_" + scope.TenantID + "_project_" + scope.ProjectID + "_"
	changed, err := upgrade.AddPasswordField(f.ctx, f.db, scope)
	if err != nil || !changed {
		t.Fatalf("migration changed=%v err=%v", changed, err)
	}
	if changed, err := upgrade.AddPasswordField(f.ctx, f.db, scope); err != nil || changed {
		t.Fatalf("migration is not idempotent: %v %v", changed, err)
	}
	var before bson.M
	if err := f.db.Collection(prefix+"auth").FindOne(f.ctx, bson.M{"_id": user}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	if before["password"] != nil {
		t.Fatal("metadata upgrade invented a user password")
	}
	report, err := upgrade.Audit(f.ctx, f.db)
	if err != nil || !report.Auth[0].PasswordLoginConfigured || report.Auth[0].AccountsNeedingEnrollment != 1 || report.Auth[0].PublicRegistrationEnabled {
		t.Fatalf("metadata upgrade falsely completed enrollment: %#v %v", report, err)
	}
	for _, project := range []string{scope.ProjectID, "333333333333333333333333"} {
		if _, err := f.db.Collection("auth_sessions").InsertOne(f.ctx, models.AuthSession{UserID: user.Hex(), TenantID: scope.TenantID, ProjectID: project, Scope: "project", FamilyID: project, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := upgrade.SetPassword(f.ctx, f.db, scope, user.Hex(), []byte("verified-owner-password")); err != nil {
		t.Fatal(err)
	}
	var stored bson.M
	if err := f.db.Collection(prefix+"auth").FindOne(f.ctx, bson.M{"_id": user}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	hash, ok := stored["password"].(string)
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte("verified-owner-password")) != nil {
		t.Fatal("enrollment did not store a usable password hash")
	}
	if stored["privateNote"] != "never-print-account-secret" {
		t.Fatal("enrollment overwrote unrelated user fields")
	}
	if n, err := f.db.Collection("auth_sessions").CountDocuments(f.ctx, bson.M{"projectId": scope.ProjectID, "revokedAt": bson.M{"$exists": true}}); err != nil || n != 1 {
		t.Fatal("enrollment did not revoke selected project sessions")
	}
	if n, err := f.db.Collection("auth_sessions").CountDocuments(f.ctx, bson.M{"projectId": "333333333333333333333333", "revokedAt": bson.M{"$exists": false}}); err != nil || n != 1 {
		t.Fatal("enrollment revoked another project's session")
	}
	report, err = upgrade.Audit(f.ctx, f.db)
	if err != nil || report.Auth[0].AccountsNeedingEnrollment != 0 {
		t.Fatalf("enrollment audit=%#v %v", report, err)
	}
}
func TestWorkflowMongoUpgradeRevocationFailureRollsBackPassword(t *testing.T) {
	f, scope, user := legacyUpgradeFixture(t)
	if _, err := upgrade.AddPasswordField(f.ctx, f.db, scope); err != nil {
		t.Fatal(err)
	}
	// A view deliberately rejects writes, after the user update has executed.
	if err := f.db.CreateView(f.ctx, "auth_sessions", "read_only_sessions", mongo.Pipeline{}); err != nil {
		t.Fatal(err)
	}
	if err := upgrade.SetPassword(f.ctx, f.db, scope, user.Hex(), []byte("verified-owner-password")); err == nil {
		t.Fatal("session revocation failure was ignored")
	}
	prefix := "tenant_" + scope.TenantID + "_project_" + scope.ProjectID + "_"
	var stored bson.M
	if err := f.db.Collection(prefix+"auth").FindOne(f.ctx, bson.M{"_id": user}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored["password"] != nil {
		t.Fatal("password changed even though session revocation rolled back")
	}
}
