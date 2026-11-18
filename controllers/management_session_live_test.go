package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"net/http/httptest"
	"testing"
)

func TestManagementSessionReportsLiveRoles(t *testing.T) {
	allowManagementSessions(t)
	t.Setenv("TENANT_JWT_SECRET", "live-session-test-secret")
	previous := loadManagementMembership
	t.Cleanup(func() { loadManagementMembership = previous })
	loadManagementMembership = func(context.Context, models.ManagementAuthorizationSubject, models.ManagementAuthorizationTarget) (models.ManagementMembershipSnapshot, error) {
		return models.ManagementMembershipSnapshot{UserActive: true, TenantActive: true, TenantMembership: &models.TenantMembership{Status: models.MembershipStatusActive, Roles: []string{models.TenantRoleMember}}}, nil
	}
	tokens, err := utils.GenerateTenantTokens("user", "user@example.com", "tenant", "", []string{models.TenantRoleOwner}, string(models.RoleScopeTenant))
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/", TenantSession)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", utils.AuthCookieName(utils.TokenScopeTenant, "access")+"="+tokens.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Roles []string `json:"roles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Roles) != 1 || result.Roles[0] != models.TenantRoleMember {
		t.Fatalf("stale token roles used: %#v", result.Roles)
	}
}

func TestManagementSessionUnavailableFailsClosed(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", "live-session-test-secret")
	t.Cleanup(middlewares.OverrideManagementSessionValidator(func(context.Context, models.ManagementPrincipal) error { return errors.New("store unavailable") }))
	tokens, _ := utils.GenerateTenantTokens("user", "user@example.com", "tenant", "", []string{models.TenantRoleMember}, string(models.RoleScopeTenant))
	app := fiber.New()
	app.Get("/", TenantSession)
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Cookie", utils.AuthCookieName(utils.TokenScopeTenant, "access")+"="+tokens.AccessToken)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 503 {
		t.Fatalf("store failure incorrectly treated as logout: %d", response.StatusCode)
	}
}
