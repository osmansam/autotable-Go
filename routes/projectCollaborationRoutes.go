package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/services"
)

func collaborationRouteContracts() []ManagementRouteContract {
	result := []ManagementRouteContract{contract("GET", "/api/v1/tenant/auth/collaboration-capabilities", IdentityManagement, ManagementScopeAny, services.PermissionTenantAccountRead), contract("GET", "/api/v1/:tenantSlug/:projectSlug/tenant-auth/collaboration-capabilities", IdentityManagement, ManagementScopeProject, services.PermissionProjectConfigRead)}
	for _, project := range []bool{false, true} {
		base, scope, read, manage := "/api/v1/tenant/projects/:id", ManagementScopeTenantTargetProject, services.PermissionTenantProjectMembersRead, services.PermissionTenantProjectMembersManage
		if project {
			base, scope, read, manage = "/api/v1/:tenantSlug/:projectSlug/tenant-auth", ManagementScopeProject, services.PermissionProjectMembersRead, services.PermissionProjectMembersManage
		}
		result = append(result, contract("GET", base+"/members", IdentityManagement, scope, read), contract("PATCH", base+"/members/:userId", IdentityManagement, scope, manage), contract("DELETE", base+"/members/:userId", IdentityManagement, scope, manage))
	}
	return result
}
func init() {
	approvedManagementRouteContracts = append(approvedManagementRouteContracts, collaborationRouteContracts()...)
}
func ProjectCollaborationRoutes(app *fiber.App) {
	projectInvitationAdministrationRoutes(app)
	for _, path := range []string{"/api/v1/tenant/auth/collaboration-capabilities", "/api/v1/:tenantSlug/:projectSlug/tenant-auth/collaboration-capabilities"} {
		app.Get(path, strictManagementRouteHandlers("GET", path, middlewares.GeneralRateLimit(), controllers.GetCollaborationCapabilities)...)
	}
	for _, base := range []string{"/api/v1/tenant/projects/:id", "/api/v1/:tenantSlug/:projectSlug/tenant-auth"} {
		app.Get(base+"/members", strictManagementRouteHandlers("GET", base+"/members", controllers.CollaborationFeature("members"), middlewares.GeneralRateLimit(), controllers.ListProjectCollaborators)...)
		app.Patch(base+"/members/:userId", strictManagementRouteHandlers("PATCH", base+"/members/:userId", controllers.CollaborationFeature("members"), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.ChangeProjectCollaboratorRole)...)
		app.Delete(base+"/members/:userId", strictManagementRouteHandlers("DELETE", base+"/members/:userId", controllers.CollaborationFeature("members"), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.RemoveProjectCollaborator)...)
	}
}
