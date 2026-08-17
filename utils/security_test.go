package utils

import (
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGenerateAndParseTokens(t *testing.T) {
	oldSecret := jwtSecret
	jwtSecret = []byte("test-secret")
	t.Cleanup(func() { jwtSecret = oldSecret })

	tokens, err := GenerateTokens("user", "admin", "tenant", "project", "tenant-slug", "project-slug")
	if err != nil {
		t.Fatalf("GenerateTokens() error = %v", err)
	}
	userID, role, tenantID, projectID, tenantSlug, projectSlug, err := ParseToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if userID != "user" || role != "admin" || tenantID != "tenant" || projectID != "project" || tenantSlug != "tenant-slug" || projectSlug != "project-slug" {
		t.Fatalf("claims = (%q, %q, %q, %q, %q, %q)", userID, role, tenantID, projectID, tenantSlug, projectSlug)
	}
	if _, _, _, _, _, _, err := ParseToken("invalid"); err == nil {
		t.Fatal("ParseToken(invalid) error = nil")
	}
	if _, _, _, _, _, _, err := ParseToken(tokens.RefreshToken); err == nil {
		t.Fatal("ParseToken(refresh token) error = nil")
	}
	refreshClaims, err := ParseProjectRefreshToken(tokens.RefreshToken)
	if err != nil {
		t.Fatalf("ParseProjectRefreshToken() error = %v", err)
	}
	if refreshClaims.TokenType != TokenTypeRefresh || refreshClaims.Scope != TokenScopeProject || refreshClaims.Audience != ProjectTokenAudience {
		t.Fatalf("refresh claims = %#v", refreshClaims)
	}
	if refreshClaims.ID == "" || refreshClaims.FamilyID == "" {
		t.Fatalf("refresh identifiers missing: %#v", refreshClaims)
	}
	if _, err := ParseProjectRefreshToken(tokens.AccessToken); err == nil {
		t.Fatal("ParseProjectRefreshToken(access token) error = nil")
	}

	tokens, err = GenerateTokensWithDisplayName("user", "admin", "tenant", "project", "tenant-slug", "project-slug", "Ada")
	if err != nil {
		t.Fatalf("GenerateTokensWithDisplayName() error = %v", err)
	}
	if got := ParseTokenDisplayName(tokens.AccessToken); got != "Ada" {
		t.Fatalf("ParseTokenDisplayName() = %q, want Ada", got)
	}
}

func TestGenerateProjectTokensForFamilyPreservesFamilyAndRotatesJTI(t *testing.T) {
	oldSecret := jwtSecret
	jwtSecret = []byte("test-secret")
	t.Cleanup(func() { jwtSecret = oldSecret })

	first, err := GenerateTokensForFamily("user", "admin", "tenant", "project", "davinci", "goblin", "Ada", "family-1")
	if err != nil {
		t.Fatalf("first GenerateTokensForFamily() error = %v", err)
	}
	second, err := GenerateTokensForFamily("user", "admin", "tenant", "project", "davinci", "goblin", "Ada", "family-1")
	if err != nil {
		t.Fatalf("second GenerateTokensForFamily() error = %v", err)
	}
	firstClaims, _ := ParseProjectRefreshToken(first.RefreshToken)
	secondClaims, _ := ParseProjectRefreshToken(second.RefreshToken)
	if firstClaims.FamilyID != "family-1" || secondClaims.FamilyID != "family-1" || firstClaims.ID == secondClaims.ID {
		t.Fatalf("claims = %#v, %#v", firstClaims, secondClaims)
	}
}

func TestGenerateAndParseTenantTokens(t *testing.T) {
	oldSecret := tenantJwtSecret
	t.Cleanup(func() { tenantJwtSecret = oldSecret })

	tenantJwtSecret = nil
	if _, err := GenerateTenantTokens("user", "user@example.com", "tenant", "", []string{models.TenantRoleOwner}, string(models.RoleScopeTenant)); err == nil {
		t.Fatal("GenerateTenantTokens() without secret error = nil")
	}

	tenantJwtSecret = []byte("tenant-test-secret")
	tokens, err := GenerateTenantTokens("user", "user@example.com", "tenant", "project", []string{models.ProjectRoleAdmin}, string(models.RoleScopeProject))
	if err != nil {
		t.Fatalf("GenerateTenantTokens() error = %v", err)
	}
	claims, err := ParseTenantToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ParseTenantToken() error = %v", err)
	}
	if claims.UserID != "user" || claims.Email != "user@example.com" || claims.TenantID != "tenant" || claims.ProjectID != "project" || claims.RoleScope != string(models.RoleScopeProject) {
		t.Fatalf("claims = %#v", claims)
	}
	if _, err := ParseTenantToken("invalid"); err == nil {
		t.Fatal("ParseTenantToken(invalid) error = nil")
	}
	if _, err := ParseTenantToken(tokens.RefreshToken); err == nil {
		t.Fatal("ParseTenantToken(refresh token) error = nil")
	}
	refreshClaims, err := ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil {
		t.Fatalf("ParseTenantRefreshToken() error = %v", err)
	}
	if refreshClaims.TokenType != TokenTypeRefresh || refreshClaims.Scope != TokenScopeProject || refreshClaims.Audience != TenantTokenAudience {
		t.Fatalf("tenant refresh claims = %#v", refreshClaims)
	}
	if _, err := ParseTenantRefreshToken(tokens.AccessToken); err == nil {
		t.Fatal("ParseTenantRefreshToken(access token) error = nil")
	}
}

func TestGenerateTenantTokensForFamilyPreservesFamily(t *testing.T) {
	oldSecret := tenantJwtSecret
	tenantJwtSecret = []byte("tenant-test-secret")
	t.Cleanup(func() { tenantJwtSecret = oldSecret })

	tokens, err := GenerateTenantTokensForFamily("user", "user@example.com", "tenant", "", []string{models.TenantRoleOwner}, string(models.RoleScopeTenant), "family-tenant")
	if err != nil {
		t.Fatalf("GenerateTenantTokensForFamily() error = %v", err)
	}
	claims, err := ParseTenantRefreshToken(tokens.RefreshToken)
	if err != nil || claims.FamilyID != "family-tenant" {
		t.Fatalf("claims = %#v, error = %v", claims, err)
	}
}

func TestMembershipPlaceholdersReturnNotImplemented(t *testing.T) {
	id := primitive.NewObjectID()
	if _, err := ValidateTenantMembership(id, id); err == nil {
		t.Fatal("ValidateTenantMembership() error = nil")
	}
	if _, err := ValidateProjectMembership(id, id, id); err == nil {
		t.Fatal("ValidateProjectMembership() error = nil")
	}
}

func TestPasswordHelpers(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !CheckPasswordHash("secret", hash) || CheckPasswordHash("wrong", hash) {
		t.Fatal("CheckPasswordHash() returned incorrect result")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	first, second := GenerateRefreshToken(), GenerateRefreshToken()
	if first == second || len(first) == 0 {
		t.Fatalf("GenerateRefreshToken() = %q, %q", first, second)
	}
}

func TestGoogleOAuthConfig(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "https://example.com/callback")
	cfg := GetGoogleOAuthConfig()
	if cfg.ClientID != "client" || cfg.ClientSecret != "secret" || cfg.RedirectURL != "https://example.com/callback" || len(cfg.Scopes) != 2 {
		t.Fatalf("GetGoogleOAuthConfig() = %#v", cfg)
	}
	if got := GetGoogleOAuthConfigWithRedirect("https://example.com/other"); !strings.HasSuffix(got.RedirectURL, "/other") {
		t.Fatalf("RedirectURL = %q", got.RedirectURL)
	}
}
