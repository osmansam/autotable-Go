package services

import (
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestReconcileProjectBuildsPlaceholdersForEveryTargetLocale(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	sources := []models.SourceString{
		{TranslationKey: "a", ResourceType: "sitePages", ResourceID: "home", PropertyPath: "title", SourceText: "Home", SourceHash: "h1"},
		{TranslationKey: "b", ResourceType: "visualBlocks", ResourceID: "hero", PropertyPath: "subtitle", SourceText: "Hello", SourceHash: "h2"},
	}

	entries, summary := BuildProjectTranslationReconciliation(tenantID, projectID, "en", []string{"de", "en", "tr"}, sources, nil, now)
	if summary.Discovered != 2 || summary.Created != 4 || summary.Updated != 0 || summary.Failed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(entries) != 4 {
		t.Fatalf("entries = %#v", entries)
	}
	for _, entry := range entries {
		if entry.Locale == "en" || entry.Status != models.TranslationStatusMissing || entry.Origin != models.TranslationOriginUntranslated || !entry.IsActive {
			t.Fatalf("invalid placeholder = %#v", entry)
		}
	}
}

func TestReconcileProjectPreservesManualTextAcrossSourceChangesAndLocaleDisable(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	existing := []models.TranslationEntry{
		{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "kept", SourceHash: "old", TranslatedText: "Manuel", Origin: models.TranslationOriginManual, Status: models.TranslationStatusCurrent, IsActive: true},
		{TenantID: tenantID, ProjectID: projectID, Locale: "de", TranslationKey: "kept", SourceHash: "new", TranslatedText: "Deutsch", Origin: models.TranslationOriginAI, Status: models.TranslationStatusCurrent, IsActive: true},
		{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "removed", SourceHash: "old", TranslatedText: "Eski", Origin: models.TranslationOriginManual, Status: models.TranslationStatusCurrent, IsActive: true},
	}
	sources := []models.SourceString{{TranslationKey: "kept", ResourceType: "sitePages", ResourceID: "home", PropertyPath: "title", SourceText: "Changed", SourceHash: "new"}}

	entries, summary := BuildProjectTranslationReconciliation(tenantID, projectID, "en", []string{"en", "tr"}, sources, existing, now)
	byLocaleKey := map[string]models.TranslationEntry{}
	for _, entry := range entries {
		byLocaleKey[entry.Locale+":"+entry.TranslationKey] = entry
	}
	manual := byLocaleKey["tr:kept"]
	if manual.TranslatedText != "Manuel" || manual.Origin != models.TranslationOriginManual || manual.Status != models.TranslationStatusOutdated || !manual.IsActive {
		t.Fatalf("manual source change = %#v", manual)
	}
	disabled := byLocaleKey["de:kept"]
	if disabled.TranslatedText != "Deutsch" || disabled.IsActive {
		t.Fatalf("disabled locale was not retained inactive: %#v", disabled)
	}
	removed := byLocaleKey["tr:removed"]
	if removed.IsActive || removed.OrphanedAt == nil {
		t.Fatalf("removed source was not retained inactive: %#v", removed)
	}
	if summary.Outdated != 1 || summary.Inactive != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestReconcileProjectReactivatesMatchingEntryIdempotently(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	orphanedAt := now.Add(-time.Hour)
	existing := []models.TranslationEntry{{
		TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "title", SourceHash: "same",
		TranslatedText: "Başlık", Origin: models.TranslationOriginManual, Status: models.TranslationStatusCurrent,
		IsActive: false, OrphanedAt: &orphanedAt,
	}}
	sources := []models.SourceString{{TranslationKey: "title", ResourceType: "sitePages", ResourceID: "home", PropertyPath: "title", SourceText: "Title", SourceHash: "same"}}

	first, summary := BuildProjectTranslationReconciliation(tenantID, projectID, "en", []string{"en", "tr"}, sources, existing, now)
	if len(first) != 1 || !first[0].IsActive || first[0].OrphanedAt != nil || summary.Updated != 1 {
		t.Fatalf("reactivation = %#v, summary = %#v", first, summary)
	}
	second, secondSummary := BuildProjectTranslationReconciliation(tenantID, projectID, "en", []string{"en", "tr"}, sources, first, now)
	if len(second) != 0 || secondSummary.Updated != 0 || secondSummary.Created != 0 || secondSummary.Inactive != 0 || secondSummary.Outdated != 0 {
		t.Fatalf("idempotent run = %#v, summary = %#v", second, secondSummary)
	}
}
