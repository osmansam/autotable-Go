package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"reflect"
	"testing"
)

func TestTenantIntegrationPermissionsRequireTenantOwnerOrAdmin(t *testing.T) {
	for _, p := range []ManagementPermission{"tenant.integrations.read", "tenant.integrations.manage"} {
		if got := RolesForPermission(p); !reflect.DeepEqual(got, []string{models.TenantRoleOwner, models.TenantRoleAdmin}) {
			t.Errorf("%s roles=%v", p, got)
		}
	}
}

type sharedCredentialMembershipRepository struct{ snapshot ManagementMembershipSnapshot }

func (r *sharedCredentialMembershipRepository) LoadManagementMembership(context.Context, ManagementAuthorizationSubject, ManagementAuthorizationTarget) (ManagementMembershipSnapshot, error) {
	return r.snapshot, nil
}
func TestTenantCredentialAuthorityUsesLiveTenantMembership(t *testing.T) {
	repo := &sharedCredentialMembershipRepository{snapshot: ManagementMembershipSnapshot{UserActive: true, TenantActive: true, ProjectActive: true, TenantMembership: &models.TenantMembership{Status: models.MembershipStatusActive, Roles: []string{models.TenantRoleAdmin}}, ProjectMembership: &models.ProjectMembership{Status: models.MembershipStatusActive, Roles: []string{models.ProjectRoleAdmin}}}}
	service := NewManagementAuthorizationService(repo)
	subject := ManagementAuthorizationSubject{UserID: "user", TenantID: "tenant"}
	target := ManagementAuthorizationTarget{TenantID: "tenant"}
	for _, permission := range []ManagementPermission{PermissionTenantIntegrationsRead, PermissionTenantIntegrationsManage} {
		if err := service.Authorize(context.Background(), subject, target, permission); err != nil {
			t.Fatal("active tenant admin denied", err)
		}
		repo.snapshot.TenantMembership.Roles = []string{models.TenantRoleBilling}
		if err := service.Authorize(context.Background(), subject, target, permission); !errors.Is(err, ErrManagementForbidden) {
			t.Fatal("project admin obtained tenant authority", err)
		}
		repo.snapshot.TenantMembership.Roles = []string{models.TenantRoleOwner}
		repo.snapshot.TenantMembership.Status = models.MembershipStatusDisabled
		if err := service.Authorize(context.Background(), subject, target, permission); !errors.Is(err, ErrManagementForbidden) {
			t.Fatal("disabled owner retained credential authority", err)
		}
		repo.snapshot.TenantMembership.Status = models.MembershipStatusActive
		foreign := target
		foreign.TenantID = "foreign"
		if err := service.Authorize(context.Background(), subject, foreign, permission); !errors.Is(err, ErrManagementForbidden) {
			t.Fatal("foreign tenant allowed", err)
		}
	}
}
