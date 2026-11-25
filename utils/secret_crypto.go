package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/osmansam/autotableGo/internal/keymaterial"
	"io"
	"strings"
)

var ErrInvalidExternalAPIEncryptionKey = keymaterial.ErrInvalidExternalAPIEncryptionKey

func ValidateExternalAPIEncryptionKey(encodedKey string) error {
	_, err := decodeExternalAPIEncryptionKey(encodedKey)
	return err
}

func EncryptExternalSecret(plaintext, encodedKey string) (string, error) {
	key, err := decodeExternalAPIEncryptionKey(encodedKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func DecryptExternalSecret(ciphertext, encodedKey string) (string, error) {
	key, err := decodeExternalAPIEncryptionKey(encodedKey)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted external api secret")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted external api secret")
	}
	nonce := data[:gcm.NonceSize()]
	sealed := data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted external api secret")
	}
	return string(plaintext), nil
}

func decodeExternalAPIEncryptionKey(encodedKey string) ([]byte, error) {
	return keymaterial.DecodeExternalAPIEncryptionKey(encodedKey)
}
