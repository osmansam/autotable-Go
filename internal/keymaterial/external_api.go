// Package keymaterial validates key encodings without application initializers.
package keymaterial

import (
	"encoding/base64"
	"errors"
	"strings"
)

var ErrInvalidExternalAPIEncryptionKey = errors.New("EXTERNAL_API_CREDENTIAL_KEY must be base64-encoded 32 bytes")

func DecodeExternalAPIEncryptionKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidExternalAPIEncryptionKey
	}
	return key, nil
}

func ValidateExternalAPIEncryptionKey(encoded string) error {
	_, err := DecodeExternalAPIEncryptionKey(encoded)
	return err
}
