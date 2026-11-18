//go:build integration

package collaborationmigration

import (
	"github.com/osmansam/autotableGo/internal/testmongo"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
)

func TestCollaborationPreflightUsableAdministrator(t *testing.T) {
	for _, mode := range []string{"empty_roles", "unknown_roles", "wrong_tenant"} {
		t.Run(mode, func(t *testing.T) {
			f := testmongo.New(t)
			user, tenant, other, project := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
			roles := []string{models.TenantRoleMember}
			memberTenant := tenant
			if mode == "empty_roles" {
				roles = []string{}
			}
			if mode == "unknown_roles" {
				roles = []string{"future_role"}
			}
			if mode == "wrong_tenant" {
				memberTenant = other
			}
			for _, doc := range []struct {
				collection string
				value      interface{}
			}{{"users", models.User{ID: user, Email: "admin@example.test", IsActive: true}}, {"tenants", models.Tenant{ID: tenant, IsActive: true}}, {"tenants", models.Tenant{ID: other, IsActive: true}}, {"projects", models.Project{ID: project, TenantID: tenant, IsActive: true}}, {"tenant_memberships", models.TenantMembership{ID: primitive.NewObjectID(), UserID: user, TenantID: memberTenant, Status: models.MembershipStatusActive, Roles: roles}}, {"project_memberships", models.ProjectMembership{ID: primitive.NewObjectID(), UserID: user, TenantID: memberTenant, ProjectID: project, Status: models.MembershipStatusActive, Roles: []string{models.ProjectRoleAdmin}}}} {
				if _, err := f.DB.Collection(doc.collection).InsertOne(f.Ctx, doc.value); err != nil {
					t.Fatal(err)
				}
			}
			report, err := Preflight(f.Ctx, f.DB)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Conflicts) == 0 {
				t.Fatal("unusable administrator certified by migration")
			}
		})
	}
}
