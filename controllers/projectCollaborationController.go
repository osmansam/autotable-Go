package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/services"
	"go.mongodb.org/mongo-driver/mongo"
	"io"
	"strconv"
	"strings"
)

var collaborationDatabase = func() *mongo.Database { return configs.GetCollection("projects").Database() }

func OverrideCollaborationDatabase(db *mongo.Database) func() {
	previous := collaborationDatabase
	collaborationDatabase = func() *mongo.Database { return db }
	return func() { collaborationDatabase = previous }
}

var readCollaborationFeatures = func(ctx context.Context) (services.CollaborationFeatures, error) {
	return services.ReadCollaborationFeatures(ctx, collaborationDatabase())
}

func CollaborationFeature(feature string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		flags := configs.GetCollaborationConfig()
		if !flags.MembersEnabled {
			return c.SendStatus(404)
		}
		features, err := readCollaborationFeatures(c.UserContext())
		if err != nil {
			return collaborationError(c, err)
		}
		enabled := features.MembersEnabled
		switch feature {
		case "invitations":
			enabled = features.InvitationsEnabled
		case "recovery":
			enabled = features.RecoveryEnabled
		}
		if !enabled {
			return c.SendStatus(404)
		}
		c.SetUserContext(repositories.WithCollaborationReadiness(c.UserContext()))
		return c.Next()
	}
}
func collaborationContext(c *fiber.Ctx) (models.CollaborationAuthority, models.ManagementAuthorizationSubject, models.ManagementAuthorizationTarget, error) {
	p, ok := middlewares.ManagementPrincipalFromContext(c)
	if !ok {
		return "", models.ManagementAuthorizationSubject{}, models.ManagementAuthorizationTarget{}, services.ErrManagementForbidden
	}
	authority := models.CollaborationAuthorityTenant
	projectID := c.Params("id")
	if p.Scope == "project" {
		authority = models.CollaborationAuthorityProject
		projectID = p.ProjectID
	}
	return authority, models.ManagementAuthorizationSubject{UserID: p.UserID, TenantID: p.TenantID}, models.ManagementAuthorizationTarget{TenantID: p.TenantID, ProjectID: projectID}, nil
}
func collaborationError(c *fiber.Ctx, err error) error {
	status, code, message := 503, "collaboration_unavailable", "Collaboration is temporarily unavailable"
	switch {
	case errors.Is(err, services.ErrManagementForbidden), errors.Is(err, repositories.ErrCollaborationInactive):
		status, code, message = 403, "collaboration_forbidden", "Collaboration permission denied"
	case errors.Is(err, mongo.ErrNoDocuments):
		status, code, message = 404, "collaboration_not_found", "Collaborator or project not found"
	case errors.Is(err, services.ErrCollaborationInvalid):
		status, code, message = 400, "collaboration_invalid", "Invalid collaboration request"
	case errors.Is(err, services.ErrCollaborationVersion), errors.Is(err, repositories.ErrCollaborationConflict):
		status, code, message = 409, "collaboration_version", "The member changed. Reload before editing."
	case errors.Is(err, services.ErrCollaborationLegacyRoles):
		status, code, message = 409, "collaboration_legacy_roles", "Confirm conversion of the complete existing role set."
	case errors.Is(err, services.ErrCollaborationLastAdmin):
		status, code, message = 409, "collaboration_last_admin", "Appoint another project administrator before removing this access."
	case errors.Is(err, services.ErrCollaborationRecoveryRequired):
		status, code, message = 409, "collaboration_recovery_required", "This project requires verified administrator recovery."
	}
	return c.Status(status).JSON(responses.GeneralResponse{Status: status, Message: message, Data: &fiber.Map{"code": code}})
}
func decodeCollaborationBody(c *fiber.Ctx, value interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Body())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return services.ErrCollaborationInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return services.ErrCollaborationInvalid
	}
	return nil
}
func ListProjectCollaborators(c *fiber.Ctx) error {
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return collaborationError(c, services.ErrCollaborationInvalid)
		}
	}
	result, err := services.NewProjectCollaborationService(collaborationDatabase()).List(c.UserContext(), authority, actor, target, models.CollaborationPageRequest{Limit: limit, AfterID: c.Query("after")})
	if err != nil {
		return collaborationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Project collaborators retrieved", Data: result})
}
func ChangeProjectCollaboratorRole(c *fiber.Ctx) error {
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	var body struct {
		Role                 string   `json:"role"`
		ExpectedVersion      *int64   `json:"expectedVersion"`
		ExpectedRoles        []string `json:"expectedRoles"`
		ConvertLegacyRoleSet bool     `json:"convertLegacyRoleSet"`
		ActivateMembership   bool     `json:"activateMembership"`
		Reason               string   `json:"reason"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil || body.ExpectedVersion == nil || len(body.Reason) > 500 {
		return collaborationError(c, services.ErrCollaborationInvalid)
	}
	result, err := services.NewProjectCollaborationService(collaborationDatabase()).ChangeRole(c.UserContext(), models.RoleChangeInput{Authority: authority, Actor: actor, Target: target, UserID: c.Params("userId"), Role: body.Role, ExpectedVersion: *body.ExpectedVersion, ExpectedRoles: body.ExpectedRoles, ConvertLegacyRoleSet: body.ConvertLegacyRoleSet, ActivateMembership: body.ActivateMembership, Reason: strings.TrimSpace(body.Reason)})
	if err != nil {
		return collaborationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Project collaborator updated", Data: result})
}
func RemoveProjectCollaborator(c *fiber.Ctx) error {
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	var body struct {
		ExpectedVersion *int64 `json:"expectedVersion"`
		Reason          string `json:"reason"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil || body.ExpectedVersion == nil || len(body.Reason) > 500 {
		return collaborationError(c, services.ErrCollaborationInvalid)
	}
	err = services.NewProjectCollaborationService(collaborationDatabase()).Remove(c.UserContext(), models.RemoveMemberInput{Authority: authority, Actor: actor, Target: target, UserID: c.Params("userId"), ExpectedVersion: *body.ExpectedVersion, Reason: strings.TrimSpace(body.Reason)})
	if err != nil {
		return collaborationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Project access removed", Data: &fiber.Map{"removed": true}})
}
func GetCollaborationCapabilities(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	p, ok := middlewares.ManagementPrincipalFromContext(c)
	if !ok {
		return c.SendStatus(401)
	}
	features, err := readCollaborationFeatures(c.UserContext())
	if err != nil {
		return collaborationError(c, err)
	}
	actor, target := models.ManagementAuthorizationSubject{UserID: p.UserID, TenantID: p.TenantID}, models.ManagementAuthorizationTarget{TenantID: p.TenantID, ProjectID: p.ProjectID}
	snapshot, err := loadManagementMembership(c.UserContext(), actor, target)
	if err != nil {
		return collaborationError(c, err)
	}
	permission := services.PermissionTenantProjectsUpdate
	if p.Scope == "project" {
		permission = services.PermissionProjectMembersManage
	}
	canManage := features.MembersEnabled && services.AuthorizeManagementSnapshot(actor, target, permission, snapshot) == nil
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Collaboration capabilities", Data: &fiber.Map{"membersEnabled": features.MembersEnabled, "invitationsEnabled": features.InvitationsEnabled, "recoveryEnabled": features.RecoveryEnabled, "canManageMembers": canManage, "canInvite": canManage && features.InvitationsEnabled, "canRecover": canManage && features.RecoveryEnabled && p.Scope == "tenant", "tenantId": p.TenantID, "projectId": p.ProjectID, "scope": p.Scope}})
}
