package controllers

import (
	"testing"

	"github.com/osmansam/autotableGo/models"
)

func TestLocaleDirection(t *testing.T) {
	for locale, want := range map[string]string{"en": "ltr", "tr": "ltr", "ar": "rtl", "ar-SA": "rtl", "he": "rtl"} {
		if got := localeDirection(locale); got != want {
			t.Fatalf("localeDirection(%q) = %q, want %q", locale, got, want)
		}
	}
}

func TestRuntimeTranslationCatalogUsesCurrentActiveTranslations(t *testing.T) {
	entries := []models.TranslationEntry{
		{SourceText: "Customer Name", TranslatedText: "Müşteri Adı", Status: models.TranslationStatusCurrent, IsActive: true},
		{SourceText: "Archived", TranslatedText: "Arşivlenmiş", Status: models.TranslationStatusOutdated, IsActive: true},
		{SourceText: "Deleted", TranslatedText: "Silinmiş", Status: models.TranslationStatusCurrent, IsActive: false},
	}

	resources := runtimeTranslationResources(entries)
	if len(resources) != 1 {
		t.Fatalf("resources = %#v, want one current active translation", resources)
	}
	if resources[0].SourceText != "Customer Name" || resources[0].TranslatedText != "Müşteri Adı" {
		t.Fatalf("resource = %#v", resources[0])
	}
}

func TestDecodeTranslationKeyParam(t *testing.T) {
	encoded := "container%3A69542a0d.field%3Aemail.displayName"
	want := "container:69542a0d.field:email.displayName"
	got, err := decodeTranslationKeyParam(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded key = %q, want %q", got, want)
	}
}
