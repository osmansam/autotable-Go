package utils

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCollaborationDeliveryEncryptionBoundToVersion(t *testing.T) {
	raw := `{"first":"` + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))) + `"}`
	keys, err := NewInvitationKeyring(raw, "first")
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("https://panel.example.test/invitations/accept#private-invite-token")
	sealed, err := keys.Seal(secret, "invitation:abc:1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "private-invite-token") {
		t.Fatal("plaintext persisted in ciphertext")
	}
	opened, err := keys.Open(sealed, "invitation:abc:1")
	if err != nil || string(opened) != string(secret) {
		t.Fatalf("decrypt failed: %v", err)
	}
	if _, err := keys.Open(sealed, "invitation:abc:2"); err == nil {
		t.Fatal("ciphertext accepted for another invitation version")
	}
	changed := []byte(sealed)
	position := len(changed) - 12
	if changed[position] == 'A' {
		changed[position] = 'B'
	} else {
		changed[position] = 'A'
	}
	if _, err := keys.Open(string(changed), "invitation:abc:1"); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	second, _ := keys.Seal(secret, "invitation:abc:1")
	if second == sealed {
		t.Fatal("encryption nonce reused")
	}
}
func TestCollaborationInvitationKeyRotation(t *testing.T) {
	first, second := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))), base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32)))
	old, _ := NewInvitationKeyring(`{"old":"`+first+`"}`, "old")
	sealed, _ := old.Seal([]byte("token"), "scope")
	rotated, err := NewInvitationKeyring(`{"old":"`+first+`","new":"`+second+`"}`, "new")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.Open(sealed, "scope"); err != nil {
		t.Fatal(err)
	}
	dropped, _ := NewInvitationKeyring(`{"new":"`+second+`"}`, "new")
	if _, err := dropped.Open(sealed, "scope"); err == nil {
		t.Fatal("missing key did not fail closed")
	}
	if _, err := NewInvitationKeyring(`{"bad":"short"}`, "bad"); err == nil {
		t.Fatal("invalid key accepted")
	}
}
func TestCollaborationInvitationTokensHaveIndependentHashes(t *testing.T) {
	token, hash, err := GenerateInvitationToken()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 || token == hash || HashInvitationToken(token) != hash {
		t.Fatal("invalid invitation token encoding/hash")
	}
}
