package models

import "time"

type RefreshRotationState string

const (
	RefreshRotationSucceeded        RefreshRotationState = "succeeded"
	RefreshRotationPreviousConflict RefreshRotationState = "previous_conflict"
	RefreshRotationReuse            RefreshRotationState = "reuse"
)

type AuthSession struct {
	FamilyID           string     `bson:"familyId" json:"familyId"`
	CurrentJTIHash     string     `bson:"currentJtiHash" json:"-"`
	PreviousJTIHash    string     `bson:"previousJtiHash,omitempty" json:"-"`
	PreviousValidUntil *time.Time `bson:"previousValidUntil,omitempty" json:"-"`
	Scope              string     `bson:"scope" json:"scope"`
	UserID             string     `bson:"userId" json:"userId"`
	TenantID           string     `bson:"tenantId" json:"tenantId"`
	ProjectID          string     `bson:"projectId,omitempty" json:"projectId,omitempty"`
	ExpiresAt          time.Time  `bson:"expiresAt" json:"expiresAt"`
	RevokedAt          *time.Time `bson:"revokedAt,omitempty" json:"revokedAt,omitempty"`
	CreatedAt          time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt          time.Time  `bson:"updatedAt" json:"updatedAt"`
}
