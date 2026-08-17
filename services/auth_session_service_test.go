package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
)

type fakeAuthSessionStore struct {
	created       models.AuthSession
	rotationState models.RefreshRotationState
	presentedHash string
	newHash       string
	revokedFamily string
}

func (f *fakeAuthSessionStore) Create(_ context.Context, session models.AuthSession) error {
	f.created = session
	return nil
}

func (f *fakeAuthSessionStore) Rotate(_ context.Context, familyID, presentedHash, newHash string, now, expiresAt time.Time) (models.RefreshRotationState, error) {
	f.presentedHash = presentedHash
	f.newHash = newHash
	return f.rotationState, nil
}

func (f *fakeAuthSessionStore) RevokeFamily(_ context.Context, familyID string, _ time.Time) error {
	f.revokedFamily = familyID
	return nil
}

func TestAuthSessionStartStoresOnlyHashedRefreshIdentifier(t *testing.T) {
	store := &fakeAuthSessionStore{}
	service := NewAuthSessionService(store)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	err := service.Start(context.Background(), AuthSessionStart{
		FamilyID: "family-1", RefreshTokenID: "raw-jti-secret", Scope: "project",
		UserID: "user", TenantID: "tenant", ProjectID: "project", ExpiresAt: expiresAt,
	})

	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if store.created.CurrentJTIHash == "" || store.created.CurrentJTIHash == "raw-jti-secret" {
		t.Fatalf("CurrentJTIHash = %q, want non-empty hash", store.created.CurrentJTIHash)
	}
	if store.created.FamilyID != "family-1" || !store.created.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("created session = %#v", store.created)
	}
}

func TestAuthSessionRotateHashesIdentifiersAndMapsConflict(t *testing.T) {
	store := &fakeAuthSessionStore{rotationState: models.RefreshRotationPreviousConflict}
	service := NewAuthSessionService(store)

	err := service.Rotate(context.Background(), "family-1", "presented-secret", "replacement-secret", time.Now().Add(7*24*time.Hour))

	if !errors.Is(err, ErrRefreshConflict) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshConflict", err)
	}
	if store.presentedHash == "presented-secret" || store.newHash == "replacement-secret" || store.presentedHash == "" || store.newHash == "" {
		t.Fatalf("Rotate hashes = %q, %q", store.presentedHash, store.newHash)
	}
}

func TestAuthSessionRotateRevokesFamilyOnReuse(t *testing.T) {
	store := &fakeAuthSessionStore{rotationState: models.RefreshRotationReuse}
	service := NewAuthSessionService(store)

	err := service.Rotate(context.Background(), "family-1", "reused-secret", "replacement", time.Now().Add(7*24*time.Hour))

	if !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshReuse", err)
	}
	if store.revokedFamily != "family-1" {
		t.Fatalf("revoked family = %q, want family-1", store.revokedFamily)
	}
}
