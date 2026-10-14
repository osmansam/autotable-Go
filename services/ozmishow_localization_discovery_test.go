package services

import (
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestCanonicalOzmishowTranslationKeyIsStableAndUnambiguous(t *testing.T) {
	key := CanonicalOzmishowTranslationKey("sitePages", "page /: yüzde%", "hero.title:/ İstanbul")
	if key != CanonicalOzmishowTranslationKey("sitePages", "page /: yüzde%", "hero.title:/ İstanbul") {
		t.Fatal("canonical key changed for identical input")
	}
	for _, raw := range []string{"page /: yüzde%", "hero.title:/ İstanbul"} {
		if strings.Contains(key, raw) {
			t.Fatalf("canonical key did not encode reserved input %q: %q", raw, key)
		}
	}
	if key == CanonicalOzmishowTranslationKey("sitePages", "page", "/: yüzde%:hero.title:/ İstanbul") {
		t.Fatal("different resource/key boundaries collapsed to one canonical key")
	}
}

func TestCanonicalOzmishowTranslationKeyUsesGoQueryEscaping(t *testing.T) {
	got := CanonicalOzmishowTranslationKey("sitePages", "star*tilde~", "say!'() now")
	want := "ozmishow:sitePages:star%2Atilde~:say%21%27%28%29+now"
	if got != want {
		t.Fatalf("canonical key = %q, want %q", got, want)
	}
}

func TestDiscoverOzmishowRecordStringsReadsOnlySourceDictionary(t *testing.T) {
	for _, resourceType := range []string{"sitePages", "visualBlocks", "reusableBlocks"} {
		t.Run(resourceType, func(t *testing.T) {
			record := bson.M{
				"_id": primitive.NewObjectID(),
				"localization": bson.M{
					"en": bson.M{"hero.title": "Welcome", "cta / label": "Buy now"},
					"tr": bson.M{"hero.title": "Hoş geldiniz"},
				},
				"title": "must not be discovered",
			}

			got, err := DiscoverOzmishowRecordStrings(resourceType, record, "en")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("sources = %#v", got)
			}
			if got[0].PropertyPath != "cta / label" || got[1].PropertyPath != "hero.title" {
				t.Fatalf("property paths were not preserved and sorted: %#v", got)
			}
			for _, item := range got {
				if item.ResourceType != resourceType || item.ResourceID == "" || item.SourceHash == "" {
					t.Fatalf("incomplete ownership metadata: %#v", item)
				}
				if item.SourceText == "Hoş geldiniz" || item.SourceText == "must not be discovered" {
					t.Fatalf("discovered text outside source dictionary: %#v", item)
				}
			}
		})
	}
}

func TestDiscoverOzmishowRecordStringsFallsBackToVisualDocumentText(t *testing.T) {
	record := bson.M{
		"id": "hero-visual",
		"document": bson.M{"root": bson.M{
			"id": "root", "type": "section",
			"children": bson.A{
				bson.M{"id": "headline", "type": "heading", "text": "Build better pages"},
				bson.M{"id": "body", "type": "text", "text": "Everything stays reusable."},
				bson.M{"id": "cta", "type": "button", "label": "Start now"},
				bson.M{"id": "photo", "type": "image", "alt": "Team working together"},
				bson.M{"id": "theme", "type": "theme-switcher", "accessibleLabel": "Choose theme", "labels": bson.M{
					"light": "Light", "dark": "Dark", "system": "System",
				}},
			},
		}},
	}

	got, err := DiscoverOzmishowRecordStrings("visualBlocks", record, "en")
	if err != nil {
		t.Fatal(err)
	}
	texts := make(map[string]string, len(got))
	for _, item := range got {
		texts[item.PropertyPath] = item.SourceText
	}
	want := map[string]string{
		"visual.hero-visual.node.headline.text":         "Build better pages",
		"visual.hero-visual.node.body.text":             "Everything stays reusable.",
		"visual.hero-visual.node.cta.label":             "Start now",
		"visual.hero-visual.node.photo.alt":             "Team working together",
		"visual.hero-visual.node.theme.accessibleLabel": "Choose theme",
		"visual.hero-visual.node.theme.labels.light":    "Light",
		"visual.hero-visual.node.theme.labels.dark":     "Dark",
		"visual.hero-visual.node.theme.labels.system":   "System",
	}
	if !reflect.DeepEqual(texts, want) {
		t.Fatalf("visual source texts = %#v, want %#v", texts, want)
	}
}

func TestDiscoverOzmishowRecordStringsFindsSidebarVisualContentInsideSitePage(t *testing.T) {
	record := bson.M{
		"id": "page-docs",
		"localization": bson.M{"en": bson.M{
			"block.workflow-docs.interactive.introduction.label": "Localized introduction",
		}},
		"blocks": bson.A{bson.M{
			"id": "workflow-docs", "kind": "component", "componentKey": "legacy-component",
			"props": bson.M{"container": bson.M{
				"componentName": "SidebarContent",
				"twoPictureArray": bson.A{
					bson.M{"items": bson.A{
						bson.M{
							"id": "introduction", "label": "Introduction", "group": "Getting started",
							"document": bson.M{"root": bson.M{
								"id": "sidebar-root", "type": "section", "children": bson.A{
									bson.M{"id": "sidebar-title", "type": "heading", "text": "Configure your workflow"},
									bson.M{"id": "sidebar-body", "type": "text", "text": "Build the steps visually."},
								},
							}},
						},
					}},
				},
			}},
		}},
	}

	got, err := DiscoverOzmishowRecordStrings("sitePages", record, "en")
	if err != nil {
		t.Fatal(err)
	}
	texts := make(map[string]string, len(got))
	for _, item := range got {
		texts[item.PropertyPath] = item.SourceText
		if item.ResourceType != "sitePages" || item.ResourceID != "page-docs" {
			t.Fatalf("sidebar source ownership = %#v", item)
		}
	}
	want := map[string]string{
		"block.workflow-docs.interactive.introduction.label":                   "Localized introduction",
		"block.workflow-docs.interactive.introduction.group":                   "Getting started",
		"block.workflow-docs.interactive.introduction.node.sidebar-title.text": "Configure your workflow",
		"block.workflow-docs.interactive.introduction.node.sidebar-body.text":  "Build the steps visually.",
	}
	if !reflect.DeepEqual(texts, want) {
		t.Fatalf("sidebar source texts = %#v, want %#v", texts, want)
	}
}

func TestDiscoverOzmishowRecordStringsAcceptsStringID(t *testing.T) {
	got, err := DiscoverOzmishowRecordStrings("reusableBlocks", bson.M{
		"id": "header/main", "localization": bson.M{"en": bson.M{"nav.home": "Home"}},
	}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ResourceID != "header/main" || got[0].PropertyPath != "nav.home" {
		t.Fatalf("sources = %#v", got)
	}
}

func TestDiscoverOzmishowRecordStringsReportsMalformedRecords(t *testing.T) {
	if got, err := DiscoverOzmishowRecordStrings("sitePages", bson.M{"localization": bson.M{"en": bson.M{"title": "Home"}}}, "en"); err == nil || len(got) != 0 {
		t.Fatalf("missing ID sources/error = %#v/%v", got, err)
	}

	got, err := DiscoverOzmishowRecordStrings("sitePages", bson.M{
		"id": "home", "localization": bson.M{"en": bson.M{"title": "Home", "count": 4, "blank": ""}},
	}, "en")
	if err == nil || !strings.Contains(err.Error(), "count") {
		t.Fatalf("non-string diagnostic = %v", err)
	}
	if len(got) != 1 || got[0].PropertyPath != "title" {
		t.Fatalf("valid strings were not retained: %#v", got)
	}
}

func TestDiscoverOzmishowRecordsSkipsDuplicateIDs(t *testing.T) {
	records := []bson.M{
		{"id": "shared", "localization": bson.M{"en": bson.M{"title": "First"}}},
		{"id": "shared", "localization": bson.M{"en": bson.M{"title": "Second"}}},
		{"id": "unique", "localization": bson.M{"en": bson.M{"title": "Unique"}}},
	}
	got, diagnostics := discoverOzmishowRecords("sitePages", records, "en")
	if len(got) != 1 || got[0].ResourceID != "unique" {
		t.Fatalf("duplicate identity was not skipped: %#v", got)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Error(), "shared") {
		t.Fatalf("duplicate diagnostics = %#v", diagnostics)
	}
}

func TestIsNamespaceNotFoundForLocalization(t *testing.T) {
	if !isLocalizationNamespaceNotFound(mongoCommandError(26, "NamespaceNotFound")) {
		t.Fatal("namespace-not-found error was not recognized")
	}
	if isLocalizationNamespaceNotFound(mongoCommandError(13, "Unauthorized")) {
		t.Fatal("unrelated command error treated as an absent collection")
	}
}

func mongoCommandError(code int32, name string) error {
	return mongo.CommandError{Code: code, Name: name, Message: name}
}
