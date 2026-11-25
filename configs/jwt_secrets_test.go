package configs

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

const strongProjectTestKey = "b2615c0e92f7483ad6810c9e7f024568a5e330c19bf607d40e8a672d195b3fc4"
const strongTenantTestKey = "c5ae13f4d06897b22c8e106fa471d3506e9c08ab32745f16b09d8e6a1257f3c0"

func TestJWTSecretsRejectWeakAndReusedMaterial(t *testing.T) {
	for _, tc := range []struct {
		name, project, tenant, container string
		valid                            bool
	}{
		{name: "strong distinct", project: strongProjectTestKey, tenant: strongTenantTestKey, valid: true},
		{name: "short", project: "test-secret", tenant: strongTenantTestKey},
		{name: "reused", project: strongProjectTestKey, tenant: strongProjectTestKey},
		{name: "repeated", project: strings.Repeat("ab", 32), tenant: strongTenantTestKey},
		{name: "human sentence", project: "a long but human generated password should not pass", tenant: strongTenantTestKey},
		{name: "whitespace", project: " " + strongProjectTestKey, tenant: strongTenantTestKey},
		{name: "legacy key reused", project: strongProjectTestKey, tenant: strongTenantTestKey, container: strongProjectTestKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", tc.project)
			t.Setenv("TENANT_JWT_SECRET", tc.tenant)
			t.Setenv("CONTAINER_JWT_SECRET", tc.container)
			err := ValidateJWTSecrets()
			if (err == nil) != tc.valid {
				t.Fatalf("validation error=%v valid=%v", err, tc.valid)
			}
			if err != nil && strings.Contains(err.Error(), tc.project) && tc.project != "" {
				t.Fatal("secret exposed in error")
			}
		})
	}
	decoded, _ := hex.DecodeString(strongProjectTestKey)
	t.Setenv("JWT_SECRET", strongProjectTestKey)
	t.Setenv("TENANT_JWT_SECRET", base64.StdEncoding.EncodeToString(decoded))
	t.Setenv("CONTAINER_JWT_SECRET", "")
	if ValidateJWTSecrets() == nil {
		t.Fatal("same underlying key accepted with different encoding")
	}
}
