package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TranslationOrigin string

const (
	TranslationOriginAI     TranslationOrigin = "ai"
	TranslationOriginManual TranslationOrigin = "manual"
)

type TranslationStatus string

const (
	TranslationStatusCurrent  TranslationStatus = "current"
	TranslationStatusOutdated TranslationStatus = "outdated"
	TranslationStatusFailed   TranslationStatus = "failed"
)

type TranslationEntry struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID       primitive.ObjectID  `bson:"tenantId" json:"tenantId"`
	ProjectID      primitive.ObjectID  `bson:"projectId" json:"projectId"`
	Locale         string              `bson:"locale" json:"locale"`
	TranslationKey string              `bson:"translationKey" json:"translationKey"`
	ResourceType   string              `bson:"resourceType" json:"resourceType"`
	ResourceID     string              `bson:"resourceId" json:"resourceId"`
	PropertyPath   string              `bson:"propertyPath" json:"propertyPath"`
	SourceText     string              `bson:"sourceText" json:"sourceText"`
	SourceHash     string              `bson:"sourceHash" json:"sourceHash"`
	TranslatedText string              `bson:"translatedText" json:"translatedText"`
	Origin         TranslationOrigin   `bson:"origin" json:"origin"`
	Status         TranslationStatus   `bson:"status" json:"status"`
	IsActive       bool                `bson:"isActive" json:"isActive"`
	OrphanedAt     *time.Time          `bson:"orphanedAt,omitempty" json:"orphanedAt,omitempty"`
	LastDiscovered time.Time           `bson:"lastDiscovered" json:"lastDiscovered"`
	CreatedBy      *primitive.ObjectID `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	UpdatedBy      *primitive.ObjectID `bson:"updatedBy,omitempty" json:"updatedBy,omitempty"`
	CreatedAt      time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time           `bson:"updatedAt" json:"updatedAt"`
}

type SourceString struct {
	TranslationKey string `json:"translationKey"`
	ResourceType   string `json:"resourceType"`
	ResourceID     string `json:"resourceId"`
	PropertyPath   string `json:"propertyPath"`
	SourceText     string `json:"sourceText"`
	SourceHash     string `json:"sourceHash"`
	Context        string `json:"context,omitempty"`
}

type ProjectLocalePreference struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID primitive.ObjectID `bson:"projectId" json:"projectId"`
	Locale    string             `bson:"locale" json:"locale"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

type TranslationJobStatus string

const (
	TranslationJobPending   TranslationJobStatus = "pending"
	TranslationJobRunning   TranslationJobStatus = "running"
	TranslationJobCompleted TranslationJobStatus = "completed"
	TranslationJobFailed    TranslationJobStatus = "failed"
	TranslationJobCancelled TranslationJobStatus = "cancelled"
)

type TranslationJob struct {
	ID             primitive.ObjectID   `bson:"_id,omitempty" json:"id,omitempty"`
	TenantID       primitive.ObjectID   `bson:"tenantId" json:"tenantId"`
	ProjectID      primitive.ObjectID   `bson:"projectId" json:"projectId"`
	RequestedBy    primitive.ObjectID   `bson:"requestedBy" json:"requestedBy"`
	Operation      string               `bson:"operation" json:"operation"`
	TargetLocales  []string             `bson:"targetLocales" json:"targetLocales"`
	Status         TranslationJobStatus `bson:"status" json:"status"`
	Total          int64                `bson:"total" json:"total"`
	Completed      int64                `bson:"completed" json:"completed"`
	SkippedManual  int64                `bson:"skippedManual" json:"skippedManual"`
	Failed         int64                `bson:"failed" json:"failed"`
	RetryCount     int                  `bson:"retryCount" json:"retryCount"`
	LeaseOwner     string               `bson:"leaseOwner,omitempty" json:"leaseOwner,omitempty"`
	LeaseExpiresAt *time.Time           `bson:"leaseExpiresAt,omitempty" json:"leaseExpiresAt,omitempty"`
	NextAttemptAt  time.Time            `bson:"nextAttemptAt" json:"nextAttemptAt"`
	ErrorSummary   []string             `bson:"errorSummary,omitempty" json:"errorSummary,omitempty"`
	CreatedAt      time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time            `bson:"updatedAt" json:"updatedAt"`
}
