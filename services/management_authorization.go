package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/osmansam/autotableGo/models"
)

type ManagementPermission string

const (
	PermissionTenantIntegrationsRead    ManagementPermission = "tenant.integrations.read"
	PermissionTenantIntegrationsManage  ManagementPermission = "tenant.integrations.manage"
	PermissionTenantProjectsRead        ManagementPermission = "tenant.projects.read"
	PermissionTenantProjectsCreate      ManagementPermission = "tenant.projects.create"
	PermissionTenantProjectsUpdate      ManagementPermission = "tenant.projects.update"
	PermissionTenantProjectsDelete      ManagementPermission = "tenant.projects.delete"
	PermissionTenantBrandingRead        ManagementPermission = "tenant.branding.read"
	PermissionTenantBrandingManage      ManagementPermission = "tenant.branding.manage"
	PermissionProjectConfigRead         ManagementPermission = "project.config.read"
	PermissionProjectSchemaWrite        ManagementPermission = "project.schema.write"
	PermissionProjectSchemaDelete       ManagementPermission = "project.schema.delete"
	PermissionProjectCacheReset         ManagementPermission = "project.cache.reset"
	PermissionProjectPageWrite          ManagementPermission = "project.page.write"
	PermissionProjectPageDelete         ManagementPermission = "project.page.delete"
	PermissionProjectRolesRead          ManagementPermission = "project.roles.read"
	PermissionProjectImport             ManagementPermission = "project.import"
	PermissionProjectIntegrationsRead   ManagementPermission = "project.integrations.read"
	PermissionProjectIntegrationsManage ManagementPermission = "project.integrations.manage"
	PermissionProjectLocalizationRead   ManagementPermission = "project.localization.read"
	PermissionProjectLocalizationManage ManagementPermission = "project.localization.manage"
	PermissionProjectBrandingRead       ManagementPermission = "project.branding.read"
	PermissionProjectBrandingManage     ManagementPermission = "project.branding.manage"
	PermissionProjectAuditRead          ManagementPermission = "project.audit.read"
	PermissionProjectAuditConfigure     ManagementPermission = "project.audit.configure"
)

type ManagementAuthorizationSubject = models.ManagementAuthorizationSubject
type ManagementAuthorizationTarget = models.ManagementAuthorizationTarget
type ManagementMembershipSnapshot = models.ManagementMembershipSnapshot

var (
	ErrManagementForbidden                = errors.New("management permission denied")
	ErrManagementAuthorizationUnavailable = errors.New("management authorization unavailable")
)

type ManagementMembershipRepository interface {
	LoadManagementMembership(context.Context, ManagementAuthorizationSubject, ManagementAuthorizationTarget) (ManagementMembershipSnapshot, error)
}

type ManagementAuthorizer interface {
	Authorize(context.Context, ManagementAuthorizationSubject, ManagementAuthorizationTarget, ManagementPermission) error
}

type ManagementAuthorizationService struct {
	repository ManagementMembershipRepository
}

func NewManagementAuthorizationService(repository ManagementMembershipRepository) *ManagementAuthorizationService {
	return &ManagementAuthorizationService{repository: repository}
}

var managementPermissionRoles = map[ManagementPermission][]string{
	PermissionTenantIntegrationsRead:    {models.TenantRoleOwner, models.TenantRoleAdmin},
	PermissionTenantIntegrationsManage:  {models.TenantRoleOwner, models.TenantRoleAdmin},
	PermissionTenantProjectsRead:        {models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor},
	PermissionTenantProjectsCreate:      {models.TenantRoleOwner, models.TenantRoleAdmin},
	PermissionTenantProjectsUpdate:      {models.TenantRoleOwner, models.TenantRoleAdmin},
	PermissionTenantProjectsDelete:      {models.TenantRoleOwner},
	PermissionTenantBrandingRead:        {models.TenantRoleOwner, models.TenantRoleAdmin, models.TenantRoleBilling, models.TenantRoleAuditor},
	PermissionTenantBrandingManage:      {models.TenantRoleOwner, models.TenantRoleAdmin},
	PermissionProjectConfigRead:         {models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer, models.ProjectRoleSupport},
	PermissionProjectSchemaWrite:        {models.ProjectRoleAdmin, models.ProjectRoleDeveloper},
	PermissionProjectSchemaDelete:       {models.ProjectRoleAdmin},
	PermissionProjectCacheReset:         {models.ProjectRoleAdmin},
	PermissionProjectPageWrite:          {models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor},
	PermissionProjectPageDelete:         {models.ProjectRoleAdmin},
	PermissionProjectRolesRead:          {models.ProjectRoleAdmin, models.ProjectRoleDeveloper},
	PermissionProjectImport:             {models.ProjectRoleAdmin, models.ProjectRoleDeveloper},
	PermissionProjectIntegrationsRead:   {models.ProjectRoleAdmin, models.ProjectRoleDeveloper},
	PermissionProjectIntegrationsManage: {models.ProjectRoleAdmin, models.ProjectRoleDeveloper},
	PermissionProjectLocalizationRead:   {models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer, models.ProjectRoleSupport},
	PermissionProjectLocalizationManage: {models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor},
	PermissionProjectBrandingRead:       {models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleEditor, models.ProjectRoleViewer, models.ProjectRoleSupport},
	PermissionProjectBrandingManage:     {models.ProjectRoleAdmin},
	PermissionProjectAuditRead:          {models.ProjectRoleAdmin, models.ProjectRoleDeveloper, models.ProjectRoleSupport},
	PermissionProjectAuditConfigure:     {models.ProjectRoleAdmin, models.ProjectRoleDeveloper},
}

func AllProjectPermissions() []ManagementPermission {
	permissions := make([]ManagementPermission, 0)
	for permission := range managementPermissionRoles {
		if IsProjectPermission(permission) {
			permissions = append(permissions, permission)
		}
	}
	return permissions
}

func IsProjectPermission(permission ManagementPermission) bool {
	return strings.HasPrefix(string(permission), "project.")
}

func (s *ManagementAuthorizationService) Authorize(ctx context.Context, subject ManagementAuthorizationSubject, target ManagementAuthorizationTarget, permission ManagementPermission) error {
	roles, known := managementPermissionRoles[permission]
	if !known || s == nil || s.repository == nil || subject.UserID == "" || subject.TenantID == "" || target.TenantID == "" || subject.TenantID != target.TenantID {
		return ErrManagementForbidden
	}
	if IsProjectPermission(permission) && target.ProjectID == "" {
		return ErrManagementForbidden
	}
	snapshot, err := s.repository.LoadManagementMembership(ctx, subject, target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrManagementAuthorizationUnavailable, err)
	}
	if !snapshot.UserActive || !snapshot.TenantActive || snapshot.TenantMembership == nil || snapshot.TenantMembership.Status != models.MembershipStatusActive {
		return ErrManagementForbidden
	}
	actualRoles := snapshot.TenantMembership.Roles
	if IsProjectPermission(permission) {
		if !snapshot.ProjectActive || snapshot.ProjectMembership == nil || snapshot.ProjectMembership.Status != models.MembershipStatusActive {
			return ErrManagementForbidden
		}
		actualRoles = snapshot.ProjectMembership.Roles
	}
	if !containsAnyRole(actualRoles, roles) {
		return ErrManagementForbidden
	}
	return nil
}

func containsAnyRole(actual, allowed []string) bool {
	for _, role := range actual {
		for _, candidate := range allowed {
			if role == candidate {
				return true
			}
		}
	}
	return false
}

// CurrentManagementSessionRoles validates the live entities required to keep a
// management session and returns a copy of the current membership roles.
func CurrentManagementSessionRoles(snapshot ManagementMembershipSnapshot, projectScoped bool) ([]string, error) {
	if !snapshot.UserActive || !snapshot.TenantActive || snapshot.TenantMembership == nil || snapshot.TenantMembership.Status != models.MembershipStatusActive {
		return nil, ErrManagementForbidden
	}
	if !projectScoped {
		return append([]string(nil), snapshot.TenantMembership.Roles...), nil
	}
	if !snapshot.ProjectActive || snapshot.ProjectMembership == nil || snapshot.ProjectMembership.Status != models.MembershipStatusActive {
		return nil, ErrManagementForbidden
	}
	return append([]string(nil), snapshot.ProjectMembership.Roles...), nil
}

// RolesForPermission returns a copy of the roles that satisfy a permission.
func RolesForPermission(permission ManagementPermission) []string {
	return append([]string(nil), managementPermissionRoles[permission]...)
}
