package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type CollaborationEvent struct {
	RevokeBeforeVersion int64              `bson:"revokeBeforeVersion,omitempty"`
	ID                  primitive.ObjectID `bson:"_id"`
	TenantID            string             `bson:"tenantId"`
	ProjectID           string             `bson:"projectId"`
	RevokeUserID        string             `bson:"revokeUserId,omitempty"`
	Audit               AuditLog           `bson:"audit"`
	Status              string             `bson:"status"`
	NextAttemptAt       time.Time          `bson:"nextAttemptAt"`
	LeaseKey            string             `bson:"leaseKey,omitempty"`
	LeaseUntil          time.Time          `bson:"leaseUntil,omitempty"`
	Attempts            int                `bson:"attempts"`
	CreatedAt           time.Time          `bson:"createdAt"`
}
