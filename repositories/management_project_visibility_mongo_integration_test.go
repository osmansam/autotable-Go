//go:build integration

package repositories

import (
	"errors"
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"testing"
)

func TestCollaborationPrivilegedProjectVisibility(t *testing.T) {
	for _, role := range []string{models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor} {
		t.Run(role, func(t *testing.T) {
			f := testmongo.New(t)
			user, tenant := primitive.NewObjectID(), primitive.NewObjectID()
			active, inactive, initializing := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
			for collection, doc := range map[string]interface{}{"users": models.User{ID: user, IsActive: true}, "tenants": models.Tenant{ID: tenant, IsActive: true}, "tenant_memberships": models.TenantMembership{ID: primitive.NewObjectID(), UserID: user, TenantID: tenant, Status: models.MembershipStatusActive, Roles: []string{role}}} {
				if _, err := f.DB.Collection(collection).InsertOne(f.Ctx, doc); err != nil {
					t.Fatal(err)
				}
			}
			for _, p := range []models.Project{{ID: active, TenantID: tenant, IsActive: true}, {ID: inactive, TenantID: tenant, IsActive: false}, {ID: initializing, TenantID: tenant, IsActive: true, Initializing: true}} {
				if _, err := f.DB.Collection("projects").InsertOne(f.Ctx, p); err != nil {
					t.Fatal(err)
				}
			}
			repository := NewManagementProjectAccessRepository(f.DB)
			subject := models.ManagementAuthorizationSubject{UserID: user.Hex(), TenantID: tenant.Hex()}
			projects, err := repository.ListVisibleProjects(f.Ctx, subject)
			if err != nil {
				t.Fatal(err)
			}
			if len(projects) != 2 {
				t.Fatalf("expected ready active/inactive projects, got %d", len(projects))
			}
			for _, p := range projects {
				if p.ID == initializing {
					t.Fatal("initializing project disclosed")
				}
			}
			if _, err := repository.GetVisibleProject(f.Ctx, subject, initializing.Hex()); !errors.Is(err, mongo.ErrNoDocuments) {
				t.Fatal("direct lookup exposed initializing project")
			}
			if _, err := repository.GetVisibleProject(f.Ctx, subject, inactive.Hex()); err != nil {
				t.Fatal("inactive administrative project hidden")
			}
		})
	}
}
