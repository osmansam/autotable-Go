package configs

import "testing"

func TestDiagnosticsTokenRejectsReusedAndWeakCredentials(t *testing.T) {
	t.Setenv("JWT_SECRET", strongProjectTestKey)
	for _, tc := range []struct {
		token string
		valid bool
	}{{"", true}, {strongTenantTestKey, true}, {strongProjectTestKey, false}, {"short", false}} {
		t.Setenv("OBSERVABILITY_TOKEN", tc.token)
		_, err := DiagnosticsToken()
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%v err=%v", tc.valid, err)
		}
	}
}
