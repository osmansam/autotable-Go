package configs

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ValidateJWTSecrets fails closed before database initialization. Environment
// values remain the literal HMAC keys, preserving existing signing semantics.
func ValidateJWTSecrets() error {
	names := []string{"JWT_SECRET", "TENANT_JWT_SECRET"}
	if os.Getenv("CONTAINER_JWT_SECRET") != "" {
		names = append(names, "CONTAINER_JWT_SECRET")
	}
	var keys [][]byte
	for _, name := range names {
		key, err := ValidateJWTSecret(os.Getenv(name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, previous := range keys {
			if bytes.Equal(key, previous) {
				return errors.New("JWT signing secrets must be distinct")
			}
		}
		keys = append(keys, key)
	}
	return nil
}

// ValidateJWTSecret checks encoded key material, not its unknowable provenance.
// GenerateJWTSecret is the supported crypto/rand source. A size/encoding check
// cannot certify that an externally provided key was cryptographically generated.
func ValidateJWTSecret(value string) ([]byte, error) {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 512 {
		return nil, errors.New("use a cryptographically generated key encoded as hex or base64")
	}
	key, err := hex.DecodeString(value)
	if err != nil {
		for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			key, err = encoding.Strict().DecodeString(value)
			if err == nil {
				break
			}
		}
	}
	if err != nil || len(key) < 32 || len(key) > 128 {
		return nil, errors.New("key must encode 32 to 128 random bytes as hex or base64")
	}
	distinct := map[byte]bool{}
	for _, b := range key {
		distinct[b] = true
	}
	if len(distinct) < 16 {
		return nil, errors.New("key material is repetitive; generate a new random key")
	}
	return key, nil
}

// GenerateJWTSecret returns 256 bits from the operating system CSPRNG.
func GenerateJWTSecret() (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(key[:]), nil
}
