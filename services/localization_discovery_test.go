package services

import (
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBuildTranslationReconciliationPreservesManualTextAndMarksChangedSourceOutdated(t *testing.T) {
	now := time.Now().UTC()
	entry := models.TranslationEntry{
		TranslationKey: "page:abc.name", SourceText: "Customer", SourceHash: LocalizationSourceHash("en", "Customer"),
		TranslatedText: "Müşteriler", Origin: models.TranslationOriginManual, Status: models.TranslationStatusCurrent, IsActive: true,
	}
	source := models.SourceString{
		TranslationKey: "page:abc.name", ResourceType: "page", ResourceID: "abc", PropertyPath: "name",
		SourceText: "Client", SourceHash: LocalizationSourceHash("en", "Client"),
	}

	changes := BuildTranslationReconciliation([]models.SourceString{source}, []models.TranslationEntry{entry}, now)
	if len(changes) != 1 {
		t.Fatalf("changes = %#v, want one", changes)
	}
	got := changes[0]
	if got.TranslatedText != "Müşteriler" || got.Origin != models.TranslationOriginManual {
		t.Fatalf("manual translation changed: %#v", got)
	}
	if got.Status != models.TranslationStatusOutdated || got.SourceText != "Client" || got.SourceHash != source.SourceHash {
		t.Fatalf("source reconciliation = %#v", got)
	}
}

func TestBuildTranslationReconciliationOrphansAndReactivatesEntries(t *testing.T) {
	now := time.Now().UTC()
	orphanCandidate := models.TranslationEntry{TranslationKey: "page:abc.old", IsActive: true}
	reactivated := models.TranslationEntry{TranslationKey: "page:abc.name", IsActive: false, OrphanedAt: &now}
	source := models.SourceString{TranslationKey: "page:abc.name", SourceText: "Orders", SourceHash: "new"}

	changes := BuildTranslationReconciliation([]models.SourceString{source}, []models.TranslationEntry{orphanCandidate, reactivated}, now)
	if len(changes) != 2 {
		t.Fatalf("changes = %#v, want two", changes)
	}
	byKey := map[string]models.TranslationEntry{}
	for _, change := range changes {
		byKey[change.TranslationKey] = change
	}
	if byKey["page:abc.old"].IsActive || byKey["page:abc.old"].OrphanedAt == nil {
		t.Fatalf("missing key was not orphaned: %#v", byKey["page:abc.old"])
	}
	if !byKey["page:abc.name"].IsActive || byKey["page:abc.name"].OrphanedAt != nil {
		t.Fatalf("rediscovered key was not reactivated: %#v", byKey["page:abc.name"])
	}
}

func TestLocalizationSourceHashPreservesExactText(t *testing.T) {
	base := LocalizationSourceHash("tr", "Save")
	for _, changed := range []string{"Save!", " save", "save", "Save "} {
		if got := LocalizationSourceHash("tr", changed); got == base {
			t.Fatalf("hash treated %q as equivalent to Save", changed)
		}
	}
	if LocalizationSourceHash("TR", "Save") != base {
		t.Fatal("locale canonicalization changed the hash for equivalent locale casing")
	}
}

func TestDiscoverPageStringsUsesStableSemanticKeys(t *testing.T) {
	pageID := primitive.NewObjectID()
	page := models.PageModel{
		ID:   pageID,
		Name: "Orders",
		Filters: []models.PageFilterDefinition{
			{ID: "status-filter", Label: "Status"},
		},
		Sections: []models.Section{{
			ID: "main",
			Component: &models.ComponentBlock{
				ID:    "orders-table",
				Title: "Recent orders",
				Table: &models.TableComponentConfig{Columns: []models.TableColumnConfig{{Field: "customer", DisplayName: "Customer"}}},
			},
		}},
	}

	strings := DiscoverPageStrings(page, "en")
	assertSourceStrings(t, strings, map[string]string{
		"page:" + pageID.Hex() + ".name":                                               "Orders",
		"page:" + pageID.Hex() + ".filter:status-filter.label":                         "Status",
		"page:" + pageID.Hex() + ".component:orders-table.title":                       "Recent orders",
		"page:" + pageID.Hex() + ".component:orders-table.column:customer.displayName": "Customer",
	})
}

func TestDiscoverPageStringsSurvivesSectionReordering(t *testing.T) {
	page := models.PageModel{ID: primitive.NewObjectID(), Sections: []models.Section{
		{ID: "one", Component: &models.ComponentBlock{ID: "alpha", Title: "Alpha"}},
		{ID: "two", Component: &models.ComponentBlock{ID: "beta", Title: "Beta"}},
	}}
	before := sourceKeySet(DiscoverPageStrings(page, "en"))
	page.Sections[0], page.Sections[1] = page.Sections[1], page.Sections[0]
	after := sourceKeySet(DiscoverPageStrings(page, "en"))
	if len(before) != len(after) {
		t.Fatalf("key counts changed: %v vs %v", before, after)
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			t.Fatalf("key %q disappeared after reorder", key)
		}
	}
}

func TestDiscoverContainerStringsIncludesPresentationMetadataOnly(t *testing.T) {
	containerID := primitive.NewObjectID()
	container := models.ContainerModel{
		ID: containerID,
		Fields: []models.Field{{
			Name: "status",
			Frontend: &models.Frontend{
				DisplayName: "Order status",
				Actions: []models.ActionConfig{{
					ID: "archive", Key: "archive", Label: "Archive", ConfirmText: "Archive this order?",
					FormFields: &[]models.ActionFormFieldConfig{{
						ID: "reason", FormKey: "reason", Label: "Reason", Placeholder: "Select a reason",
						StaticOptions: []models.ActionFormOptionConfig{{ID: "duplicate", Value: "duplicate", Label: "Duplicate"}},
					}},
				}},
			},
			PopulationSettings: &models.PopulationSettings{DisplayLabel: "Current status"},
		}},
	}

	strings := DiscoverContainerStrings(container, "en")
	prefix := "container:" + containerID.Hex()
	assertSourceStrings(t, strings, map[string]string{
		prefix + ".field:status.displayName":                                            "Order status",
		prefix + ".field:status.population.displayLabel":                                "Current status",
		prefix + ".field:status.action:archive.label":                                   "Archive",
		prefix + ".field:status.action:archive.confirmText":                             "Archive this order?",
		prefix + ".field:status.action:archive.formField:reason.label":                  "Reason",
		prefix + ".field:status.action:archive.formField:reason.placeholder":            "Select a reason",
		prefix + ".field:status.action:archive.formField:reason.option:duplicate.label": "Duplicate",
	})
}

func TestDiscoverContainerStringsIncludesHumanizedFallbackFieldLabel(t *testing.T) {
	containerID := primitive.NewObjectID()
	container := models.ContainerModel{
		ID:     containerID,
		Fields: []models.Field{{Name: "totalAmount"}},
	}

	strings := DiscoverContainerStrings(container, "en")
	assertSourceStrings(t, strings, map[string]string{
		"container:" + containerID.Hex() + ".field:totalAmount.displayName": "Total Amount",
	})
}

func assertSourceStrings(t *testing.T, got []models.SourceString, want map[string]string) {
	t.Helper()
	values := map[string]string{}
	for _, item := range got {
		values[item.TranslationKey] = item.SourceText
	}
	for key, text := range want {
		if values[key] != text {
			t.Fatalf("%s = %q, want %q; all = %#v", key, values[key], text, values)
		}
	}
}

func sourceKeySet(items []models.SourceString) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		result[item.TranslationKey] = struct{}{}
	}
	return result
}
