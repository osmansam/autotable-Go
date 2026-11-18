package services

import (
	"errors"
	"github.com/osmansam/autotableGo/models"
	"testing"
)

func TestCollaborationMemberAuthority(t *testing.T) {
	actor, target := models.ManagementAuthorizationSubject{UserID: "user", TenantID: "tenant"}, models.ManagementAuthorizationTarget{TenantID: "tenant", ProjectID: "project"}
	snapshot := activeCollaborationSnapshot([]string{models.TenantRoleMember}, []string{models.ProjectRoleDeveloper})
	if err := authorizeCollaboration(models.CollaborationAuthorityProject, actor, target, snapshot, true); !errors.Is(err, ErrManagementForbidden) {
		t.Fatalf("developer can manage members: %v", err)
	}
	snapshot.TenantMembership.Roles = []string{models.TenantRoleAdmin}
	snapshot.ProjectMembership = nil
	if err := authorizeCollaboration(models.CollaborationAuthorityTenant, actor, target, snapshot, true); err != nil {
		t.Fatal(err)
	}
	if err := authorizeCollaboration(models.CollaborationAuthorityProject, actor, target, snapshot, true); err == nil {
		t.Fatal("tenant admin implied project admin")
	}
}
func TestCollaborationLegacyRoleConversionExplicit(t *testing.T) {
	member := models.ProjectMembership{Roles: []string{models.ProjectRoleSupport, models.ProjectRoleEditor}, Status: models.MembershipStatusActive, Version: 4}
	input := models.RoleChangeInput{Role: models.ProjectRoleViewer, ExpectedVersion: 4}
	if err := validateCollaborationRoleChange(member, input); !errors.Is(err, ErrCollaborationLegacyRoles) {
		t.Fatalf("legacy roles silently replaced: %v", err)
	}
	input.ConvertLegacyRoleSet = true
	input.ExpectedRoles = []string{models.ProjectRoleEditor, models.ProjectRoleSupport}
	if err := validateCollaborationRoleChange(member, input); err != nil {
		t.Fatal(err)
	}
	input.ExpectedRoles = []string{models.ProjectRoleEditor}
	if err := validateCollaborationRoleChange(member, input); err == nil {
		t.Fatal("incomplete legacy conversion accepted")
	}
}
func TestCollaborationMemberVersionConflict(t *testing.T) {
	member := models.ProjectMembership{Roles: []string{models.ProjectRoleViewer}, Status: models.MembershipStatusActive, Version: 3}
	if err := validateCollaborationRoleChange(member, models.RoleChangeInput{Role: models.ProjectRoleAdmin, ExpectedVersion: 2}); !errors.Is(err, ErrCollaborationVersion) {
		t.Fatalf("stale role update accepted: %v", err)
	}
	if err := validateCollaborationRoleChange(member, models.RoleChangeInput{Role: models.TenantRoleOwner, ExpectedVersion: 3}); err == nil {
		t.Fatal("project role escalated to tenant owner")
	}
}
