package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LocalizationRepository struct {
	translations *mongo.Collection
	preferences  *mongo.Collection
	jobs         *mongo.Collection
}

func NewLocalizationRepository() *LocalizationRepository {
	return NewLocalizationRepositoryWithCollections(
		configs.GetCollection("project_translations"),
		configs.GetCollection("project_locale_preferences"),
		configs.GetCollection("translation_jobs"),
	)
}

func NewLocalizationRepositoryWithCollections(translations, preferences, jobs *mongo.Collection) *LocalizationRepository {
	return &LocalizationRepository{translations: translations, preferences: preferences, jobs: jobs}
}

func localizationIndexModels() (translations, preferences, jobs []mongo.IndexModel) {
	translations = []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "locale", Value: 1}, {Key: "translationKey", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "locale", Value: 1}, {Key: "status", Value: 1}, {Key: "isActive", Value: 1}}},
	}
	preferences = []mongo.IndexModel{{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}},
		Options: options.Index().SetUnique(true),
	}}
	jobs = []mongo.IndexModel{{
		Keys: bson.D{{Key: "status", Value: 1}, {Key: "nextAttemptAt", Value: 1}, {Key: "leaseExpiresAt", Value: 1}},
	}}
	return translations, preferences, jobs
}

func (r *LocalizationRepository) EnsureIndexes(ctx context.Context) error {
	translationIndexes, preferenceIndexes, jobIndexes := localizationIndexModels()
	if _, err := r.translations.Indexes().CreateMany(ctx, translationIndexes); err != nil {
		return err
	}
	if _, err := r.preferences.Indexes().CreateMany(ctx, preferenceIndexes); err != nil {
		return err
	}
	_, err := r.jobs.Indexes().CreateMany(ctx, jobIndexes)
	return err
}

func (r *LocalizationRepository) UpsertGeneratedTranslation(ctx context.Context, entry models.TranslationEntry) error {
	now := time.Now().UTC()
	filter := bson.M{
		"tenantId":       entry.TenantID,
		"projectId":      entry.ProjectID,
		"locale":         entry.Locale,
		"translationKey": entry.TranslationKey,
		"origin":         bson.M{"$ne": models.TranslationOriginManual},
	}
	update := bson.M{
		"$set": bson.M{
			"resourceType": entry.ResourceType, "resourceId": entry.ResourceID, "propertyPath": entry.PropertyPath,
			"sourceText": entry.SourceText, "sourceHash": entry.SourceHash, "translatedText": entry.TranslatedText,
			"origin": models.TranslationOriginAI, "status": models.TranslationStatusCurrent, "isActive": true,
			"lastDiscovered": entry.LastDiscovered, "updatedBy": entry.UpdatedBy, "updatedAt": now,
		},
		"$setOnInsert": bson.M{"createdAt": now},
		"$unset":       bson.M{"orphanedAt": ""},
	}
	result, err := r.translations.UpdateOne(ctx, filter, update)
	if err != nil || result.MatchedCount > 0 {
		return err
	}
	entry.Origin = models.TranslationOriginAI
	entry.Status = models.TranslationStatusCurrent
	entry.IsActive = true
	entry.CreatedAt = now
	entry.UpdatedAt = now
	_, err = r.translations.InsertOne(ctx, entry)
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (r *LocalizationRepository) CreateJob(ctx context.Context, job *models.TranslationJob) error {
	if job.ID.IsZero() {
		job.ID = primitive.NewObjectID()
	}
	_, err := r.jobs.InsertOne(ctx, job)
	return err
}

func (r *LocalizationRepository) SetJobRunning(ctx context.Context, id primitive.ObjectID, total int64) error {
	_, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": models.TranslationJobRunning, "total": total, "updatedAt": time.Now().UTC()}})
	return err
}

func (r *LocalizationRepository) CompleteJob(ctx context.Context, id primitive.ObjectID, completed, failed int64, messages []string) error {
	status := models.TranslationJobCompleted
	if failed > 0 {
		status = models.TranslationJobFailed
	}
	_, err := r.jobs.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"status": status, "completed": completed, "failed": failed, "errorSummary": messages, "updatedAt": time.Now().UTC(),
	}, "$unset": bson.M{"leaseOwner": "", "leaseExpiresAt": ""}})
	return err
}

func IsNoLocalizationJob(err error) bool { return errors.Is(err, mongo.ErrNoDocuments) }

func (r *LocalizationRepository) UpsertManualTranslation(ctx context.Context, entry models.TranslationEntry) error {
	now := time.Now().UTC()
	filter := bson.M{
		"tenantId":       entry.TenantID,
		"projectId":      entry.ProjectID,
		"locale":         entry.Locale,
		"translationKey": entry.TranslationKey,
	}
	update := bson.M{
		"$set": bson.M{
			"resourceType": entry.ResourceType, "resourceId": entry.ResourceID, "propertyPath": entry.PropertyPath,
			"sourceText": entry.SourceText, "sourceHash": entry.SourceHash, "translatedText": entry.TranslatedText,
			"origin": models.TranslationOriginManual, "status": models.TranslationStatusCurrent, "isActive": true,
			"lastDiscovered": entry.LastDiscovered, "updatedBy": entry.UpdatedBy, "updatedAt": now,
		},
		"$setOnInsert": bson.M{"createdAt": now},
		"$unset":       bson.M{"orphanedAt": ""},
	}
	_, err := r.translations.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *LocalizationRepository) UpsertPreference(ctx context.Context, preference models.ProjectLocalePreference) error {
	now := preference.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	filter := bson.M{"userId": preference.UserID, "tenantId": preference.TenantID, "projectId": preference.ProjectID}
	update := bson.M{
		"$set":         bson.M{"locale": preference.Locale, "updatedAt": now},
		"$setOnInsert": bson.M{"createdAt": now},
	}
	_, err := r.preferences.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *LocalizationRepository) GetPreference(ctx context.Context, userID, tenantID, projectID primitive.ObjectID) (*models.ProjectLocalePreference, error) {
	var preference models.ProjectLocalePreference
	err := r.preferences.FindOne(ctx, bson.M{"userId": userID, "tenantId": tenantID, "projectId": projectID}).Decode(&preference)
	if err != nil {
		return nil, err
	}
	return &preference, nil
}

func (r *LocalizationRepository) ListTranslations(ctx context.Context, tenantID, projectID primitive.ObjectID, locale string) ([]models.TranslationEntry, error) {
	filter := bson.M{"tenantId": tenantID, "projectId": projectID, "isActive": true}
	if locale != "" {
		filter["locale"] = locale
	}
	cursor, err := r.translations.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "resourceType", Value: 1}, {Key: "translationKey", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var entries []models.TranslationEntry
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *LocalizationRepository) GetTranslation(ctx context.Context, tenantID, projectID primitive.ObjectID, locale, key string) (*models.TranslationEntry, error) {
	var entry models.TranslationEntry
	err := r.translations.FindOne(ctx, bson.M{"tenantId": tenantID, "projectId": projectID, "locale": locale, "translationKey": key}).Decode(&entry)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *LocalizationRepository) LeaseNextJob(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration) (*models.TranslationJob, error) {
	filter := bson.M{
		"status":        models.TranslationJobPending,
		"nextAttemptAt": bson.M{"$lte": now},
		"$or": []bson.M{
			{"leaseExpiresAt": bson.M{"$exists": false}},
			{"leaseExpiresAt": nil},
			{"leaseExpiresAt": bson.M{"$lte": now}},
		},
	}
	update := bson.M{"$set": bson.M{
		"status": models.TranslationJobRunning, "leaseOwner": owner,
		"leaseExpiresAt": now.Add(leaseDuration), "updatedAt": now,
	}}
	findOptions := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}, {Key: "createdAt", Value: 1}}).
		SetReturnDocument(options.After)
	var job models.TranslationJob
	if err := r.jobs.FindOneAndUpdate(ctx, filter, update, findOptions).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}
