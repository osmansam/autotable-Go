package controllers

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func InvitationMutationOrigin(c *fiber.Ctx) error {
	setAuthNoStoreHeaders(c)
	c.Set("Referrer-Policy", "no-referrer")
	if !enforceAuthMutationOrigin(c) {
		return nil
	}
	return c.Next()
}
func invitationError(c *fiber.Ctx, err error) error {
	status, code, message := 503, "invitation_unavailable", "Invitations are temporarily unavailable. Please try again."
	switch {
	case errors.Is(err, services.ErrCollaborationInvalid):
		status, code, message = 400, "invitation_invalid_request", "Check the invitation details and try again."
	case errors.Is(err, services.ErrInvitationInvalid), errors.Is(err, repositories.ErrCollaborationInactive), errors.Is(err, mongo.ErrNoDocuments):
		status, code, message = 410, "invitation_invalid", "This invitation is no longer available. Ask an administrator for a new invitation."
	case errors.Is(err, services.ErrInvitationOwnership):
		status, code, message = 401, "invitation_account_action_required", "Sign in with the invited account. If you cannot sign in, use the separate account recovery process."
	case errors.Is(err, services.ErrInvitationReauthentication):
		status, code, message = 401, "invitation_reauthenticate", "Sign in again before joining this project."
	case errors.Is(err, services.ErrInvitationAccountMismatch):
		status, code, message = 409, "invitation_account_mismatch", "Sign out of the current account, then continue with the invited account."
	case errors.Is(err, services.ErrInvitationVerification):
		status, code, message = 400, "invitation_verification_invalid", "The verification code is invalid or expired."
	case errors.Is(err, services.ErrInvitationMailboxRequired):
		status, code, message = 409, "invitation_verify_email", "Verify your email before joining this project."
	case errors.Is(err, services.ErrInvitationConflict), mongo.IsDuplicateKeyError(err):
		status, code, message = 409, "invitation_conflict", "The invitation changed. Refresh the list before trying again."
	case errors.Is(err, services.ErrInvitationMemberExists):
		status, code, message = 409, "invitation_already_member", "Project access already exists. Use member management to change that access."
	case errors.Is(err, services.ErrCollaborationVersion):
		status, code, message = 409, "invitation_version", "The invitation changed. Refresh the list before trying again."
	case errors.Is(err, services.ErrManagementForbidden):
		status, code, message = 403, "invitation_forbidden", "This account cannot join or manage this project."
	}
	setAuthNoStoreHeaders(c)
	return c.Status(status).JSON(responses.GeneralResponse{Status: status, Message: message, Data: &fiber.Map{"code": code}})
}
func invitationServices() (*services.InvitationService, *services.InvitationIdentityService, error) {
	cfg, keys, err := services.ValidateInvitationConfiguration()
	if err != nil {
		return nil, nil, err
	}
	db := collaborationDatabase()
	return services.NewInvitationService(db, cfg, keys), services.NewInvitationIdentityService(db, cfg, keys), nil
}
func currentInvitationAccount(c *fiber.Ctx, db *mongo.Database) (*models.User, error) {
	var claims *utils.TenantTokenClaims
	repo := repositories.NewAuthSessionRepositoryWithCollection(db.Collection("auth_sessions"))
	for _, refresh := range []bool{false, true} {
		kind := "access"
		if refresh {
			kind = "refresh"
		}
		raw := c.Cookies(utils.AuthCookieName(utils.TokenScopeTenant, kind))
		if raw == "" {
			continue
		}
		var parsed *utils.TenantTokenClaims
		var err error
		if refresh {
			parsed, err = utils.ParseTenantRefreshToken(raw)
		} else {
			parsed, err = utils.ParseTenantToken(raw)
		}
		if err != nil || parsed.Scope != utils.TokenScopeTenant || parsed.RoleScope != string(models.RoleScopeTenant) || parsed.ProjectID != "" {
			continue
		}
		principal := models.ManagementPrincipal{UserID: parsed.UserID, TenantID: parsed.TenantID, Scope: parsed.Scope, FamilyID: parsed.FamilyID}
		if refresh {
			err = repo.ValidateManagementRefreshIdentity(c.UserContext(), principal, parsed.Id, time.Now())
		} else {
			err = repo.ValidateManagementFamily(c.UserContext(), principal, time.Now())
		}
		if errors.Is(err, repositories.ErrManagementSessionInvalid) {
			continue
		}
		if err != nil {
			return nil, err
		}
		claims = parsed
		break
	}
	if claims == nil {
		return nil, nil
	}
	id, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, nil
	}
	account, err := repositories.NewManagementAccountRepository(db).GetByID(c.UserContext(), id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !account.IsActive {
		return nil, nil
	}
	return account, nil
}
func requireMatchingInvitationAccount(c *fiber.Ctx, db *mongo.Database, userID string, create bool) error {
	current, err := currentInvitationAccount(c, db)
	if err != nil {
		return err
	}
	if current != nil && (create || current.ID.Hex() != userID) {
		return services.ErrInvitationAccountMismatch
	}
	return nil
}
func decorateInvitationPreview(c *fiber.Ctx, handle string, preview *models.InvitationPreview) error {
	db := collaborationDatabase()
	account, err := currentInvitationAccount(c, db)
	if err != nil {
		return err
	}
	if account == nil {
		return nil
	}
	continuation, err := repositories.NewManagementInvitationContinuationRepository(db).GetByHandle(c.UserContext(), handle, time.Now())
	if err != nil {
		return err
	}
	invite, err := repositories.NewManagementInvitationRepository(db).GetByID(c.UserContext(), continuation.InvitationID)
	if err != nil {
		return err
	}
	if account.NormalizedEmail != invite.NormalizedEmail || (!continuation.UserID.IsZero() && account.ID != continuation.UserID) {
		preview.Stage = "switch_account"
	}
	return nil
}
func ExchangeProjectInvitation(c *fiber.Ctx) error {
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return invitationError(c, err)
	}
	service := services.NewInvitationContinuationService(collaborationDatabase())
	handle, preview, err := service.Exchange(c.UserContext(), body.Token)
	if err != nil {
		return invitationError(c, err)
	}
	if old := c.Cookies(utils.InvitationContinuationCookieName()); old != "" {
		if err := service.Cancel(c.UserContext(), old); err != nil {
			_ = service.Cancel(c.UserContext(), handle)
			return invitationError(c, err)
		}
	}
	if err := decorateInvitationPreview(c, handle, &preview); err != nil {
		return invitationError(c, err)
	}
	utils.SetInvitationContinuationCookie(c, handle, preview.ContinuationExpiresAt)
	setAuthNoStoreHeaders(c)
	c.Set("Referrer-Policy", "no-referrer")
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Invitation ready", Data: preview})
}
func ResumeProjectInvitation(c *fiber.Ctx) error {
	setAuthNoStoreHeaders(c)
	c.Set("Referrer-Policy", "no-referrer")
	handle := c.Cookies(utils.InvitationContinuationCookieName())
	preview, err := services.NewInvitationContinuationService(collaborationDatabase()).Resume(c.UserContext(), handle)
	if err != nil {
		return invitationError(c, err)
	}
	if err := decorateInvitationPreview(c, handle, &preview); err != nil {
		return invitationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Invitation ready", Data: preview})
}
func CancelProjectInvitationContinuation(c *fiber.Ctx) error {
	handle := c.Cookies(utils.InvitationContinuationCookieName())
	utils.ClearInvitationContinuationCookie(c)
	err := services.NewInvitationContinuationService(collaborationDatabase()).Cancel(c.UserContext(), handle)
	if err != nil {
		return invitationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Invitation progress cleared"})
}
func ReauthenticateProjectInvitation(c *fiber.Ctx) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return invitationError(c, err)
	}
	db := collaborationDatabase()
	current, err := currentInvitationAccount(c, db)
	if err != nil {
		return invitationError(c, err)
	}
	if current != nil && current.NormalizedEmail != models.NormalizeManagementEmail(body.Email) {
		return invitationError(c, services.ErrInvitationAccountMismatch)
	}
	_, identity, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	password := []byte(body.Password)
	defer clear(password)
	handle, proof, err := identity.Reauthenticate(c.UserContext(), c.Cookies(utils.InvitationContinuationCookieName()), body.Email, password)
	if err != nil {
		return invitationError(c, err)
	}
	preview, err := services.NewInvitationContinuationService(db).Resume(c.UserContext(), handle)
	if err != nil {
		return invitationError(c, err)
	}
	utils.SetInvitationContinuationCookie(c, handle, proof.ContinuationExpiresAt)
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Account verified", Data: preview})
}
func RequestProjectInvitationVerification(c *fiber.Ctx) error {
	var body struct{}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return invitationError(c, err)
	}
	_, identity, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	proof, err := identity.ProofForHandle(c.UserContext(), c.Cookies(utils.InvitationContinuationCookieName()))
	if err != nil {
		return invitationError(c, err)
	}
	if err := requireMatchingInvitationAccount(c, collaborationDatabase(), proof.UserID, false); err != nil {
		return invitationError(c, err)
	}
	if err := identity.RequestVerification(c.UserContext(), proof); err != nil {
		return invitationError(c, err)
	}
	return c.Status(202).JSON(responses.GeneralResponse{Status: 202, Message: "Verification email queued", Data: &fiber.Map{"queued": true}})
}
func VerifyProjectInvitationEmail(c *fiber.Ctx) error {
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return invitationError(c, err)
	}
	_, identity, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	handle := c.Cookies(utils.InvitationContinuationCookieName())
	proof, err := identity.ProofForHandle(c.UserContext(), handle)
	if err != nil {
		return invitationError(c, err)
	}
	if err := requireMatchingInvitationAccount(c, collaborationDatabase(), proof.UserID, false); err != nil {
		return invitationError(c, err)
	}
	if err := identity.VerifyEmail(c.UserContext(), proof, strings.TrimSpace(body.Code)); err != nil {
		return invitationError(c, err)
	}
	preview, err := services.NewInvitationContinuationService(collaborationDatabase()).Resume(c.UserContext(), handle)
	if err != nil {
		return invitationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Email verified", Data: preview})
}
func startManagementSessionInDatabase(ctx context.Context, db *mongo.Database, tokens *utils.TenantTokenDetails) error {
	claims, err := utils.ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil {
		return err
	}
	return services.NewAuthSessionService(repositories.NewAuthSessionRepositoryWithCollection(db.Collection("auth_sessions"))).Start(ctx, services.AuthSessionStart{Identity: "management", FamilyID: claims.FamilyID, RefreshTokenID: claims.Id, Scope: claims.Scope, UserID: claims.UserID, TenantID: claims.TenantID, ProjectID: claims.ProjectID, ExpiresAt: time.Unix(tokens.RTExpires, 0)})
}
func invitationSessionTokens(ctx context.Context, db *mongo.Database, result models.AcceptedProjectInvitation) (*utils.TenantTokenDetails, *utils.TenantTokenDetails, error) {
	tenant, err := utils.GenerateTenantTokens(result.User.ID.Hex(), result.User.Email, result.Tenant.ID.Hex(), "", result.TenantRoles, string(models.RoleScopeTenant))
	if err != nil {
		return nil, nil, err
	}
	project, err := utils.GenerateTenantTokens(result.User.ID.Hex(), result.User.Email, result.Tenant.ID.Hex(), result.Project.ID.Hex(), result.Roles, string(models.RoleScopeProject))
	if err != nil {
		return nil, nil, err
	}
	if err := startManagementSessionInDatabase(ctx, db, tenant); err != nil {
		return nil, nil, err
	}
	if err := startManagementSessionInDatabase(ctx, db, project); err != nil {
		claims, _ := utils.ParseTenantRefreshToken(tenant.RefreshToken)
		if claims != nil {
			_ = repositories.NewAuthSessionRepositoryWithCollection(db.Collection("auth_sessions")).RevokeFamily(ctx, claims.FamilyID, time.Now())
		}
		return nil, nil, err
	}
	return tenant, project, nil
}
func AcceptProjectInvitation(c *fiber.Ctx) error {
	var body struct {
		Mode     string `json:"mode"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return invitationError(c, err)
	}
	if (body.Mode != "new" && body.Mode != "existing") || (body.Mode == "existing" && (body.Name != "" || body.Password != "")) {
		return invitationError(c, services.ErrCollaborationInvalid)
	}
	db := collaborationDatabase()
	handle := c.Cookies(utils.InvitationContinuationCookieName())
	input := models.AcceptProjectInvitationInput{ContinuationHandle: handle, CreateAccount: body.Mode == "new", Name: body.Name, Password: []byte(body.Password)}
	defer clear(input.Password)
	if input.CreateAccount {
		if err := requireMatchingInvitationAccount(c, db, "", true); err != nil {
			return invitationError(c, err)
		}
	} else {
		identity := services.NewInvitationIdentityService(db, configs.InvitationMailConfig{}, nil)
		proof, err := identity.ProofForHandle(c.UserContext(), handle)
		if err != nil {
			return invitationError(c, err)
		}
		if err := requireMatchingInvitationAccount(c, db, proof.UserID, false); err != nil {
			return invitationError(c, err)
		}
		input.Proof = proof
	}
	result, err := services.NewInvitationAcceptanceService(db).Accept(c.UserContext(), input)
	if err != nil {
		return invitationError(c, err)
	}
	utils.ClearInvitationContinuationCookie(c)
	tenant, project, err := invitationSessionTokens(c.UserContext(), db, result)
	if err != nil {
		return c.Status(503).JSON(responses.GeneralResponse{Status: 503, Message: "Invitation accepted. Sign in to open your project.", Data: &fiber.Map{"code": "invitation_joined_sign_in_required"}})
	}
	utils.SetAuthCookies(c, utils.TokenScopeTenant, tenantCookieTokens(tenant))
	tenantSlug := result.Project.TenantSlug
	if tenantSlug == "" {
		tenantSlug = result.Tenant.Slug
	}
	utils.SetManagementProjectAuthCookies(c, tenantSlug, result.Project.Slug, tenantCookieTokens(project))
	setAuthNoStoreHeaders(c)
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Project joined", Data: result})
}
func ListProjectInvitations(c *fiber.Ctx) error {
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	issuer, _, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return invitationError(c, services.ErrCollaborationInvalid)
		}
	}
	result, err := issuer.List(c.UserContext(), authority, actor, target, models.CollaborationPageRequest{Limit: limit, AfterID: c.Query("after")})
	if err != nil {
		return invitationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Project invitations retrieved", Data: result})
}
func CreateProjectInvitation(c *fiber.Ctx) error {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return invitationError(c, err)
	}
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	issuer, _, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	result, err := issuer.Create(c.UserContext(), models.CreateProjectInvitationInput{Authority: authority, Actor: actor, Target: target, Email: body.Email, Role: body.Role, IdempotencyKey: c.Get("Idempotency-Key")})
	if err != nil {
		return invitationError(c, err)
	}
	return c.Status(202).JSON(responses.GeneralResponse{Status: 202, Message: "Invitation queued", Data: result})
}
func ResendProjectInvitation(c *fiber.Ctx) error {
	var body struct {
		ExpectedVersion *int64 `json:"expectedVersion"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil || body.ExpectedVersion == nil {
		return invitationError(c, services.ErrCollaborationInvalid)
	}
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	issuer, _, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	result, err := issuer.Resend(c.UserContext(), models.ResendProjectInvitationInput{Authority: authority, Actor: actor, Target: target, InvitationID: c.Params("invitationId"), ExpectedVersion: *body.ExpectedVersion, IdempotencyKey: c.Get("Idempotency-Key")})
	if err != nil {
		return invitationError(c, err)
	}
	return c.Status(202).JSON(responses.GeneralResponse{Status: 202, Message: "Invitation queued", Data: result})
}
func RevokeProjectInvitation(c *fiber.Ctx) error {
	var body struct {
		ExpectedVersion *int64 `json:"expectedVersion"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil || body.ExpectedVersion == nil {
		return invitationError(c, services.ErrCollaborationInvalid)
	}
	authority, actor, target, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	issuer, _, err := invitationServices()
	if err != nil {
		return invitationError(c, err)
	}
	err = issuer.Revoke(c.UserContext(), models.RevokeProjectInvitationInput{Authority: authority, Actor: actor, Target: target, InvitationID: c.Params("invitationId"), ExpectedVersion: *body.ExpectedVersion})
	if err != nil {
		return invitationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Invitation canceled"})
}
func RecoverProjectAdministrator(c *fiber.Ctx) error {
	var body struct {
		UserID          string `json:"userId"`
		ExpectedVersion *int64 `json:"expectedVersion"`
		Reason          string `json:"reason"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil || body.ExpectedVersion == nil {
		return collaborationError(c, services.ErrCollaborationInvalid)
	}
	authority, actor, target, err := collaborationContext(c)
	if err != nil || authority != models.CollaborationAuthorityTenant {
		return collaborationError(c, services.ErrManagementForbidden)
	}
	result, err := services.NewProjectCollaborationService(collaborationDatabase()).RecoverAdmin(c.UserContext(), models.RecoverAdminInput{Actor: actor, Target: target, UserID: body.UserID, ExpectedVersion: *body.ExpectedVersion, Reason: body.Reason})
	if err != nil {
		if errors.Is(err, services.ErrCollaborationRecoveryNotRequired) {
			return c.Status(409).JSON(responses.GeneralResponse{Status: 409, Message: "An active administrator already exists. Use member management to change roles.", Data: &fiber.Map{"code": "collaboration_recovery_not_required"}})
		}
		return collaborationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Project administrator restored", Data: result})
}
func ProjectAdministratorRecoveryCandidates(c *fiber.Ctx) error {
	authority, actor, target, err := collaborationContext(c)
	if err != nil || authority != models.CollaborationAuthorityTenant {
		return collaborationError(c, services.ErrManagementForbidden)
	}
	result, err := services.NewProjectCollaborationService(collaborationDatabase()).RecoveryCandidates(c.UserContext(), actor, target, c.Query("query"))
	if err != nil {
		return collaborationError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Verified tenant members retrieved", Data: &fiber.Map{"items": result}})
}
func SwitchManagementTenant(c *fiber.Ctx) error {
	var body struct {
		TenantID string `json:"tenantId"`
	}
	if err := decodeCollaborationBody(c, &body); err != nil {
		return collaborationError(c, err)
	}
	_, actor, _, err := collaborationContext(c)
	if err != nil {
		return collaborationError(c, err)
	}
	db := collaborationDatabase()
	tenantOID, parseErr := primitive.ObjectIDFromHex(body.TenantID)
	if parseErr != nil {
		return collaborationError(c, services.ErrCollaborationInvalid)
	}
	body.TenantID = tenantOID.Hex()
	target := models.ManagementAuthorizationTarget{TenantID: body.TenantID}
	subject := models.ManagementAuthorizationSubject{UserID: actor.UserID, TenantID: body.TenantID}
	snapshot, err := repositories.NewManagementMembershipRepositoryWithDatabase(db).LoadManagementMembership(c.UserContext(), subject, target)
	if err != nil {
		return collaborationError(c, err)
	}
	roles, err := services.CurrentManagementSessionRoles(snapshot, false)
	if err != nil {
		return collaborationError(c, err)
	}
	userID, err := primitive.ObjectIDFromHex(actor.UserID)
	if err != nil {
		return collaborationError(c, services.ErrManagementForbidden)
	}
	account, err := repositories.NewManagementAccountRepository(db).GetByID(c.UserContext(), userID)
	if err != nil {
		return collaborationError(c, err)
	}
	tenantID, _ := primitive.ObjectIDFromHex(body.TenantID)
	var tenant models.Tenant
	if err := db.Collection("tenants").FindOne(c.UserContext(), bson.M{"_id": tenantID, "isActive": true}).Decode(&tenant); err != nil {
		return collaborationError(c, err)
	}
	tokens, err := utils.GenerateTenantTokens(account.ID.Hex(), account.Email, body.TenantID, "", roles, string(models.RoleScopeTenant))
	if err != nil {
		return collaborationError(c, err)
	}
	if err := startManagementSessionInDatabase(c.UserContext(), db, tokens); err != nil {
		return collaborationError(c, err)
	}
	tenants, err := services.ActiveManagementTenants(c.UserContext(), db, userID)
	if err != nil {
		return collaborationError(c, err)
	}
	utils.SetAuthCookies(c, utils.TokenScopeTenant, tenantCookieTokens(tokens))
	setAuthNoStoreHeaders(c)
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Tenant switched", Data: &fiber.Map{"user": account, "tenant": tenant, "allTenants": tenants, "roles": roles}})
}
