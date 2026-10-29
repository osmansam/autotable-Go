package models

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	ExternalAPICredentialScopeProject = "project"
	ExternalAPICredentialScopeTenant  = "tenant"
	ExternalAPIAuthTypeBearer         = "bearer"
	ExternalAPIAuthTypeHeader         = "header"
)

type ExternalAPICredential struct {
	ID                primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	TenantID          primitive.ObjectID   `bson:"tenantId" json:"tenantId"`
	ProjectID         primitive.ObjectID   `bson:"projectId,omitempty" json:"projectId,omitempty"`
	Scope             string               `bson:"scope,omitempty" json:"scope"`
	GrantedProjectIDs []primitive.ObjectID `bson:"-" json:"grantedProjectIds"`
	Name              string               `bson:"name" json:"name"`
	AuthType          string               `bson:"authType" json:"authType"`
	HeaderName        string               `bson:"headerName,omitempty" json:"headerName,omitempty"`
	EncryptedSecret   string               `bson:"encryptedSecret" json:"-"`
	AllowedDomains    []string             `bson:"allowedDomains" json:"allowedDomains"`
	ExpiresAt         time.Time            `bson:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	RevokedAt         *time.Time           `bson:"revokedAt,omitempty" json:"revokedAt,omitempty"`
	CreatedBy         primitive.ObjectID   `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	CreatedAt         time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time            `bson:"updatedAt" json:"updatedAt"`
	LastUsedAt        *time.Time           `bson:"lastUsedAt,omitempty" json:"lastUsedAt,omitempty"`
}

func (c ExternalAPICredential) EffectiveScope() string {
	if c.Scope == "" {
		return ExternalAPICredentialScopeProject
	}
	return c.Scope
}

type ExternalAPICredentialGrant struct {
	TenantID     primitive.ObjectID `bson:"tenantId"`
	CredentialID primitive.ObjectID `bson:"credentialId"`
	ProjectID    primitive.ObjectID `bson:"projectId"`
	CreatedBy    primitive.ObjectID `bson:"createdBy"`
	CreatedAt    time.Time          `bson:"createdAt"`
}

// MarshalJSON keeps optional ownership and expiry absent for tenant and non-expiring keys.
func (c ExternalAPICredential) MarshalJSON() ([]byte, error) {
	type alias ExternalAPICredential
	var projectID *primitive.ObjectID
	if c.EffectiveScope() == ExternalAPICredentialScopeProject && !c.ProjectID.IsZero() {
		projectID = &c.ProjectID
	}
	var expiresAt *time.Time
	if !c.ExpiresAt.IsZero() {
		expiresAt = &c.ExpiresAt
	}
	return json.Marshal(struct {
		alias
		ProjectID *primitive.ObjectID `json:"projectId,omitempty"`
		ExpiresAt *time.Time          `json:"expiresAt,omitempty"`
	}{alias: alias(c), ProjectID: projectID, ExpiresAt: expiresAt})
}
