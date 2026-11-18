package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type CollaborationAuthority string

const (
	CollaborationAuthorityTenant  CollaborationAuthority = "tenant"
	CollaborationAuthorityProject CollaborationAuthority = "project"
)

type CollaborationPageRequest struct {
	Limit   int
	AfterID string
}
type ProjectCollaborator struct {
	ID        primitive.ObjectID `json:"id"`
	UserID    primitive.ObjectID `json:"userId"`
	Name      string             `json:"name"`
	Email     string             `json:"email"`
	Roles     []string           `json:"roles"`
	Status    MembershipStatus   `json:"status"`
	Version   int64              `json:"version"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}
type ProjectCollaboratorPage struct {
	Items            []ProjectCollaborator `json:"items"`
	NextCursor       string                `json:"nextCursor,omitempty"`
	RecoveryRequired bool                  `json:"recoveryRequired"`
}

type CollaborationRecoveryCandidate struct {
	MembershipVersion int64              `bson:"membershipVersion" json:"membershipVersion"`
	ID                primitive.ObjectID `bson:"_id" json:"id"`
	Name              string             `bson:"name" json:"name"`
	Email             string             `bson:"email" json:"email"`
}
type RoleChangeInput struct {
	Authority            CollaborationAuthority
	Actor                ManagementAuthorizationSubject
	Target               ManagementAuthorizationTarget
	UserID               string
	Role                 string
	ExpectedVersion      int64
	ExpectedRoles        []string
	ConvertLegacyRoleSet bool
	ActivateMembership   bool
	Reason               string
}
type RemoveMemberInput struct {
	Authority       CollaborationAuthority
	Actor           ManagementAuthorizationSubject
	Target          ManagementAuthorizationTarget
	UserID          string
	ExpectedVersion int64
	Reason          string
}
type RecoverAdminInput struct {
	Actor           ManagementAuthorizationSubject
	Target          ManagementAuthorizationTarget
	UserID          string
	ExpectedVersion int64
	Reason          string
}
