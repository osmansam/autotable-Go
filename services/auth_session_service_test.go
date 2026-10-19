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

type revokingFakeStore struct {
	fakeAuthSessionStore
	userCalls    [][3]string
	projectCalls [][2]string
}

func (f *revokingFakeStore) RevokeUserProjectSessions(_ context.Context, userID, tenantID, projectID string, _ time.Time) error {
	f.userCalls = append(f.userCalls, [3]string{userID, tenantID, projectID})
	return nil
}

func (f *revokingFakeStore) RevokeProjectSessions(_ context.Context, tenantID, projectID string, _ time.Time) error {
	f.projectCalls = append(f.projectCalls, [2]string{tenantID, projectID})
	return nil
}

func TestProjectSessionRevocationHooksAreScopedAndFailClosed(t *testing.T) {
	store := &revokingFakeStore{}
	svc := NewAuthSessionService(store)
	ctx := context.Background()
	if err := svc.RevokeUserProjectSessions(ctx, "u", "t", "p"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeProjectSessions(ctx, "t", "p"); err != nil {
		t.Fatal(err)
	}
	if len(store.userCalls) != 1 || store.userCalls[0] != [3]string{"u", "t", "p"} || len(store.projectCalls) != 1 || store.projectCalls[0] != [2]string{"t", "p"} {
		t.Fatalf("calls = %v %v", store.userCalls, store.projectCalls)
	}
	if err := svc.RevokeUserProjectSessions(ctx, "", "t", "p"); err == nil {
		t.Fatal("unscoped user revocation must be rejected")
	}
	if err := NewAuthSessionService(&fakeAuthSessionStore{}).RevokeProjectSessions(ctx, "t", "p"); !errors.Is(err, ErrSessionRevocationUnsupported) {
		t.Fatalf("unsupported store error = %v, want fail-closed ErrSessionRevocationUnsupported", err)
	}
}
