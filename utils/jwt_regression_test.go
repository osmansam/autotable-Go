package utils

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTValidationBoundaries(t *testing.T) {
	// Equal keys deliberately isolate claim validation from key separation.
	const secret = "test-shared-domain-key"
	t.Setenv("JWT_SECRET", secret)
	t.Setenv("TENANT_JWT_SECRET", secret)
	for _, domain := range []string{"project", "tenant"} {
		t.Run(domain, func(t *testing.T) {
			base := jwt.MapClaims{"user_id": "user", "role": "admin", "tenant_id": "tenant", "project_id": "project", "jti": "id", "family_id": "family", "scope": TokenScopeProject, "token_type": TokenTypeAccess, "aud": ProjectTokenAudience, "exp": time.Now().Add(time.Hour).Unix()}
			if domain == "tenant" {
				base["aud"] = TenantTokenAudience
				base["scope"] = TokenScopeTenant
				base["role_scope"] = "tenant"
			}
			parse := func(s string) error {
				if domain == "tenant" {
					_, err := ParseTenantToken(s)
					return err
				}
				_, _, _, _, _, _, err := ParseToken(s)
				return err
			}
			for _, scenario := range []string{"valid", "refresh", "other-audience", "other-scope", "missing-expiry", "expired", "bad-expiry", "future-nbf", "future-iat", "no-jti", "no-family", "wrong-key", "wrong-algorithm", "none"} {
				t.Run(scenario, func(t *testing.T) {
					claims := jwt.MapClaims{}
					for k, v := range base {
						claims[k] = v
					}
					method := jwt.SigningMethod(jwt.SigningMethodHS256)
					var key interface{} = []byte(secret)
					switch scenario {
					case "refresh":
						claims["token_type"] = TokenTypeRefresh
					case "other-audience":
						claims["aud"] = "other-domain"
					case "other-scope":
						claims["scope"] = "other-scope"
					case "missing-expiry":
						delete(claims, "exp")
					case "expired":
						claims["exp"] = time.Now().Add(-time.Hour).Unix()
					case "bad-expiry":
						claims["exp"] = "tomorrow"
					case "future-nbf":
						claims["nbf"] = time.Now().Add(time.Hour).Unix()
					case "future-iat":
						claims["iat"] = time.Now().Add(time.Hour).Unix()
					case "no-jti":
						claims["jti"] = ""
					case "no-family":
						claims["family_id"] = ""
					case "wrong-key":
						key = []byte("wrong-key")
					case "wrong-algorithm":
						method = jwt.SigningMethodHS384
					case "none":
						method = jwt.SigningMethodNone
						key = jwt.UnsafeAllowNoneSignatureType
					}
					signed, err := jwt.NewWithClaims(method, claims).SignedString(key)
					if err != nil {
						t.Fatal(err)
					}
					err = parse(signed)
					if (err == nil) != (scenario == "valid") {
						t.Fatalf("%s validation error=%v", scenario, err)
					}
				})
			}
		})
	}
}

func TestTokensCannotCrossAuthenticationDomains(t *testing.T) {
	t.Setenv("JWT_SECRET", "same-test-key")
	t.Setenv("TENANT_JWT_SECRET", "same-test-key")
	project, err := GenerateTokens("user", "admin", "tenant", "project", "tenant-slug", "project-slug")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := GenerateTenantTokens("user", "user@example.test", "tenant", "project", []string{"admin"}, "project")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{project.AccessToken, project.RefreshToken} {
		if _, err := ParseTenantToken(token); err == nil {
			t.Fatal("project accepted as tenant access")
		}
		if _, err := ParseTenantRefreshToken(token); err == nil {
			t.Fatal("project accepted as tenant refresh")
		}
	}
	for _, token := range []string{tenant.AccessToken, tenant.RefreshToken} {
		if _, _, _, _, _, _, err := ParseToken(token); err == nil {
			t.Fatal("tenant accepted as project access")
		}
		if _, err := ParseProjectRefreshToken(token); err == nil {
			t.Fatal("tenant accepted as project refresh")
		}
	}
}

func TestTenantAudienceWireFormatRemainsString(t *testing.T) {
	t.Setenv("TENANT_JWT_SECRET", "test-key")
	tokens, err := GenerateTenantTokens("user", "a@example.test", "tenant", "", []string{"owner"}, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tokens.AccessToken, ".")
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]interface{}
	if err = json.Unmarshal(data, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["aud"] != TenantTokenAudience {
		t.Fatalf("audience wire format changed: %#v", claims["aud"])
	}
}
