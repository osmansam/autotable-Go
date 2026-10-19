package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
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

func (r *LocalizationRepository) UpsertPlaceholder(ctx context.Context, entry models.TranslationEntry) error {
	now := time.Now().UTC()
	filter := translationIdentityFilter(entry)
	update := bson.M{
		"$set": bson.M{
			"resourceType": entry.ResourceType, "resourceId": entry.ResourceID, "propertyPath": entry.PropertyPath,
			"sourceText": entry.SourceText, "sourceHash": entry.SourceHash, "isActive": true,
			"lastDiscovered": entry.LastDiscovered, "updatedAt": now,
		},
		"$setOnInsert": bson.M{
			"translatedText": "", "origin": models.TranslationOriginUntranslated,
			"status": models.TranslationStatusMissing, "createdAt": now,
		},
		"$unset": bson.M{"orphanedAt": ""},
	}
	_, err := r.translations.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *LocalizationRepository) ApplyReconciledEntries(ctx context.Context, entries []models.TranslationEntry) error {
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().UTC()
	writes := make([]mongo.WriteModel, 0, len(entries))
	for _, entry := range entries {
		update := reconciliationUpdate(entry, now)
		writes = append(writes, mongo.NewUpdateOneModel().
			SetFilter(translationIdentityFilter(entry)).
			SetUpdate(update).
			SetUpsert(true))
	}
	_, err := r.translations.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(false))
	return err
}

func (r *LocalizationRepository) UpsertGeneratedTranslation(ctx context.Context, entry models.TranslationEntry) (bool, error) {
	now := time.Now().UTC()
	filter := generatedTranslationFilter(entry)
	update := bson.M{
		"$set": bson.M{
			"translatedText": entry.TranslatedText, "origin": models.TranslationOriginAI,
			"status": models.TranslationStatusCurrent, "updatedBy": entry.UpdatedBy, "updatedAt": now,
		},
	}
	var stored bool
	err := r.writeContentWithClaim(ctx, func(tx context.Context) error {
		result, err := r.translations.UpdateOne(tx, filter, update)
		if err == nil {
			stored = result.MatchedCount > 0
		}
		return err
	})
	return stored && err == nil, err
}

func (r *LocalizationRepository) MarkGeneratedTranslationFailed(ctx context.Context, entry models.TranslationEntry) error {
	return r.writeContentWithClaim(ctx, func(tx context.Context) error {
		_, err := r.translations.UpdateOne(tx, generatedTranslationFilter(entry), bson.M{"$set": bson.M{
			"status": models.TranslationStatusFailed, "updatedAt": time.Now().UTC(),
		}})
		return err
	})
}

func generatedTranslationFilter(entry models.TranslationEntry) bson.M {
	filter := translationIdentityFilter(entry)
	filter["origin"] = bson.M{"$ne": models.TranslationOriginManual}
	filter["sourceHash"] = entry.SourceHash
	filter["isActive"] = true
	filter["status"] = bson.M{"$in": []models.TranslationStatus{
		models.TranslationStatusMissing,
		models.TranslationStatusOutdated,
		models.TranslationStatusFailed,
	}}
	return filter
}

func translationIdentityFilter(entry models.TranslationEntry) bson.M {
	return bson.M{
		"tenantId":       entry.TenantID,
		"projectId":      entry.ProjectID,
		"locale":         entry.Locale,
		"translationKey": entry.TranslationKey,
	}
}

func (r *LocalizationRepository) CreateJob(ctx context.Context, job *models.TranslationJob) error {
	if job.ID.IsZero() {
		job.ID = primitive.NewObjectID()
	}
	_, err := r.jobs.InsertOne(ctx, job)
	return err
}

func (r *LocalizationRepository) GetJob(ctx context.Context, tenantID, projectID, jobID primitive.ObjectID) (*models.TranslationJob, error) {
	var job models.TranslationJob
	err := r.jobs.FindOne(ctx, bson.M{"_id": jobID, "tenantId": tenantID, "projectId": projectID}).Decode(&job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *LocalizationRepository) SetJobRunning(ctx context.Context, id primitive.ObjectID, total int64) error {
	filter, err := localizationJobClaimFilter(ctx, id, time.Now().UTC())
	if err != nil {
		return err
	}
	result, err := r.jobs.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"status": models.TranslationJobRunning, "total": total, "completed": 0,
		"skippedManual": 0, "failed": 0, "errorSummary": []string{}, "updatedAt": time.Now().UTC(),
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrLocalizationLeaseLost
	}
	return nil
}

func (r *LocalizationRepository) UpdateJobProgress(ctx context.Context, id primitive.ObjectID, completed, skippedManual, failed int64, messages []string) error {
	filter, err := localizationJobClaimFilter(ctx, id, time.Now().UTC())
	if err != nil {
		return err
	}
	result, err := r.jobs.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"completed": completed, "skippedManual": skippedManual, "failed": failed,
		"errorSummary": messages, "updatedAt": time.Now().UTC(),
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrLocalizationLeaseLost
	}
	return nil
}

func (r *LocalizationRepository) CompleteJob(ctx context.Context, id primitive.ObjectID, completed, skippedManual, failed int64, messages []string) error {
	status := models.TranslationJobCompleted
	if failed > 0 {
		status = models.TranslationJobFailed
	}
	filter, err := localizationJobClaimFilter(ctx, id, time.Now().UTC())
	if err != nil {
		return err
	}
	result, err := r.jobs.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"status": status, "completed": completed, "skippedManual": skippedManual,
		"failed": failed, "errorSummary": messages, "updatedAt": time.Now().UTC(),
	}, "$unset": bson.M{"leaseOwner": "", "leaseToken": "", "leaseExpiresAt": ""}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrLocalizationLeaseLost
	}
	return nil
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

func (r *LocalizationRepository) ListProjectTranslations(ctx context.Context, tenantID, projectID primitive.ObjectID) ([]models.TranslationEntry, error) {
	cursor, err := r.translations.Find(
		ctx,
		bson.M{"tenantId": tenantID, "projectId": projectID},
		options.Find().SetSort(bson.D{{Key: "locale", Value: 1}, {Key: "translationKey", Value: 1}}),
	)
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
		"nextAttemptAt": bson.M{"$lte": now},
		"$or": []bson.M{
			{
				"status": models.TranslationJobPending,
				"$or":    []bson.M{{"leaseExpiresAt": bson.M{"$exists": false}}, {"leaseExpiresAt": nil}, {"leaseExpiresAt": bson.M{"$lte": now}}},
			},
			{"status": models.TranslationJobRunning, "leaseExpiresAt": bson.M{"$lte": now}},
		},
	}
	update := bson.M{"$set": bson.M{
		"status": models.TranslationJobRunning, "leaseOwner": owner, "leaseToken": uuid.NewString(),
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
