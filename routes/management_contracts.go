package routes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/services"
)

type ManagementRouteIdentity string
type ManagementRouteScope string

const (
	IdentityManagement           ManagementRouteIdentity = "management"
	IdentityPublicManagementAuth ManagementRouteIdentity = "public-management-auth"

	ManagementScopeNone            ManagementRouteScope = "none"
	ManagementScopeTenant          ManagementRouteScope = "tenant"
	ManagementScopeProject         ManagementRouteScope = "project"
	ManagementScopeTargetProject   ManagementRouteScope = "target-project"
	ManagementScopeTenantToProject ManagementRouteScope = "tenant-to-project"
	ManagementScopeAny             ManagementRouteScope = "tenant-or-project"

	ManagementPermissionNone services.ManagementPermission = "none"
)

type ManagementRouteContract struct {
	Method     string
	Path       string
	Identity   ManagementRouteIdentity
	Scope      ManagementRouteScope
	Permission services.ManagementPermission
	// Full definitions require management; this flag permits only a filtered
	// read-only metadata response for a separately authenticated application user.
	RuntimeMetadata bool
}

func contract(method, path string, identity ManagementRouteIdentity, scope ManagementRouteScope, permission services.ManagementPermission) ManagementRouteContract {
	return ManagementRouteContract{Method: method, Path: path, Identity: identity, Scope: scope, Permission: permission}
}

func containerMetadataContract(path string) ManagementRouteContract {
	c := contract("GET", path, IdentityManagement, ManagementScopeProject, services.PermissionProjectConfigRead)
	c.RuntimeMetadata = true
	return c
}

var approvedManagementRouteContracts = []ManagementRouteContract{
	contract("GET", "/api/v1/tenant/integrations/external-api-credentials", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsRead),
	contract("POST", "/api/v1/tenant/integrations/external-api-credentials", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsManage),
	contract("POST", "/api/v1/tenant/integrations/external-api-credentials/:id/revoke", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsManage),
	contract("POST", "/api/v1/tenant/integrations/external-api-credentials/:id/promote", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsManage),
	contract("PUT", "/api/v1/tenant/integrations/external-api-credentials/:id/projects/:projectId", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsManage),
	contract("DELETE", "/api/v1/tenant/integrations/external-api-credentials/:id/projects/:projectId", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsManage),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/dynamic/testPipeline", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/tenant-auth/refresh", IdentityPublicManagementAuth, ManagementScopeProject, ManagementPermissionNone),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/tenant-auth/logout", IdentityPublicManagementAuth, ManagementScopeProject, ManagementPermissionNone),
	contract("POST", "/api/v1/tenant/auth/register", IdentityPublicManagementAuth, ManagementScopeNone, ManagementPermissionNone),
	contract("POST", "/api/v1/tenant/auth/login", IdentityPublicManagementAuth, ManagementScopeNone, ManagementPermissionNone),
	contract("GET", "/api/v1/tenant/auth/session", IdentityPublicManagementAuth, ManagementScopeNone, ManagementPermissionNone),
	contract("POST", "/api/v1/tenant/auth/refresh", IdentityPublicManagementAuth, ManagementScopeTenant, ManagementPermissionNone),
	contract("POST", "/api/v1/tenant/auth/logout", IdentityPublicManagementAuth, ManagementScopeTenant, ManagementPermissionNone),
	contract("GET", "/api/v1/tenant/auth/me", IdentityManagement, ManagementScopeAny, services.PermissionTenantProjectsRead),
	contract("POST", "/api/v1/tenant/auth/switch-project", IdentityManagement, ManagementScopeTenantToProject, services.PermissionProjectConfigRead),

	contract("POST", "/api/v1/tenant/projects/", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsCreate),
	contract("GET", "/api/v1/tenant/projects/", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsRead),
	contract("GET", "/api/v1/tenant/projects/templates/:id/external-api-requirements", IdentityManagement, ManagementScopeTenant, services.PermissionTenantIntegrationsRead),
	contract("GET", "/api/v1/tenant/projects/templates", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsRead),
	contract("PATCH", "/api/v1/tenant/projects/:id/locales", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectLocalizationManage),
	contract("GET", "/api/v1/tenant/projects/:id/translations", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectLocalizationRead),
	contract("POST", "/api/v1/tenant/projects/:id/translations/generate", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectLocalizationManage),
	contract("GET", "/api/v1/tenant/projects/:id/translation-jobs/:jobId", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectLocalizationRead),
	contract("PATCH", "/api/v1/tenant/projects/:id/translations/:locale/:key", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectLocalizationManage),
	contract("PATCH", "/api/v1/tenant/projects/:id/template", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsUpdate),
	contract("GET", "/api/v1/tenant/projects/:id", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsRead),
	contract("PATCH", "/api/v1/tenant/projects/:id", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsUpdate),
	contract("DELETE", "/api/v1/tenant/projects/:id", IdentityManagement, ManagementScopeTenant, services.PermissionTenantProjectsDelete),

	contract("GET", "/api/v1/tenant/branding/", IdentityManagement, ManagementScopeTenant, services.PermissionTenantBrandingRead),
	contract("PATCH", "/api/v1/tenant/branding/", IdentityManagement, ManagementScopeTenant, services.PermissionTenantBrandingManage),
	contract("POST", "/api/v1/tenant/branding/assets/:slot", IdentityManagement, ManagementScopeTenant, services.PermissionTenantBrandingManage),
	contract("DELETE", "/api/v1/tenant/branding/assets/:slot", IdentityManagement, ManagementScopeTenant, services.PermissionTenantBrandingManage),
	contract("GET", "/api/v1/tenant/projects/:id/branding/", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectBrandingRead),
	contract("PATCH", "/api/v1/tenant/projects/:id/branding/", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectBrandingManage),
	contract("POST", "/api/v1/tenant/projects/:id/branding/assets/:slot", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectBrandingManage),
	contract("DELETE", "/api/v1/tenant/projects/:id/branding/assets/:slot", IdentityManagement, ManagementScopeTargetProject, services.PermissionProjectBrandingManage),

	containerMetadataContract("/api/v1/:tenantSlug/:projectSlug/container"),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/admin/auth/users", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/container/", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/container/create-multiple", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/container/reset-redis", IdentityManagement, ManagementScopeProject, services.PermissionProjectCacheReset),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/container/tenant", IdentityManagement, ManagementScopeProject, services.PermissionProjectConfigRead),
	containerMetadataContract("/api/v1/:tenantSlug/:projectSlug/container/types"),
	// Dynamic functions are paused; restore this contract with the update route.
	// contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/container/dynamicFunctions/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/container/pipelines/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/container/workflows/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/container/dynamicApis/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),
	containerMetadataContract("/api/v1/:tenantSlug/:projectSlug/container/:id"),
	contract("DELETE", "/api/v1/:tenantSlug/:projectSlug/container/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaDelete),
	contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/container/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectSchemaWrite),

	contract("POST", "/api/v1/:tenantSlug/:projectSlug/admin/page/", IdentityManagement, ManagementScopeProject, services.PermissionProjectPageWrite),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/admin/page/", IdentityManagement, ManagementScopeProject, services.PermissionProjectConfigRead),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/admin/page/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectConfigRead),
	contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/admin/page/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectPageWrite),
	contract("DELETE", "/api/v1/:tenantSlug/:projectSlug/admin/page/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectPageDelete),

	contract("GET", "/api/v1/:tenantSlug/:projectSlug/roles", IdentityManagement, ManagementScopeProject, services.PermissionProjectRolesRead),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/roles/:id", IdentityManagement, ManagementScopeProject, services.PermissionProjectRolesRead),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/excel/upload", IdentityManagement, ManagementScopeProject, services.PermissionProjectImport),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/excel/upload-multiple", IdentityManagement, ManagementScopeProject, services.PermissionProjectImport),

	contract("GET", "/api/v1/:tenantSlug/:projectSlug/integrations/credentials", IdentityManagement, ManagementScopeProject, services.PermissionProjectIntegrationsRead),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/integrations/credentials", IdentityManagement, ManagementScopeProject, services.PermissionProjectIntegrationsManage),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/integrations/credentials/:id/revoke", IdentityManagement, ManagementScopeProject, services.PermissionProjectIntegrationsManage),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/integrations/external-api-credentials", IdentityManagement, ManagementScopeProject, services.PermissionProjectIntegrationsRead),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/integrations/external-api-credentials", IdentityManagement, ManagementScopeProject, services.PermissionProjectIntegrationsManage),
	contract("POST", "/api/v1/:tenantSlug/:projectSlug/integrations/external-api-credentials/:id/revoke", IdentityManagement, ManagementScopeProject, services.PermissionProjectIntegrationsManage),

	contract("GET", "/api/v1/:tenantSlug/:projectSlug/admin/audit-logs/config", IdentityManagement, ManagementScopeProject, services.PermissionProjectAuditConfigure),
	contract("PATCH", "/api/v1/:tenantSlug/:projectSlug/admin/audit-logs/config", IdentityManagement, ManagementScopeProject, services.PermissionProjectAuditConfigure),
	contract("GET", "/api/v1/:tenantSlug/:projectSlug/admin/audit-logs/", IdentityManagement, ManagementScopeProject, services.PermissionProjectAuditRead),
}

func ApprovedManagementRouteContracts() []ManagementRouteContract {
	return append([]ManagementRouteContract(nil), approvedManagementRouteContracts...)
}

func validateManagementContracts(contracts []ManagementRouteContract) error {
	seen := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if c.RuntimeMetadata {
			allowed := c.Path == "/api/v1/:tenantSlug/:projectSlug/container" || c.Path == "/api/v1/:tenantSlug/:projectSlug/container/types" || c.Path == "/api/v1/:tenantSlug/:projectSlug/container/:id"
			if !allowed || c.Method != "GET" || c.Identity != IdentityManagement || c.Scope != ManagementScopeProject || c.Permission != services.PermissionProjectConfigRead {
				return errors.New("runtime metadata is permitted only on approved container reads")
			}
		}
		if strings.TrimSpace(c.Method) == "" || strings.TrimSpace(c.Path) == "" || c.Identity == "" || c.Scope == "" || c.Permission == "" {
			return errors.New("management route contract has an empty method, path, identity, scope, or permission")
		}
		if c.Identity == IdentityManagement && c.Permission == ManagementPermissionNone {
			return fmt.Errorf("management route %s %s has no permission", c.Method, c.Path)
		}
		if c.Identity == IdentityPublicManagementAuth && c.Permission != ManagementPermissionNone {
			return fmt.Errorf("public management-auth route %s %s cannot grant a management permission", c.Method, c.Path)
		}
		key := strings.ToUpper(c.Method) + " " + c.Path
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate management route contract: %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func isManagementRoute(path string) bool {
	return strings.HasPrefix(path, "/api/v1/tenant/auth") ||
		strings.HasPrefix(path, "/api/v1/tenant/projects") ||
		strings.HasPrefix(path, "/api/v1/tenant/branding") ||
		strings.Contains(path, "/tenant-auth/") || strings.Contains(path, "/container") ||
		strings.Contains(path, "/dynamic/testPipeline") || strings.Contains(path, "/admin/page") || strings.Contains(path, "/admin/auth/") || strings.Contains(path, "/roles") ||
		strings.Contains(path, "/excel/") || strings.Contains(path, "/integrations/") ||
		strings.Contains(path, "/admin/audit-logs")
}

func validateManagementRouteInventory(registered []fiber.Route, contracts []ManagementRouteContract) error {
	if err := validateManagementContracts(contracts); err != nil {
		return err
	}
	contractSet := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		contractSet[strings.ToUpper(c.Method)+" "+c.Path] = struct{}{}
	}
	registeredSet := make(map[string]struct{})
	for _, route := range registered {
		if route.Method == fiber.MethodHead || !isManagementRoute(route.Path) {
			continue
		}
		key := route.Method + " " + route.Path
		registeredSet[key] = struct{}{}
		if _, ok := contractSet[key]; !ok {
			return fmt.Errorf("registered management route missing contract: %s", key)
		}
	}
	for key := range contractSet {
		if _, ok := registeredSet[key]; !ok {
			return fmt.Errorf("management route contract has no registered route: %s", key)
		}
	}
	return nil
}

type liveManagementAuthorizer struct{}

func (liveManagementAuthorizer) Authorize(ctx context.Context, subject services.ManagementAuthorizationSubject, target services.ManagementAuthorizationTarget, permission services.ManagementPermission) error {
	return services.NewManagementAuthorizationService(repositories.NewManagementMembershipRepository()).Authorize(ctx, subject, target, permission)
}

var routeManagementAuthorizer services.ManagementAuthorizer = liveManagementAuthorizer{}

func approvedManagementContract(method, path string) (ManagementRouteContract, bool) {
	for _, c := range approvedManagementRouteContracts {
		if c.Method == strings.ToUpper(method) && c.Path == path {
			return c, true
		}
	}
	return ManagementRouteContract{}, false
}

func strictManagementRouteHandlers(method, path string, routeHandlers ...fiber.Handler) []fiber.Handler {
	c, ok := approvedManagementContract(method, path)
	if !ok || c.Identity != IdentityManagement {
		panic("missing strict management route contract for " + strings.ToUpper(method) + " " + path)
	}
	securityHandlers, err := strictManagementHandlers(c)
	if err != nil {
		panic(err)
	}
	return append(securityHandlers, routeHandlers...)
}

func strictManagementHandlers(c ManagementRouteContract) ([]fiber.Handler, error) {
	if err := validateManagementContracts([]ManagementRouteContract{c}); err != nil {
		return nil, err
	}
	if c.Identity != IdentityManagement {
		return nil, nil
	}
	handlers := []fiber.Handler{middlewares.ManagementAuthenticate}
	var resolver middlewares.ManagementTargetResolver
	switch c.Scope {
	case ManagementScopeTenant:
		handlers = append(handlers, middlewares.RequireManagementTenantScope)
		resolver = principalTenantTarget
	case ManagementScopeProject:
		handlers = append(handlers, middlewares.RequireManagementProjectScope, middlewares.BindManagementProject)
		resolver = principalProjectTarget
	case ManagementScopeTargetProject:
		resolver = parameterProjectTarget(false)
	case ManagementScopeTenantToProject:
		handlers = append(handlers, middlewares.RequireManagementTenantScope)
		resolver = parameterProjectTarget(true)
	case ManagementScopeAny:
		resolver = principalTenantTarget
	default:
		return nil, fmt.Errorf("unsupported management scope %q", c.Scope)
	}
	handlers = append(handlers, middlewares.RequireManagementPermission(routeManagementAuthorizer, c.Permission, resolver))
	return handlers, nil
}

func principalTenantTarget(c *fiber.Ctx) (services.ManagementAuthorizationTarget, error) {
	principal, ok := middlewares.ManagementPrincipalFromContext(c)
	if !ok {
		return services.ManagementAuthorizationTarget{}, errors.New("management principal missing")
	}
	return services.ManagementAuthorizationTarget{TenantID: principal.TenantID}, nil
}

func principalProjectTarget(c *fiber.Ctx) (services.ManagementAuthorizationTarget, error) {
	principal, ok := middlewares.ManagementPrincipalFromContext(c)
	if !ok || principal.ProjectID == "" {
		return services.ManagementAuthorizationTarget{}, errors.New("project management principal missing")
	}
	return services.ManagementAuthorizationTarget{TenantID: principal.TenantID, ProjectID: principal.ProjectID}, nil
}

func parameterProjectTarget(requireTenantScope bool) middlewares.ManagementTargetResolver {
	return func(c *fiber.Ctx) (services.ManagementAuthorizationTarget, error) {
		principal, ok := middlewares.ManagementPrincipalFromContext(c)
		projectID := c.Params("id")
		if projectID == "" && requireTenantScope {
			var input struct {
				ProjectID string `json:"projectId"`
			}
			if err := c.BodyParser(&input); err == nil {
				projectID = input.ProjectID
			}
		}
		if !ok || projectID == "" || (requireTenantScope && principal.Scope != "tenant") || (principal.ProjectID != "" && principal.ProjectID != projectID) {
			return services.ManagementAuthorizationTarget{}, errors.New("target project does not match management scope")
		}
		return services.ManagementAuthorizationTarget{TenantID: principal.TenantID, ProjectID: projectID}, nil
	}
}
