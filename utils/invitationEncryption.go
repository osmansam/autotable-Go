package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var errInvitationCiphertext = errors.New("invitation delivery encryption unavailable or invalid")

type InvitationKeyring struct {
	active string
	keys   map[string][]byte
}

func NewInvitationKeyring(raw, active string) (*InvitationKeyring, error) {
	var encoded map[string]string
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil || len(encoded) == 0 || len(encoded) > 32 {
		return nil, errInvitationCiphertext
	}
	keys := map[string][]byte{}
	for id, value := range encoded {
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`).MatchString(id) {
			return nil, errInvitationCiphertext
		}
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 {
			return nil, errInvitationCiphertext
		}
		keys[id] = key
	}
	if keys[active] == nil {
		return nil, errInvitationCiphertext
	}
	return &InvitationKeyring{active: active, keys: keys}, nil
}
func (k *InvitationKeyring) gcm(id string) (cipher.AEAD, error) {
	if k == nil || len(k.keys[id]) != 32 {
		return nil, errInvitationCiphertext
	}
	block, err := aes.NewCipher(k.keys[id])
	if err != nil {
		return nil, errInvitationCiphertext
	}
	return cipher.NewGCM(block)
}
func (k *InvitationKeyring) Seal(plaintext []byte, aad string) (string, error) {
	if k == nil || len(plaintext) > 64*1024 {
		return "", errInvitationCiphertext
	}
	gcm, err := k.gcm(k.active)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errInvitationCiphertext
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, []byte(aad))
	return "v1." + k.active + "." + base64.RawURLEncoding.EncodeToString(sealed), nil
}
func (k *InvitationKeyring) Open(encoded, aad string) ([]byte, error) {
	if len(encoded) > 128*1024 {
		return nil, errInvitationCiphertext
	}
	parts := strings.Split(encoded, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return nil, errInvitationCiphertext
	}
	gcm, err := k.gcm(parts[1])
	if err != nil {
		return nil, err
	}
	sealed, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errInvitationCiphertext
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(aad))
	if err != nil {
		return nil, errInvitationCiphertext
	}
	return plaintext, nil
}
func HashInvitationToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func GenerateInvitationToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, HashInvitationToken(token), nil
}
