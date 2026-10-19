package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type localizationJobRepository interface {
	RenewJobLease(context.Context, *models.TranslationJob, time.Time, time.Duration) (bool, error)
	CreateJob(context.Context, *models.TranslationJob) error
	ListProjectTranslations(context.Context, primitive.ObjectID, primitive.ObjectID) ([]models.TranslationEntry, error)
	SetJobRunning(context.Context, primitive.ObjectID, int64) error
	UpdateJobProgress(context.Context, primitive.ObjectID, int64, int64, int64, []string) error
	CompleteJob(context.Context, primitive.ObjectID, int64, int64, int64, []string) error
	UpsertGeneratedTranslation(context.Context, models.TranslationEntry) (bool, error)
	MarkGeneratedTranslationFailed(context.Context, models.TranslationEntry) error
	LeaseNextJob(context.Context, string, time.Time, time.Duration) (*models.TranslationJob, error)
}

type projectLocalizationRepository interface {
	GetProject(context.Context, primitive.ObjectID, primitive.ObjectID) (*models.Project, error)
	IncrementVersion(context.Context, primitive.ObjectID, primitive.ObjectID) error
}

var (
	localizationJobRepositoryFactory = func() localizationJobRepository {
		return repositories.NewLocalizationRepository()
	}
	projectLocalizationRepositoryFactory = func() projectLocalizationRepository {
		return repositories.NewProjectLocalizationRepository()
	}
	reconcileTranslationsForJob = ReconcileProjectTranslations
	translateLocalizationBatch  = TranslateWithOpenAI
)

func QueueProjectTranslationJob(tenantID, projectID, requestedBy primitive.ObjectID, sourceLocale string, targetLocales, translationKeys []string) (primitive.ObjectID, error) {
	project, err := projectLocalizationRepositoryFactory().GetProject(context.Background(), tenantID, projectID)
	if err != nil {
		return primitive.NilObjectID, err
	}
	if sourceLocale == "" {
		sourceLocale = project.SourceLocale
	}
	if sourceLocale != project.SourceLocale {
		return primitive.NilObjectID, fmt.Errorf("source locale %q does not match project source locale %q", sourceLocale, project.SourceLocale)
	}
	normalizedTargets, err := validateTranslationJobTargets(project, targetLocales)
	if err != nil {
		return primitive.NilObjectID, err
	}
	now := time.Now().UTC()
	job := &models.TranslationJob{
		ID: primitive.NewObjectID(), TenantID: tenantID, ProjectID: projectID, RequestedBy: requestedBy,
		Operation: "translate_locale", SourceLocale: sourceLocale, TargetLocales: normalizedTargets,
		TranslationKeys: uniqueSortedStrings(translationKeys), Status: models.TranslationJobPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := localizationJobRepositoryFactory().CreateJob(context.Background(), job); err != nil {
		return primitive.NilObjectID, err
	}
	log.Printf("localization_job queued tenant_id=%s project_id=%s job_id=%s targets=%d keys=%d", tenantID.Hex(), projectID.Hex(), job.ID.Hex(), len(job.TargetLocales), len(job.TranslationKeys))
	return job.ID, nil
}

func ProcessProjectTranslationJob(ctx context.Context, job *models.TranslationJob) error {
	repository := localizationJobRepositoryFactory()
	projects := projectLocalizationRepositoryFactory()
	project, err := projects.GetProject(ctx, job.TenantID, job.ProjectID)
	if err != nil {
		return completeJobWithError(ctx, repository, job, fmt.Errorf("load project: %w", err))
	}
	if job.SourceLocale == "" {
		job.SourceLocale = project.SourceLocale
	}
	if job.SourceLocale != project.SourceLocale {
		return completeJobWithError(ctx, repository, job, fmt.Errorf("job source locale %q no longer matches project source locale %q", job.SourceLocale, project.SourceLocale))
	}
	targets, err := validateTranslationJobTargets(project, job.TargetLocales)
	if err != nil {
		return completeJobWithError(ctx, repository, job, err)
	}
	if _, err := reconcileTranslationsForJob(ctx, job.TenantID, job.ProjectID, job.SourceLocale, project.EnabledLocales); err != nil {
		return completeJobWithError(ctx, repository, job, fmt.Errorf("reconcile translations: %w", err))
	}
	entries, err := repository.ListProjectTranslations(ctx, job.TenantID, job.ProjectID)
	if err != nil {
		return completeJobWithError(ctx, repository, job, fmt.Errorf("list translations: %w", err))
	}
	selected, skippedManual := selectTranslationJobEntries(entries, targets, job.TranslationKeys)
	if err := repository.SetJobRunning(ctx, job.ID, int64(len(selected))+skippedManual); err != nil {
		return err
	}
	// Reserve a new public-cache version before any translation row can change.
	// A failed write may cause a harmless extra invalidation; the reverse order
	// could publish changed translations under an indefinitely cached old version.
	if len(selected) > 0 {
		if err := projects.IncrementVersion(ctx, job.TenantID, job.ProjectID); err != nil {
			return completeJobWithError(ctx, repository, job, fmt.Errorf("reserve localization version: %w", err))
		}
	}

	var completed, failed int64
	errorsList := make([]string, 0)
	byLocale := make(map[string][]models.TranslationEntry, len(targets))
	for _, entry := range selected {
		byLocale[entry.Locale] = append(byLocale[entry.Locale], entry)
	}
	for _, locale := range targets {
		localeEntries := byLocale[locale]
		for start := 0; start < len(localeEntries); start += 50 {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := start + 50
			if end > len(localeEntries) {
				end = len(localeEntries)
			}
			batch := localeEntries[start:end]
			sources := make([]models.SourceString, 0, len(batch))
			for _, entry := range batch {
				sources = append(sources, models.SourceString{
					TranslationKey: entry.TranslationKey, ResourceType: entry.ResourceType, ResourceID: entry.ResourceID,
					PropertyPath: entry.PropertyPath, SourceText: entry.SourceText, SourceHash: entry.SourceHash,
				})
			}
			outputs, translateErr := translateLocalizationBatch(ctx, job.SourceLocale, locale, sources)
			if translateErr != nil {
				for index := range batch {
					if err := ctx.Err(); err != nil {
						return err
					}
					markTranslationJobEntryFailed(ctx, repository, &batch[index])
					failed++
				}
				errorsList = appendBoundedJobError(errorsList, locale, translateErr)
				_ = repository.UpdateJobProgress(ctx, job.ID, completed, skippedManual, failed, errorsList)
				continue
			}
			outputByKey := make(map[string]TranslationOutput, len(outputs))
			duplicates := make(map[string]bool)
			for _, output := range outputs {
				if _, exists := outputByKey[output.Key]; exists {
					duplicates[output.Key] = true
				}
				outputByKey[output.Key] = output
			}
			for index := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				entry := batch[index]
				output, found := outputByKey[entry.TranslationKey]
				var validationErr error
				switch {
				case !found:
					validationErr = fmt.Errorf("AI response omitted translation key")
				case duplicates[entry.TranslationKey]:
					validationErr = fmt.Errorf("AI response duplicated translation key")
				case strings.TrimSpace(output.Text) == "":
					validationErr = fmt.Errorf("AI response returned an empty translation")
				default:
					validationErr = validateProtectedTranslation(entry.SourceText, output.Text)
				}
				if validationErr != nil {
					markTranslationJobEntryFailed(ctx, repository, &entry)
					failed++
					errorsList = appendBoundedJobError(errorsList, entry.TranslationKey, validationErr)
					continue
				}
				entry.TranslatedText = output.Text
				entry.UpdatedBy = &job.RequestedBy
				entry.LastDiscovered = time.Now().UTC()
				stored, storeErr := repository.UpsertGeneratedTranslation(ctx, entry)
				if storeErr != nil {
					markTranslationJobEntryFailed(ctx, repository, &entry)
					failed++
					errorsList = appendBoundedJobError(errorsList, entry.TranslationKey, storeErr)
					continue
				}
				if !stored {
					skippedManual++
					continue
				}
				completed++
			}
			if err := repository.UpdateJobProgress(ctx, job.ID, completed, skippedManual, failed, errorsList); err != nil {
				return err
			}
		}
	}
	if completed > 0 {
		if err := projects.IncrementVersion(ctx, job.TenantID, job.ProjectID); err != nil {
			errorsList = appendBoundedJobError(errorsList, "publish localization version", err)
			failed++
		}
	}
	if err := repository.CompleteJob(ctx, job.ID, completed, skippedManual, failed, errorsList); err != nil {
		return err
	}
	log.Printf("localization_job completed tenant_id=%s project_id=%s job_id=%s completed=%d skipped_manual=%d failed=%d", job.TenantID.Hex(), job.ProjectID.Hex(), job.ID.Hex(), completed, skippedManual, failed)
	return nil
}

func selectTranslationJobEntries(entries []models.TranslationEntry, targetLocales, translationKeys []string) ([]models.TranslationEntry, int64) {
	targetSet := make(map[string]bool, len(targetLocales))
	for _, locale := range targetLocales {
		targetSet[locale] = true
	}
	keySet := make(map[string]bool, len(translationKeys))
	for _, key := range translationKeys {
		keySet[key] = true
	}
	selected := make([]models.TranslationEntry, 0)
	var skippedManual int64
	for _, entry := range entries {
		if !entry.IsActive || !targetSet[entry.Locale] || (len(keySet) > 0 && !keySet[entry.TranslationKey]) {
			continue
		}
		if entry.Origin == models.TranslationOriginManual && entry.TranslatedText != "" {
			skippedManual++
			continue
		}
		if entry.Status == models.TranslationStatusMissing || entry.Status == models.TranslationStatusFailed || (entry.Status == models.TranslationStatusOutdated && entry.Origin != models.TranslationOriginManual) {
			selected = append(selected, entry)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Locale == selected[j].Locale {
			return selected[i].TranslationKey < selected[j].TranslationKey
		}
		return selected[i].Locale < selected[j].Locale
	})
	return selected, skippedManual
}

func validateTranslationJobTargets(project *models.Project, requested []string) ([]string, error) {
	enabled := make(map[string]bool, len(project.EnabledLocales))
	for _, locale := range project.EnabledLocales {
		enabled[locale] = true
	}
	targets := uniqueSortedStrings(requested)
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one target locale is required")
	}
	for _, locale := range targets {
		if locale == project.SourceLocale {
			return nil, fmt.Errorf("source locale %q cannot be a translation target", locale)
		}
		if !enabled[locale] {
			return nil, fmt.Errorf("target locale %q is not enabled for this project", locale)
		}
	}
	return targets, nil
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func markTranslationJobEntryFailed(ctx context.Context, repository localizationJobRepository, entry *models.TranslationEntry) {
	_ = repository.MarkGeneratedTranslationFailed(ctx, *entry)
}

func appendBoundedJobError(messages []string, key string, err error) []string {
	if len(messages) >= 20 {
		return messages
	}
	message := strings.Join(strings.Fields(fmt.Sprintf("%s: %v", key, err)), " ")
	if len(message) > 240 {
		message = message[:240]
	}
	return append(messages, message)
}

func completeJobWithError(ctx context.Context, repository localizationJobRepository, job *models.TranslationJob, jobErr error) error {
	messages := appendBoundedJobError(nil, "job", jobErr)
	if err := repository.CompleteJob(ctx, job.ID, 0, 0, 1, messages); err != nil {
		return fmt.Errorf("%v; update failed job: %w", jobErr, err)
	}
	log.Printf("localization_job failed tenant_id=%s project_id=%s job_id=%s error=%q", job.TenantID.Hex(), job.ProjectID.Hex(), job.ID.Hex(), messages[0])
	return jobErr
}
