package configs

import (
	"bytes"
	"errors"
	"os"
)

// DiagnosticsToken is optional: empty disables diagnostics. An invalid configured
// token is a startup error, and must never fall back to a signing/service key.
func DiagnosticsToken() (string, error) {
	token := os.Getenv("OBSERVABILITY_TOKEN")
	if token == "" {
		return "", nil
	}
	key, err := ValidateJWTSecret(token)
	if err != nil {
		return "", errors.New("OBSERVABILITY_TOKEN must be a cryptographically generated hex/base64 key of at least 32 bytes")
	}
	for _, name := range []string{"JWT_SECRET", "TENANT_JWT_SECRET", "CONTAINER_JWT_SECRET", "EXTERNAL_API_CREDENTIAL_KEY", "OZMISHOW_RATE_LIMIT_SERVICE_SECRET"} {
		other := os.Getenv(name)
		decoded, _ := ValidateJWTSecret(other)
		if token == other || bytes.Equal(key, decoded) {
			return "", errors.New("OBSERVABILITY_TOKEN must be distinct from application credentials")
		}
	}
	return token, nil
}
