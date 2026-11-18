package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type InvitationContinuation struct {
	ID                primitive.ObjectID `bson:"_id" json:"-"`
	InvitationID      primitive.ObjectID `bson:"invitationId" json:"-"`
	InvitationVersion int64              `bson:"invitationVersion" json:"-"`
	HandleHash        string             `bson:"handleHash" json:"-"`
	Purpose           string             `bson:"purpose" json:"-"`
	Status            string             `bson:"status" json:"-"`
	UserID            primitive.ObjectID `bson:"userId,omitempty" json:"-"`
	AccountVersion    int64              `bson:"accountVersion" json:"-"`
	AuthenticatedAt   time.Time          `bson:"authenticatedAt,omitempty" json:"-"`
	ProofExpiresAt    time.Time          `bson:"proofExpiresAt,omitempty" json:"-"`
	ExpiresAt         time.Time          `bson:"expiresAt" json:"-"`
	CreatedAt         time.Time          `bson:"createdAt" json:"-"`
	UpdatedAt         time.Time          `bson:"updatedAt" json:"-"`
}

// AccountVersion tracks credential/email identity changes, not the authorization
// counter touched by grant fences. This value is never supplied by a client.
type InvitationIdentityProof struct {
	ContinuationID        primitive.ObjectID
	UserID                string
	NormalizedEmail       string
	AccountVersion        int64
	AuthenticatedAt       time.Time
	ExpiresAt             time.Time
	ContinuationExpiresAt time.Time
	EmailVerified         bool
}
type ManagementEmailChallenge struct {
	ID                primitive.ObjectID `bson:"_id" json:"-"`
	ContinuationID    primitive.ObjectID `bson:"continuationId" json:"-"`
	InvitationID      primitive.ObjectID `bson:"invitationId" json:"-"`
	InvitationVersion int64              `bson:"invitationVersion" json:"-"`
	UserID            primitive.ObjectID `bson:"userId" json:"-"`
	NormalizedEmail   string             `bson:"normalizedEmail" json:"-"`
	AccountVersion    int64              `bson:"accountVersion" json:"-"`
	Version           int64              `bson:"version" json:"-"`
	TokenHash         string             `bson:"tokenHash" json:"-"`
	Status            string             `bson:"status" json:"-"`
	Attempts          int                `bson:"attempts" json:"-"`
	ExpiresAt         time.Time          `bson:"expiresAt" json:"-"`
	CreatedAt         time.Time          `bson:"createdAt" json:"-"`
	UpdatedAt         time.Time          `bson:"updatedAt" json:"-"`
}

type InvitationPreview struct {
	TenantName                string    `json:"tenantName"`
	ProjectName               string    `json:"projectName"`
	Role                      string    `json:"role"`
	ExpiresAt                 time.Time `json:"expiresAt"`
	ContinuationExpiresAt     time.Time `json:"continuationExpiresAt"`
	Stage                     string    `json:"stage"`
	VerificationDeliveryState string    `json:"verificationDeliveryState,omitempty"`
}
type AcceptProjectInvitationInput struct {
	ContinuationHandle string                  `json:"-"`
	Proof              InvitationIdentityProof `json:"-"`
	CreateAccount      bool                    `json:"-"`
	Name               string                  `json:"-"`
	Password           []byte                  `json:"-"`
}
type AcceptedProjectInvitation struct {
	User        User     `json:"user"`
	Tenant      Tenant   `json:"tenant"`
	AllTenants  []Tenant `json:"allTenants"`
	Project     Project  `json:"project"`
	TenantRoles []string `json:"tenantRoles"`
	Roles       []string `json:"roles"`
}
