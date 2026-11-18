package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/services"
)

const invitationAuthBase = "/api/v1/tenant/auth/invitations"

func invitationRouteContracts() []ManagementRouteContract {
	result := []ManagementRouteContract{contract("POST", "/api/v1/tenant/auth/switch-tenant", IdentityManagement, ManagementScopeTenant, services.PermissionTenantAccountRead)}
	for _, route := range []struct{ method, path string }{{"POST", "/exchange"}, {"GET", "/continuation"}, {"DELETE", "/continuation"}, {"POST", "/reauthenticate"}, {"POST", "/verification"}, {"POST", "/verify"}, {"POST", "/accept"}} {
		result = append(result, contract(route.method, invitationAuthBase+route.path, IdentityPublicManagementAuth, ManagementScopeNone, ManagementPermissionNone))
	}
	for _, project := range []bool{false, true} {
		base, scope, read, manage := "/api/v1/tenant/projects/:id", ManagementScopeTenantTargetProject, services.PermissionTenantProjectMembersRead, services.PermissionTenantProjectMembersManage
		if project {
			base, scope, read, manage = "/api/v1/:tenantSlug/:projectSlug/tenant-auth", ManagementScopeProject, services.PermissionProjectMembersRead, services.PermissionProjectMembersManage
		}
		result = append(result, contract("GET", base+"/invitations", IdentityManagement, scope, read), contract("POST", base+"/invitations", IdentityManagement, scope, manage), contract("POST", base+"/invitations/:invitationId/resend", IdentityManagement, scope, manage), contract("DELETE", base+"/invitations/:invitationId", IdentityManagement, scope, manage))
	}
	return append(result, contract("POST", "/api/v1/tenant/projects/:id/members/recover-admin", IdentityManagement, ManagementScopeTenantTargetProject, services.PermissionTenantProjectMembersManage), contract("GET", "/api/v1/tenant/projects/:id/members/recovery-candidates", IdentityManagement, ManagementScopeTenantTargetProject, services.PermissionTenantProjectMembersRead))
}
func init() {
	approvedManagementRouteContracts = append(approvedManagementRouteContracts, invitationRouteContracts()...)
}
func ManagementInvitationRoutes(app *fiber.App) {
	for _, route := range []struct {
		path    string
		handler fiber.Handler
	}{{"/exchange", controllers.ExchangeProjectInvitation}, {"/reauthenticate", controllers.ReauthenticateProjectInvitation}, {"/verification", controllers.RequestProjectInvitationVerification}, {"/verify", controllers.VerifyProjectInvitationEmail}, {"/accept", controllers.AcceptProjectInvitation}} {
		app.Post(invitationAuthBase+route.path, middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.InvitationMutationOrigin, controllers.CollaborationFeature("invitations"), route.handler)
	}
	app.Get(invitationAuthBase+"/continuation", middlewares.PublicRateLimit(), controllers.CollaborationFeature("invitations"), controllers.ResumeProjectInvitation)
	// Browser-owned continuation cancellation remains available while the feature
	// is disabled, just as logout remains available after authentication expires.
	app.Delete(invitationAuthBase+"/continuation", middlewares.DefaultBodySizeLimit(), middlewares.PublicRateLimit(), controllers.InvitationMutationOrigin, controllers.CancelProjectInvitationContinuation)
	app.Post("/api/v1/tenant/auth/switch-tenant", strictManagementRouteHandlers("POST", "/api/v1/tenant/auth/switch-tenant", middlewares.DefaultBodySizeLimit(), middlewares.AuthRateLimit(), controllers.SwitchManagementTenant)...)
}
func projectInvitationAdministrationRoutes(app *fiber.App) {
	for _, base := range []string{"/api/v1/tenant/projects/:id", "/api/v1/:tenantSlug/:projectSlug/tenant-auth"} {
		app.Get(base+"/invitations", strictManagementRouteHandlers("GET", base+"/invitations", controllers.CollaborationFeature("invitations"), middlewares.GeneralRateLimit(), controllers.ListProjectInvitations)...)
		app.Post(base+"/invitations", strictManagementRouteHandlers("POST", base+"/invitations", controllers.CollaborationFeature("invitations"), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.CreateProjectInvitation)...)
		app.Post(base+"/invitations/:invitationId/resend", strictManagementRouteHandlers("POST", base+"/invitations/:invitationId/resend", controllers.CollaborationFeature("invitations"), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.ResendProjectInvitation)...)
		app.Delete(base+"/invitations/:invitationId", strictManagementRouteHandlers("DELETE", base+"/invitations/:invitationId", controllers.CollaborationFeature("invitations"), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.RevokeProjectInvitation)...)
	}
	path := "/api/v1/tenant/projects/:id/members/recover-admin"
	app.Post(path, strictManagementRouteHandlers("POST", path, controllers.CollaborationFeature("recovery"), middlewares.DefaultBodySizeLimit(), middlewares.WriteRateLimit(), controllers.RecoverProjectAdministrator)...)
	path = "/api/v1/tenant/projects/:id/members/recovery-candidates"
	app.Get(path, strictManagementRouteHandlers("GET", path, controllers.CollaborationFeature("recovery"), middlewares.GeneralRateLimit(), controllers.ProjectAdministratorRecoveryCandidates)...)
}
