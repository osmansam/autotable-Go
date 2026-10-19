package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/osmansam/autotableGo/models"
)

var (
	ErrRefreshConflict = errors.New("refresh token was already rotated; retry with the current cookie")
	ErrRefreshReuse    = errors.New("refresh token reuse detected")
)

type AuthSessionStore interface {
	Create(ctx context.Context, session models.AuthSession) error
	Rotate(ctx context.Context, familyID, presentedHash, newHash string, now, expiresAt time.Time) (models.RefreshRotationState, error)
	RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error
}

type AuthSessionStart struct {
	FamilyID       string
	RefreshTokenID string
	Scope          string
	UserID         string
	TenantID       string
	ProjectID      string
	ExpiresAt      time.Time
}

type AuthSessionService struct {
	store AuthSessionStore
	now   func() time.Time
}

func NewAuthSessionService(store AuthSessionStore) *AuthSessionService {
	return &AuthSessionService{store: store, now: time.Now}
}

func (s *AuthSessionService) Start(ctx context.Context, input AuthSessionStart) error {
	now := s.now()
	return s.store.Create(ctx, models.AuthSession{
		FamilyID:       input.FamilyID,
		CurrentJTIHash: hashTokenIdentifier(input.RefreshTokenID),
		Scope:          input.Scope,
		UserID:         input.UserID,
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		ExpiresAt:      input.ExpiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

func (s *AuthSessionService) Rotate(ctx context.Context, familyID, presentedJTI, newJTI string, expiresAt time.Time) error {
	now := s.now()
	state, err := s.store.Rotate(ctx, familyID, hashTokenIdentifier(presentedJTI), hashTokenIdentifier(newJTI), now, expiresAt)
	if err != nil {
		return err
	}
	switch state {
	case models.RefreshRotationSucceeded:
		return nil
	case models.RefreshRotationPreviousConflict:
		return ErrRefreshConflict
	default:
		if err := s.store.RevokeFamily(ctx, familyID, now); err != nil {
			return err
		}
		return ErrRefreshReuse
	}
}

func (s *AuthSessionService) RevokeFamily(ctx context.Context, familyID string) error {
	return s.store.RevokeFamily(ctx, familyID, s.now())
}

func hashTokenIdentifier(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// ProjectSessionRevoker is implemented by session stores that can revoke
// sessions by tenant/project scope. Membership-mutation code must call these
// hooks when a membership is disabled, downgraded, or removed, and when a
// project is deactivated or deleted.
type ProjectSessionRevoker interface {
	RevokeUserProjectSessions(ctx context.Context, userID, tenantID, projectID string, revokedAt time.Time) error
	RevokeProjectSessions(ctx context.Context, tenantID, projectID string, revokedAt time.Time) error
}

var ErrSessionRevocationUnsupported = errors.New("session store cannot revoke project sessions")

// RevokeUserProjectSessions revokes only the user's sessions in the exact tenant/project.
// It fails closed: callers must treat an error as "revocation not guaranteed".
func (s *AuthSessionService) RevokeUserProjectSessions(ctx context.Context, userID, tenantID, projectID string) error {
	revoker, ok := s.store.(ProjectSessionRevoker)
	if !ok {
		return ErrSessionRevocationUnsupported
	}
	if userID == "" || tenantID == "" || projectID == "" {
		return errors.New("user, tenant, and project are required")
	}
	return revoker.RevokeUserProjectSessions(ctx, userID, tenantID, projectID, s.now())
}

// RevokeProjectSessions revokes every session in the tenant/project (deactivation, deletion).
func (s *AuthSessionService) RevokeProjectSessions(ctx context.Context, tenantID, projectID string) error {
	revoker, ok := s.store.(ProjectSessionRevoker)
	if !ok {
		return ErrSessionRevocationUnsupported
	}
	if tenantID == "" || projectID == "" {
		return errors.New("tenant and project are required")
	}
	return revoker.RevokeProjectSessions(ctx, tenantID, projectID, s.now())
}
