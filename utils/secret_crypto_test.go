package utils

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testExternalSecretKey(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
}

func TestExternalSecretCryptoRoundTrip(t *testing.T) {
	key := testExternalSecretKey(t)

	ciphertext, err := EncryptExternalSecret("sk_live_secret", key)
	if err != nil {
		t.Fatalf("EncryptExternalSecret() error = %v", err)
	}
	if ciphertext == "" || strings.Contains(ciphertext, "sk_live_secret") {
		t.Fatalf("EncryptExternalSecret() = %q, want non-empty ciphertext without plaintext", ciphertext)
	}

	plaintext, err := DecryptExternalSecret(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptExternalSecret() error = %v", err)
	}
	if plaintext != "sk_live_secret" {
		t.Fatalf("DecryptExternalSecret() = %q, want original secret", plaintext)
	}
}

func TestValidateExternalAPIEncryptionKeyRejectsInvalidKeys(t *testing.T) {
	tests := []string{
		"",
		"not-base64",
		base64.StdEncoding.EncodeToString([]byte("too-short")),
		base64.StdEncoding.EncodeToString([]byte("1234567890123456789012345678901")),
	}
	for _, key := range tests {
		if err := ValidateExternalAPIEncryptionKey(key); err == nil {
			t.Fatalf("ValidateExternalAPIEncryptionKey(%q) error = nil", key)
		}
	}
}

func TestDecryptExternalSecretRejectsTamperedCiphertext(t *testing.T) {
	key := testExternalSecretKey(t)
	ciphertext, err := EncryptExternalSecret("secret", key)
	if err != nil {
		t.Fatalf("EncryptExternalSecret() error = %v", err)
	}

	tampered := ciphertext[:len(ciphertext)-2] + "AA"
	if _, err := DecryptExternalSecret(tampered, key); err == nil {
		t.Fatal("DecryptExternalSecret(tampered) error = nil")
	}
}
