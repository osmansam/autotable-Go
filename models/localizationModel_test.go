package models

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestProjectApplyLocaleDefaults(t *testing.T) {
	project := Project{}

	project.ApplyLocaleDefaults()

	if project.SourceLocale != "en" || project.DefaultLocale != "en" {
		t.Fatalf("locale defaults = %q/%q, want en/en", project.SourceLocale, project.DefaultLocale)
	}
	if len(project.EnabledLocales) != 1 || project.EnabledLocales[0] != "en" {
		t.Fatalf("enabled locales = %#v, want [en]", project.EnabledLocales)
	}
	if project.LocalizationVersion != 1 {
		t.Fatalf("localization version = %d, want 1", project.LocalizationVersion)
	}
}

func TestProjectApplyLocaleDefaultsPreservesConfiguredValues(t *testing.T) {
	project := Project{
		SourceLocale:        "tr",
		DefaultLocale:       "de",
		EnabledLocales:      []string{"tr", "de"},
		LocalizationVersion: 7,
	}

	project.ApplyLocaleDefaults()

	if project.SourceLocale != "tr" || project.DefaultLocale != "de" || project.LocalizationVersion != 7 {
		t.Fatalf("configured locale values changed: %#v", project)
	}
}

func TestTranslationEntryBSONIncludesOwnershipAndLifecycle(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	entry := TranslationEntry{
		ID:             primitive.NewObjectID(),
		TenantID:       primitive.NewObjectID(),
		ProjectID:      primitive.NewObjectID(),
		Locale:         "tr",
		TranslationKey: "page:abc.name",
		ResourceType:   "page",
		ResourceID:     "abc",
		PropertyPath:   "name",
		SourceText:     "Orders",
		SourceHash:     "hash",
		TranslatedText: "Siparişler",
		Origin:         TranslationOriginManual,
		Status:         TranslationStatusCurrent,
		IsActive:       true,
		LastDiscovered: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	raw, err := bson.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var document bson.M
	if err := bson.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tenantId", "projectId", "locale", "translationKey", "resourceType", "resourceId", "propertyPath", "origin", "status", "isActive", "lastDiscovered"} {
		if _, ok := document[key]; !ok {
			t.Fatalf("serialized translation missing %q: %#v", key, document)
		}
	}
}

func TestTranslationLifecycleMissingValuesSerialize(t *testing.T) {
	entry := TranslationEntry{
		Origin: TranslationOriginUntranslated,
		Status: TranslationStatusMissing,
	}

	raw, err := bson.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var document bson.M
	if err := bson.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if got := document["origin"]; got != string(TranslationOriginUntranslated) {
		t.Fatalf("origin = %#v, want untranslated", got)
	}
	if got := document["status"]; got != string(TranslationStatusMissing) {
		t.Fatalf("status = %#v, want missing", got)
	}
}

func TestTranslationJobBSONPersistsSourceLocaleAndOptionalKeys(t *testing.T) {
	job := TranslationJob{
		SourceLocale:    "en",
		TranslationKeys: []string{"ozmishow:sitePage:page/a:title"},
	}

	raw, err := bson.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var document bson.M
	if err := bson.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if got := document["sourceLocale"]; got != "en" {
		t.Fatalf("sourceLocale = %#v, want en", got)
	}
	keys, ok := document["translationKeys"].(bson.A)
	if !ok || len(keys) != 1 || keys[0] != "ozmishow:sitePage:page/a:title" {
		t.Fatalf("translationKeys = %#v", document["translationKeys"])
	}
}
