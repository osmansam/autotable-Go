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
