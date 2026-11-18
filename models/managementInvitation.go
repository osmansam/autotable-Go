package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

const (
	InvitationPending  = "pending"
	InvitationAccepted = "accepted"
	InvitationRevoked  = "revoked"
	InvitationExpired  = "expired"
)

type ManagementInvitation struct {
	ID              primitive.ObjectID     `bson:"_id" json:"id"`
	TenantID        primitive.ObjectID     `bson:"tenantId" json:"tenantId"`
	ProjectID       primitive.ObjectID     `bson:"projectId" json:"projectId"`
	NormalizedEmail string                 `bson:"normalizedEmail" json:"email"`
	Role            string                 `bson:"role" json:"role"`
	Authority       CollaborationAuthority `bson:"authority" json:"-"`
	InvitedBy       primitive.ObjectID     `bson:"invitedBy" json:"-"`
	TokenHash       string                 `bson:"tokenHash" json:"-"`
	TokenVersion    int64                  `bson:"tokenVersion" json:"version"`
	Status          string                 `bson:"status" json:"status"`
	DeliveryState   string                 `bson:"deliveryState" json:"deliveryState"`
	ExpiresAt       time.Time              `bson:"expiresAt" json:"expiresAt"`
	AcceptedBy      primitive.ObjectID     `bson:"acceptedBy,omitempty" json:"-"`
	AcceptedAt      *time.Time             `bson:"acceptedAt,omitempty" json:"-"`
	CreatedAt       time.Time              `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time              `bson:"updatedAt" json:"updatedAt"`
}
type InvitationSummary struct {
	ID            primitive.ObjectID `json:"id"`
	TenantID      primitive.ObjectID `json:"tenantId"`
	ProjectID     primitive.ObjectID `json:"projectId"`
	Email         string             `json:"email"`
	Role          string             `json:"role"`
	Version       int64              `json:"version"`
	Status        string             `json:"status"`
	DeliveryState string             `json:"deliveryState"`
	ExpiresAt     time.Time          `json:"expiresAt"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}
type InvitationSummaryPage struct {
	Items      []InvitationSummary `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
}
type CreateProjectInvitationInput struct {
	Authority                   CollaborationAuthority
	Actor                       ManagementAuthorizationSubject
	Target                      ManagementAuthorizationTarget
	Email, Role, IdempotencyKey string
}
type ResendProjectInvitationInput struct {
	Authority                    CollaborationAuthority
	Actor                        ManagementAuthorizationSubject
	Target                       ManagementAuthorizationTarget
	InvitationID, IdempotencyKey string
	ExpectedVersion              int64
}
type RevokeProjectInvitationInput struct {
	Authority       CollaborationAuthority
	Actor           ManagementAuthorizationSubject
	Target          ManagementAuthorizationTarget
	InvitationID    string
	ExpectedVersion int64
}
type InvitationMessage struct {
	To        string `json:"to"`
	MessageID string `json:"messageId"`
	Subject   string `json:"subject"`
	TextBody  string `json:"textBody"`
}

func (InvitationMessage) String() string { return "[redacted invitation email]" }

type InvitationDeliveryJob struct {
	ID               primitive.ObjectID `bson:"_id"`
	JobKey           string             `bson:"jobKey"`
	Purpose          string             `bson:"purpose"`
	SubjectID        primitive.ObjectID `bson:"subjectId"`
	InvitationID     primitive.ObjectID `bson:"invitationId"`
	TenantID         primitive.ObjectID `bson:"tenantId"`
	ProjectID        primitive.ObjectID `bson:"projectId"`
	TokenVersion     int64              `bson:"tokenVersion"`
	EncryptedPayload string             `bson:"encryptedPayload,omitempty" json:"-"`
	MessageID        string             `bson:"messageId"`
	Status           string             `bson:"status"`
	Attempts         int                `bson:"attempts"`
	NextAttemptAt    time.Time          `bson:"nextAttemptAt"`
	LeaseKey         string             `bson:"leaseKey,omitempty"`
	LeaseUntil       time.Time          `bson:"leaseUntil,omitempty"`
	ExpiresAt        time.Time          `bson:"expiresAt"`
	CreatedAt        time.Time          `bson:"createdAt"`
	UpdatedAt        time.Time          `bson:"updatedAt"`
}
type InvitationRequestRecord struct {
	ID         primitive.ObjectID `bson:"_id"`
	RequestKey string             `bson:"requestKey"`
	Digest     string             `bson:"digest"`
	Result     InvitationSummary  `bson:"result"`
	ExpiresAt  time.Time          `bson:"expiresAt"`
}
