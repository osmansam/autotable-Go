package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	ExternalAPIAuthTypeBearer = "bearer"
	ExternalAPIAuthTypeHeader = "header"
)

type ExternalAPICredential struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID        primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID       primitive.ObjectID `bson:"projectId" json:"projectId"`
	Name            string             `bson:"name" json:"name"`
	AuthType        string             `bson:"authType" json:"authType"`
	HeaderName      string             `bson:"headerName,omitempty" json:"headerName,omitempty"`
	EncryptedSecret string             `bson:"encryptedSecret" json:"-"`
	AllowedDomains  []string           `bson:"allowedDomains" json:"allowedDomains"`
	ExpiresAt       time.Time          `bson:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	RevokedAt       *time.Time         `bson:"revokedAt,omitempty" json:"revokedAt,omitempty"`
	CreatedBy       primitive.ObjectID `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time          `bson:"updatedAt" json:"updatedAt"`
	LastUsedAt      *time.Time         `bson:"lastUsedAt,omitempty" json:"lastUsedAt,omitempty"`
}
