package controllers

import (
	"reflect"
	"testing"

	"github.com/osmansam/autotableGo/models"
)

func TestRuntimeTranslationResourcesIncludeCanonicalOwnershipAndFilterLifecycle(t *testing.T) {
	entries := []models.TranslationEntry{
		{TranslationKey: "ozmishow:sitePages:home:title", ResourceType: "sitePages", ResourceID: "home", PropertyPath: "title", SourceText: "Home", TranslatedText: "Ana sayfa", Status: models.TranslationStatusCurrent, IsActive: true},
		{TranslationKey: "missing", SourceText: "Missing", Status: models.TranslationStatusMissing, IsActive: true},
		{TranslationKey: "outdated", SourceText: "Old", TranslatedText: "Eski", Status: models.TranslationStatusOutdated, IsActive: true},
		{TranslationKey: "inactive", SourceText: "Gone", TranslatedText: "Gitti", Status: models.TranslationStatusCurrent, IsActive: false},
	}
	got := runtimeTranslationResources(entries)
	if len(got) != 1 {
		t.Fatalf("resources = %#v", got)
	}
	if got[0].TranslationKey != entries[0].TranslationKey || got[0].ResourceType != "sitePages" || got[0].ResourceID != "home" || got[0].PropertyPath != "title" {
		t.Fatalf("ownership = %#v", got[0])
	}
}

func TestNormalizeProjectLocaleInputCanonicalizesAndValidates(t *testing.T) {
	got, err := normalizeProjectLocaleInput(projectLocaleInput{SourceLocale: "EN", DefaultLocale: "tr-tr", EnabledLocales: []string{"EN", "tr-tr"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceLocale != "en" || got.DefaultLocale != "tr-TR" || !reflect.DeepEqual(got.EnabledLocales, []string{"en", "tr-TR"}) {
		t.Fatalf("normalized input = %#v", got)
	}
	if _, err := normalizeProjectLocaleInput(projectLocaleInput{SourceLocale: "en", DefaultLocale: "tr", EnabledLocales: []string{"en"}}); err == nil {
		t.Fatal("default locale outside enabled set was accepted")
	}
}

func TestDecodeTranslationKeyParamPreservesCanonicalPercentEscapes(t *testing.T) {
	key := "ozmishow:sitePages:page%2Fwith+space:hero.title%3Avalue"
	got, err := decodeTranslationKeyParam(key)
	if err != nil {
		t.Fatal(err)
	}
	if got != key {
		t.Fatalf("decoded key = %q, want %q", got, key)
	}
}
