package models

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	NotificationTypeInfo    = "info"
	NotificationTypeWarning = "warning"
	NotificationTypeError   = "error"
	NotificationTypeSuccess = "success"
)

func NormalizeNotificationType(notificationType string) string {
	return strings.ToLower(strings.TrimSpace(notificationType))
}

func IsValidNotificationType(notificationType string) bool {
	switch NormalizeNotificationType(notificationType) {
	case NotificationTypeInfo, NotificationTypeWarning, NotificationTypeError, NotificationTypeSuccess:
		return true
	default:
		return false
	}
}

type DynamicNotification struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID      string             `bson:"tenantId" json:"tenantId"`
	ProjectID     string             `bson:"projectId" json:"projectId"`
	Title         string             `bson:"title" json:"title"`
	Message       string             `bson:"message" json:"message"`
	Type          string             `bson:"type" json:"type"`
	Event         string             `bson:"event,omitempty" json:"event,omitempty"`
	SchemaName    string             `bson:"schemaName,omitempty" json:"schemaName,omitempty"`
	RecordID      string             `bson:"recordId,omitempty" json:"recordId,omitempty"`
	SelectedUsers []string           `bson:"selectedUsers,omitempty" json:"selectedUsers,omitempty"`
	SelectedRoles []string           `bson:"selectedRoles,omitempty" json:"selectedRoles,omitempty"`
	SeenBy        []string           `bson:"seenBy,omitempty" json:"seenBy,omitempty"`
	DeletedBy     []string           `bson:"deletedBy,omitempty" json:"deletedBy,omitempty"`
	CreatedBy     string             `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	CreatedAt     time.Time          `bson:"createdAt" json:"createdAt"`
	ExpireAt      *time.Time         `bson:"expireAt,omitempty" json:"expireAt,omitempty"`
	IsActive      bool               `bson:"isActive" json:"isActive"`
}
