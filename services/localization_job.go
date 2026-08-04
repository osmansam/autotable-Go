package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func QueueProjectTranslationJob(tenantID, projectID, requestedBy primitive.ObjectID, sourceLocale string, targetLocales []string) (primitive.ObjectID, error) {
	now := time.Now().UTC()
	job := &models.TranslationJob{
		ID: primitive.NewObjectID(), TenantID: tenantID, ProjectID: projectID, RequestedBy: requestedBy,
		Operation: "translate_locale", TargetLocales: targetLocales, Status: models.TranslationJobPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	repository := repositories.NewLocalizationRepository()
	if err := repository.CreateJob(context.Background(), job); err != nil {
		return primitive.NilObjectID, err
	}
	go processProjectTranslationJob(job, sourceLocale)
	return job.ID, nil
}

func discoverAllProjectStrings(ctx context.Context, tenantID, projectID primitive.ObjectID, sourceLocale string) ([]models.SourceString, error) {
	keys := map[string]models.SourceString{}
	pageCursor, err := utils.GetPageCollectionForProject(tenantID.Hex(), projectID.Hex()).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var pages []models.PageModel
	if err := pageCursor.All(ctx, &pages); err != nil {
		pageCursor.Close(ctx)
		return nil, err
	}
	pageCursor.Close(ctx)
	for _, page := range pages {
		for _, item := range DiscoverPageStrings(page, sourceLocale) {
			keys[item.TranslationKey] = item
		}
	}
	containerCursor, err := utils.GetContainerCollectionForProject(tenantID.Hex(), projectID.Hex()).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var containers []models.ContainerModel
	if err := containerCursor.All(ctx, &containers); err != nil {
		containerCursor.Close(ctx)
		return nil, err
	}
	containerCursor.Close(ctx)
	for _, container := range containers {
		for _, item := range DiscoverContainerStrings(container, sourceLocale) {
			keys[item.TranslationKey] = item
		}
	}
	result := make([]models.SourceString, 0, len(keys))
	for _, item := range keys {
		result = append(result, item)
	}
	return result, nil
}

func processProjectTranslationJob(job *models.TranslationJob, sourceLocale string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	repository := repositories.NewLocalizationRepository()
	sources, err := discoverAllProjectStrings(ctx, job.TenantID, job.ProjectID, sourceLocale)
	if err != nil {
		_ = repository.CompleteJob(ctx, job.ID, 0, 1, []string{err.Error()})
		return
	}
	_ = repository.SetJobRunning(ctx, job.ID, int64(len(sources)*len(job.TargetLocales)))
	var completed, failed int64
	errorsList := []string{}
	for _, locale := range job.TargetLocales {
		for start := 0; start < len(sources); start += 50 {
			end := start + 50
			if end > len(sources) {
				end = len(sources)
			}
			outputs, translateErr := TranslateWithOpenAI(ctx, sourceLocale, locale, sources[start:end])
			if translateErr != nil {
				failed += int64(end - start)
				errorsList = append(errorsList, fmt.Sprintf("%s: %v", locale, translateErr))
				continue
			}
			byKey := map[string]string{}
			for _, output := range outputs {
				byKey[output.Key] = output.Text
			}
			for _, source := range sources[start:end] {
				text, ok := byKey[source.TranslationKey]
				if !ok {
					failed++
					continue
				}
				entry := models.TranslationEntry{
					TenantID: job.TenantID, ProjectID: job.ProjectID, Locale: locale,
					TranslationKey: source.TranslationKey, ResourceType: source.ResourceType, ResourceID: source.ResourceID,
					PropertyPath: source.PropertyPath, SourceText: source.SourceText, SourceHash: source.SourceHash,
					TranslatedText: text, LastDiscovered: time.Now().UTC(), UpdatedBy: &job.RequestedBy,
				}
				if err := repository.UpsertGeneratedTranslation(ctx, entry); err != nil {
					failed++
					errorsList = append(errorsList, err.Error())
					continue
				}
				completed++
			}
		}
	}
	if len(errorsList) > 20 {
		errorsList = errorsList[:20]
	}
	if err := repository.CompleteJob(ctx, job.ID, completed, failed, errorsList); err != nil {
		log.Printf("translation job %s completion update failed: %v", job.ID.Hex(), err)
	}
}
