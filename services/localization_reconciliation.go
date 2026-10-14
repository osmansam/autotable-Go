package services

import (
	"context"
	"sort"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func BuildProjectTranslationReconciliation(
	tenantID, projectID primitive.ObjectID,
	sourceLocale string,
	enabledLocales []string,
	sources []models.SourceString,
	existing []models.TranslationEntry,
	now time.Time,
) ([]models.TranslationEntry, models.TranslationReconciliationSummary) {
	summary := models.TranslationReconciliationSummary{Discovered: len(sources)}
	sourceByKey := make(map[string]models.SourceString, len(sources))
	for _, source := range sources {
		sourceByKey[source.TranslationKey] = source
	}
	targetLocales := make(map[string]bool, len(enabledLocales))
	for _, locale := range enabledLocales {
		if locale != "" && locale != sourceLocale {
			targetLocales[locale] = true
		}
	}
	existingByIdentity := make(map[string]models.TranslationEntry, len(existing))
	for _, entry := range existing {
		existingByIdentity[entry.Locale+"\x00"+entry.TranslationKey] = entry
	}

	changes := make([]models.TranslationEntry, 0)
	for _, entry := range existing {
		source, sourceExists := sourceByKey[entry.TranslationKey]
		shouldBeActive := targetLocales[entry.Locale] && sourceExists
		changed := false
		if !shouldBeActive {
			if entry.IsActive {
				entry.IsActive = false
				orphanedAt := now
				entry.OrphanedAt = &orphanedAt
				changed = true
				summary.Inactive++
			}
		} else {
			if entry.SourceHash != source.SourceHash {
				if entry.Status != models.TranslationStatusOutdated {
					summary.Outdated++
				}
				entry.Status = models.TranslationStatusOutdated
				changed = true
			}
			if entry.ResourceType != source.ResourceType || entry.ResourceID != source.ResourceID || entry.PropertyPath != source.PropertyPath || entry.SourceText != source.SourceText || entry.SourceHash != source.SourceHash {
				changed = true
			}
			if !entry.IsActive || entry.OrphanedAt != nil {
				changed = true
			}
			entry.ResourceType = source.ResourceType
			entry.ResourceID = source.ResourceID
			entry.PropertyPath = source.PropertyPath
			entry.SourceText = source.SourceText
			entry.SourceHash = source.SourceHash
			entry.IsActive = true
			entry.OrphanedAt = nil
			if changed {
				entry.LastDiscovered = now
			}
		}
		if changed {
			entry.UpdatedAt = now
			changes = append(changes, entry)
			summary.Updated++
		}
	}

	locales := make([]string, 0, len(targetLocales))
	for locale := range targetLocales {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	sortedSources := append([]models.SourceString(nil), sources...)
	sort.Slice(sortedSources, func(i, j int) bool { return sortedSources[i].TranslationKey < sortedSources[j].TranslationKey })
	for _, locale := range locales {
		for _, source := range sortedSources {
			identity := locale + "\x00" + source.TranslationKey
			if _, found := existingByIdentity[identity]; found {
				continue
			}
			changes = append(changes, models.TranslationEntry{
				TenantID: tenantID, ProjectID: projectID, Locale: locale,
				TranslationKey: source.TranslationKey, ResourceType: source.ResourceType, ResourceID: source.ResourceID,
				PropertyPath: source.PropertyPath, SourceText: source.SourceText, SourceHash: source.SourceHash,
				TranslatedText: "", Origin: models.TranslationOriginUntranslated, Status: models.TranslationStatusMissing,
				IsActive: true, LastDiscovered: now, CreatedAt: now, UpdatedAt: now,
			})
			summary.Created++
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Locale == changes[j].Locale {
			return changes[i].TranslationKey < changes[j].TranslationKey
		}
		return changes[i].Locale < changes[j].Locale
	})
	return changes, summary
}

func ReconcileProjectTranslations(ctx context.Context, tenantID, projectID primitive.ObjectID, sourceLocale string, enabledLocales []string) (models.TranslationReconciliationSummary, error) {
	sources, err := DiscoverAllProjectStrings(ctx, tenantID, projectID, sourceLocale)
	if err != nil {
		return models.TranslationReconciliationSummary{Discovered: len(sources), Failed: 1}, err
	}
	repository := repositories.NewLocalizationRepository()
	existing, err := repository.ListProjectTranslations(ctx, tenantID, projectID)
	if err != nil {
		return models.TranslationReconciliationSummary{Discovered: len(sources), Failed: 1}, err
	}
	changes, summary := BuildProjectTranslationReconciliation(tenantID, projectID, sourceLocale, enabledLocales, sources, existing, time.Now().UTC())
	if len(changes) > 0 {
		if err := repositories.NewProjectLocalizationRepository().IncrementVersion(ctx, tenantID, projectID); err != nil {
			summary.Failed++
			return summary, err
		}
		if err := repository.ApplyReconciledEntries(ctx, changes); err != nil {
			summary.Failed++
			return summary, err
		}
		if err := repositories.NewProjectLocalizationRepository().IncrementVersion(ctx, tenantID, projectID); err != nil {
			summary.Failed++
			return summary, err
		}
	}
	return summary, nil
}
