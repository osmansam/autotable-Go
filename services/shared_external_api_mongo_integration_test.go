//go:build integration

package services

import (
	"context"
	"encoding/base64"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkflowMongoSharedExternalAPICredentialLiveAuthorization(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	tenant, otherTenant := primitive.NewObjectID(), primitive.NewObjectID()
	a, b, c := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	for _, id := range []primitive.ObjectID{a, b, c} {
		if _, err := f.db.Collection("projects").InsertOne(f.ctx, models.Project{ID: id, TenantID: tenant, IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	t.Setenv("EXTERNAL_API_CREDENTIAL_KEY", key)
	encrypted, err := utils.EncryptExternalSecret("task-only-provider-key", key)
	if err != nil {
		t.Fatal(err)
	}
	credential := models.ExternalAPICredential{ID: primitive.NewObjectID(), TenantID: tenant, ProjectID: a, Name: "Local provider", AuthType: "bearer", EncryptedSecret: encrypted, AllowedDomains: []string{"8.8.8.8"}, CreatedAt: time.Now()}
	if _, err = f.repo.InsertExternalAPICredential(f.ctx, credential); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	service := workflowHTTPAuditUseServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer task-only-provider-key" {
			t.Errorf("missing protected provider key")
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	service.repository = f.repo
	step := workflowHTTPAuditStep(map[string]interface{}{"url": "http://8.8.8.8/endpoint", "credentialId": credential.ID.Hex()})
	run := func(tenantID, projectID primitive.ObjectID, want bool) {
		t.Helper()
		before := calls.Load()
		_, err := service.workflowCallAPI(f.ctx, step, workflowExecutionPayload{TenantID: tenantID.Hex(), ProjectID: projectID.Hex()})
		if (err == nil) != want {
			t.Fatalf("tenant=%s project=%s err=%v wantSuccess=%t", tenantID.Hex(), projectID.Hex(), err, want)
		}
		if !want && calls.Load() != before {
			t.Fatal("unauthorized execution reached provider")
		}
	}
	// Absent scope keeps exact original-project access. Promotion preserves the ID and ciphertext.
	run(tenant, a, true)
	run(tenant, b, false)
	if err = f.repo.PromoteExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex(), f.user); err != nil {
		t.Fatal(err)
	}
	stored, err := f.repo.GetTenantExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex())
	if err != nil || stored.Scope != "tenant" || !stored.ProjectID.IsZero() || stored.EncryptedSecret != encrypted {
		t.Fatalf("promotion altered identity or secret: %#v %v", stored, err)
	}
	run(tenant, a, true)
	run(tenant, b, false)
	for i := 0; i < 2; i++ {
		if err = f.repo.GrantExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex(), b.Hex(), f.user); err != nil {
			t.Fatal(err)
		}
	}
	grants, err := f.db.Collection("external_api_credential_grants").CountDocuments(f.ctx, bson.M{"credentialId": credential.ID})
	if err != nil || grants != 2 {
		t.Fatalf("grant idempotence: %d %v", grants, err)
	}
	run(tenant, a, true)
	run(tenant, b, true)
	run(tenant, c, false)
	run(otherTenant, a, false)
	result, err := f.repo.RevokeExternalAPICredential(f.ctx, tenant.Hex(), a.Hex(), credential.ID.Hex(), time.Now())
	if err != nil || result.MatchedCount != 0 {
		t.Fatalf("project revoked shared key: %#v %v", result, err)
	}
	if err = f.repo.DisconnectExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex(), a.Hex()); err != nil {
		t.Fatal(err)
	}
	run(tenant, a, false)
	run(tenant, b, true)
	result, err = f.repo.RevokeTenantExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex(), time.Now())
	if err != nil || result.MatchedCount != 1 {
		t.Fatalf("tenant revoke: %#v %v", result, err)
	}
	run(tenant, a, false)
	run(tenant, b, false)
	if err = f.repo.GrantExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex(), c.Hex(), f.user); err == nil {
		t.Fatal("revoked key granted")
	}
}

func TestWorkflowMongoSharedCredentialGrantCannotOutliveConcurrentProjectDeletion(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err := f.db.Collection("projects").InsertOne(f.ctx, models.Project{ID: project, TenantID: tenant, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	credential := models.ExternalAPICredential{ID: primitive.NewObjectID(), TenantID: tenant, Scope: "tenant"}
	if _, err := f.repo.InsertExternalAPICredential(f.ctx, credential); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.EnsureExternalAPICredentialGrantIndexes(f.ctx); err != nil {
		t.Fatal(err)
	}
	paused, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	client, err := mongo.Connect(f.ctx, options.Client().ApplyURI(os.Getenv("AUTOTABLE_WORKFLOW_MONGO_URI")).SetMonitor(&event.CommandMonitor{Started: func(ctx context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "update" && e.DatabaseName == f.db.Name() && e.Command.Lookup("update").StringValue() == "external_api_credential_grants" {
			once.Do(func() {
				close(paused)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			})
		}
	}}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	var resumeOnce sync.Once
	unpause := func() { resumeOnce.Do(func() { close(resume) }) }
	defer unpause()
	writer := repositories.NewDynamicRepositoryWithDatabase(client.Database(f.db.Name()))
	granted := make(chan error, 1)
	go func() {
		granted <- writer.GrantExternalAPICredential(f.ctx, tenant.Hex(), credential.ID.Hex(), project.Hex(), f.user)
	}()
	select {
	case <-paused:
	case err := <-granted:
		t.Fatalf("grant failed before mutation: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("grant did not pause")
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := f.db.Collection("projects").UpdateOne(f.ctx, bson.M{"_id": project}, bson.M{"$set": bson.M{"isActive": false}})
		if err == nil {
			_, err = f.db.Collection("external_api_credential_grants").DeleteMany(f.ctx, bson.M{"projectId": project})
		}
		if err == nil {
			_, err = f.db.Collection("projects").DeleteOne(f.ctx, bson.M{"_id": project})
		}
		deleted <- err
	}()
	// A correct grant holds the project write until commit; deletion then removes its grant.
	select {
	case err := <-deleted:
		if err != nil {
			t.Fatal(err)
		}
		deleted <- nil
	case <-time.After(150 * time.Millisecond):
	}
	unpause()
	if err := <-granted; err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	count, err := f.db.Collection("external_api_credential_grants").CountDocuments(f.ctx, bson.M{"projectId": project})
	if err != nil || count != 0 {
		t.Fatalf("concurrent deletion left grant: count=%d err=%v", count, err)
	}
}

func TestWorkflowMongoSharedCredentialPromotionCommitsOriginalGrantAtomically(t *testing.T) {
	f := newWorkflowMongoFixture(t)
	tenant, project := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err := f.db.Collection("projects").InsertOne(f.ctx, models.Project{ID: project, TenantID: tenant, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	c := models.ExternalAPICredential{ID: primitive.NewObjectID(), TenantID: tenant, ProjectID: project, EncryptedSecret: "preserve-ciphertext"}
	if _, err := f.repo.InsertExternalAPICredential(f.ctx, c); err != nil {
		t.Fatal(err)
	}
	paused, resume := make(chan struct{}), make(chan struct{})
	var once, resumeOnce sync.Once
	unpause := func() { resumeOnce.Do(func() { close(resume) }) }
	defer unpause()
	client, err := mongo.Connect(f.ctx, options.Client().ApplyURI(os.Getenv("AUTOTABLE_WORKFLOW_MONGO_URI")).SetMonitor(&event.CommandMonitor{Started: func(ctx context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "update" && e.DatabaseName == f.db.Name() && e.Command.Lookup("update").StringValue() == "external_api_credentials" {
			once.Do(func() {
				close(paused)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			})
		}
	}}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	writer := repositories.NewDynamicRepositoryWithDatabase(client.Database(f.db.Name()))
	done := make(chan error, 1)
	go func() { done <- writer.PromoteExternalAPICredential(f.ctx, tenant.Hex(), c.ID.Hex(), f.user) }()
	select {
	case <-paused:
	case err := <-done:
		t.Fatalf("promotion failed before pause: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("promotion did not pause")
	}
	// The grant insert is still uncommitted; ordinary runtime readers keep original access.
	if got, err := f.repo.GetExternalAPICredential(f.ctx, tenant.Hex(), project.Hex(), c.ID.Hex()); err != nil || got.ProjectID != project {
		t.Fatalf("promotion created access gap: %#v %v", got, err)
	}
	count, err := f.db.Collection("external_api_credential_grants").CountDocuments(f.ctx, bson.M{"credentialId": c.ID})
	if err != nil || count != 0 {
		t.Fatalf("uncommitted grant became visible: %d %v", count, err)
	}
	unpause()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, err := f.repo.GetExternalAPICredential(f.ctx, tenant.Hex(), project.Hex(), c.ID.Hex()); err != nil || got.Scope != "tenant" || got.EncryptedSecret != "preserve-ciphertext" {
		t.Fatalf("original access lost after promotion: %#v %v", got, err)
	}
	// Reject the scope update after a new original grant was inserted in the transaction.
	// Aborting must remove that partial grant and preserve the old credential unchanged.
	rejected := c
	rejected.ID = primitive.NewObjectID()
	if _, err = f.repo.InsertExternalAPICredential(f.ctx, rejected); err != nil {
		t.Fatal(err)
	}
	if err = f.db.RunCommand(f.ctx, bson.D{{Key: "collMod", Value: "external_api_credentials"}, {Key: "validator", Value: bson.M{"scope": bson.M{"$ne": "tenant"}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err = f.repo.PromoteExternalAPICredential(f.ctx, tenant.Hex(), rejected.ID.Hex(), f.user); err == nil {
		t.Fatal("expected rejected promotion")
	}
	count, err = f.db.Collection("external_api_credential_grants").CountDocuments(f.ctx, bson.M{"credentialId": rejected.ID})
	if err != nil || count != 0 {
		t.Fatalf("aborted promotion left grant: %d %v", count, err)
	}
	stored, err := f.repo.GetExternalAPICredential(f.ctx, tenant.Hex(), project.Hex(), rejected.ID.Hex())
	if err != nil || stored.ProjectID != project || stored.EffectiveScope() != "project" || stored.EncryptedSecret != c.EncryptedSecret {
		t.Fatalf("aborted promotion changed original key: %#v %v", stored, err)
	}
}
