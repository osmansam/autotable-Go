package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

/*
MEMBERSHIP STATUS
*/
type MembershipStatus string

const (
	MembershipStatusInvited  MembershipStatus = "invited"
	MembershipStatusActive   MembershipStatus = "active"
	MembershipStatusDisabled MembershipStatus = "disabled"
)

/*
ROLE SCOPE
*/
type RoleScope string

const (
	RoleScopeTenant  RoleScope = "tenant"
	RoleScopeProject RoleScope = "project"
)

/*
SYSTEM ROLE KEYS (restricted)
Validate these on write (membership create/update, invite create/update).
*/

// Tenant roles
const (
	TenantRoleOwner   = "tenant_owner"
	TenantRoleAdmin   = "tenant_admin"
	TenantRoleBilling = "tenant_billing"
	TenantRoleAuditor = "tenant_auditor"
)

// Project roles
const (
	ProjectRoleAdmin     = "project_admin"
	ProjectRoleDeveloper = "project_developer"
	ProjectRoleEditor    = "project_editor"
	ProjectRoleViewer    = "project_viewer"
	ProjectRoleSupport   = "project_support" // Optional support role
)

/*
USER (global)
*/
type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Email     string             `bson:"email" json:"email"`
	Name      string             `bson:"name,omitempty" json:"name,omitempty"`
	IsActive  bool               `bson:"isActive" json:"isActive"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

/*
TENANT (organization)
*/
type Tenant struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name        string             `bson:"name" json:"name"`
	Slug        string             `bson:"slug" json:"slug"`
	OwnerUserID primitive.ObjectID `bson:"ownerUserId" json:"ownerUserId"`
	IsActive    bool               `bson:"isActive" json:"isActive"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

/*
PROJECT (belongs to a tenant)
*/
type Project struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	Name      string             `bson:"name" json:"name"`
	Slug      string             `bson:"slug" json:"slug"`
	IsActive  bool               `bson:"isActive" json:"isActive"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

/*
ROLE REGISTRY (optional but good)
- Use this as an allowlist of roles.
- For MVP: you can skip DB lookups and validate via constants.
- Later: you can add tenant-custom roles by setting TenantID.
*/
type Role struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	Scope       RoleScope           `bson:"scope" json:"scope"` // tenant | project
	Key         string              `bson:"key" json:"key"`
	DisplayName string              `bson:"displayName" json:"displayName"`
	Description string              `bson:"description,omitempty" json:"description,omitempty"`
	TenantID    *primitive.ObjectID `bson:"tenantId,omitempty" json:"tenantId,omitempty"` // nil => system/global
	IsSystem    bool                `bson:"isSystem" json:"isSystem"`
	IsActive    bool                `bson:"isActive" json:"isActive"`
	CreatedAt   time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time           `bson:"updatedAt" json:"updatedAt"`
}

/*
TENANT MEMBERSHIP (user <-> tenant)
*/
type TenantMembership struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	Roles     []string           `bson:"roles" json:"roles"` // tenant role keys only
	Status    MembershipStatus   `bson:"status" json:"status"`
	CreatedBy primitive.ObjectID `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

/*
PROJECT MEMBERSHIP (user <-> project)
*/
type ProjectMembership struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID primitive.ObjectID `bson:"projectId" json:"projectId"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	Roles     []string           `bson:"roles" json:"roles"` // project role keys only
	Status    MembershipStatus   `bson:"status" json:"status"`
	CreatedBy primitive.ObjectID `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

/*
INVITES
*/
type InviteScope string

const (
	InviteScopeTenant  InviteScope = "tenant"
	InviteScopeProject InviteScope = "project"
)

type Invite struct {
	ID            primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	Scope         InviteScope         `bson:"scope" json:"scope"` // tenant | project
	TenantID      primitive.ObjectID  `bson:"tenantId" json:"tenantId"`
	ProjectID     *primitive.ObjectID `bson:"projectId,omitempty" json:"projectId,omitempty"`
	Email         string              `bson:"email" json:"email"`
	RolesToAssign []string            `bson:"rolesToAssign" json:"rolesToAssign"` // must match scope
	TokenHash     string              `bson:"tokenHash" json:"-"`
	Status        MembershipStatus    `bson:"status" json:"status"`
	ExpiresAt     time.Time           `bson:"expiresAt" json:"expiresAt"`
	CreatedBy     primitive.ObjectID  `bson:"createdBy" json:"createdBy"`
	AcceptedBy    *primitive.ObjectID `bson:"acceptedBy,omitempty" json:"acceptedBy,omitempty"`
	AcceptedAt    *time.Time          `bson:"acceptedAt,omitempty" json:"acceptedAt,omitempty"`
	CreatedAt     time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time           `bson:"updatedAt" json:"updatedAt"`
}

/*
CONTAINER SCOPE
- Attach to ContainerModel to ensure structure is project-scoped
- TenantID + ProjectID => ensures structure is project-scoped
- CollectionName => stores "schemaName_<projectIdHex>" once
*/
type ContainerScope struct {
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID primitive.ObjectID `bson:"projectId" json:"projectId"`
}
