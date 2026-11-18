package services

import (
	"context"
	"errors"
	"testing"

	"github.com/osmansam/autotableGo/models"
)

type collaborationSnapshotRepository struct {
	snapshot models.ManagementMembershipSnapshot
}

func (r collaborationSnapshotRepository) LoadManagementMembership(context.Context, ManagementAuthorizationSubject, ManagementAuthorizationTarget) (ManagementMembershipSnapshot, error) {
	return r.snapshot, nil
}
func activeCollaborationSnapshot(tenantRoles, projectRoles []string) models.ManagementMembershipSnapshot {
	return models.ManagementMembershipSnapshot{UserActive: true, TenantActive: true, ProjectActive: true, TenantMembership: &models.TenantMembership{Status: models.MembershipStatusActive, Roles: tenantRoles}, ProjectMembership: &models.ProjectMembership{Status: models.MembershipStatusActive, Roles: projectRoles}}
}

func TestManagementPermissionScopeIsolation(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tenant, project []string
		permission      ManagementPermission
		allowed         bool
	}{
		{"owner is not project developer", []string{models.TenantRoleOwner}, []string{models.ProjectRoleViewer}, PermissionProjectSchemaWrite, false},
		{"editor pages", []string{"tenant_member"}, []string{models.ProjectRoleEditor}, PermissionProjectPageWrite, true},
		{"editor cannot change schemas", []string{"tenant_member"}, []string{models.ProjectRoleEditor}, PermissionProjectSchemaWrite, false},
		{"support editor retains audit", []string{"tenant_member"}, []string{models.ProjectRoleSupport, models.ProjectRoleEditor}, PermissionProjectAuditRead, true},
		{"support editor retains pages", []string{"tenant_member"}, []string{models.ProjectRoleSupport, models.ProjectRoleEditor}, PermissionProjectPageWrite, true},
		{"member can discover assigned projects", []string{"tenant_member"}, []string{models.ProjectRoleViewer}, "tenant.projects.discover", true},
		{"member can read own account", []string{"tenant_member"}, nil, "tenant.account.read", true},
		{"member cannot enumerate tenant", []string{"tenant_member"}, []string{models.ProjectRoleViewer}, PermissionTenantProjectsRead, false},
		{"tenant admin manages memberships", []string{models.TenantRoleAdmin}, nil, "tenant.project.members.manage", true},
		{"project admin manages memberships", []string{"tenant_member"}, []string{models.ProjectRoleAdmin}, "project.members.manage", true},
		{"developer cannot manage members", []string{"tenant_member"}, []string{models.ProjectRoleDeveloper}, "project.members.manage", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewManagementAuthorizationService(collaborationSnapshotRepository{activeCollaborationSnapshot(tc.tenant, tc.project)})
			err := s.Authorize(context.Background(), ManagementAuthorizationSubject{UserID: "user", TenantID: "tenant", TokenRoles: []string{models.ProjectRoleAdmin}}, ManagementAuthorizationTarget{TenantID: "tenant", ProjectID: "project"}, tc.permission)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v error=%v", tc.allowed, err)
			}
		})
	}
}

func TestManagementPermissionTenantExceptions(t *testing.T) {
	for _, role := range []string{models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor, "tenant_member"} {
		t.Run(role, func(t *testing.T) {
			snapshot := activeCollaborationSnapshot([]string{role}, []string{models.ProjectRoleViewer})
			s := NewManagementAuthorizationService(collaborationSnapshotRepository{snapshot})
			subject, target := ManagementAuthorizationSubject{UserID: "user", TenantID: "tenant"}, ManagementAuthorizationTarget{TenantID: "tenant", ProjectID: "project"}
			if err := s.Authorize(context.Background(), subject, target, "tenant.project.read"); err != nil {
				t.Fatal(err)
			}
			snapshot.ProjectMembership = nil
			s = NewManagementAuthorizationService(collaborationSnapshotRepository{snapshot})
			err := s.Authorize(context.Background(), subject, target, "tenant.project.read")
			if (err == nil) != (role != "tenant_member") {
				t.Fatalf("unassigned role=%s err=%v", role, err)
			}
			snapshot.TenantMembership.Status = models.MembershipStatusDisabled
			s = NewManagementAuthorizationService(collaborationSnapshotRepository{snapshot})
			if err := s.Authorize(context.Background(), subject, target, "tenant.project.read"); !errors.Is(err, ErrManagementForbidden) {
				t.Fatalf("disabled allowed: %v", err)
			}
		})
	}
}

func TestManagementSessionRolesRequireRecognizedRole(t *testing.T) {
	snapshot := activeCollaborationSnapshot([]string{"unknown"}, []string{models.ProjectRoleViewer})
	if _, err := CurrentManagementSessionRoles(snapshot, false); !errors.Is(err, ErrManagementForbidden) {
		t.Fatalf("unknown tenant roles accepted: %v", err)
	}
	snapshot.TenantMembership.Roles = []string{"tenant_member"}
	snapshot.ProjectMembership.Roles = []string{"unknown"}
	if _, err := CurrentManagementSessionRoles(snapshot, true); !errors.Is(err, ErrManagementForbidden) {
		t.Fatalf("unknown project roles accepted: %v", err)
	}
}
