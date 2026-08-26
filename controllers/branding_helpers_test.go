package controllers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
)

func controllerString(value string) *string { return &value }

func TestBrandingPatchUpdateUsesSetAndUnset(t *testing.T) {
	patch := models.BrandingPatch{
		DisplayName: controllerString("Acme"),
		Reset:       []string{"logoAlt"},
	}

	set, unset := brandingUpdateDocuments(patch, 7, time.Unix(1, 0))

	if set["branding.displayName"] != "Acme" || set["branding.version"] != int64(8) {
		t.Fatalf("bad set document: %#v", set)
	}
	if unset["branding.logoAlt"] != "" {
		t.Fatalf("bad unset document: %#v", unset)
	}
}

func TestBrandingAssetSlotRejectsNestedMetadata(t *testing.T) {
	for _, slot := range []string{"logo.assetId", "version", "unknown"} {
		if _, err := brandingAssetField(slot); err == nil {
			t.Fatalf("brandingAssetField(%q) accepted an unsafe slot", slot)
		}
	}
	for _, slot := range []string{"logo", "compactLogo", "favicon"} {
		if _, err := brandingAssetField(slot); err != nil {
			t.Fatalf("brandingAssetField(%q) error = %v", slot, err)
		}
	}
}

func TestRuntimeBrandingJSONExcludesManagementMetadata(t *testing.T) {
	stored := &models.Branding{
		Logo: &models.BrandingAsset{
			URL: "https://cdn.example/logo.png", Provider: "cloudinary",
			AssetID: "secret-id", Bytes: 1234,
		},
		Version: 3,
	}
	runtime := models.ResolveBranding("Acme", nil, stored)

	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	if !strings.Contains(got, `"logoUrl":"https://cdn.example/logo.png"`) || !strings.Contains(got, `"version":3`) {
		t.Fatalf("runtime response is missing renderable fields: %s", got)
	}
	for _, forbidden := range []string{"assetId", "secret-id", "provider", "bytes", "tenantId", "projectId"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("runtime response leaked %q: %s", forbidden, got)
		}
	}
}

func TestProjectScopeMayOnlyManageItsCurrentProject(t *testing.T) {
	if err := validateBrandingProjectScope("project", "project-a", "project-a"); err != nil {
		t.Fatalf("own project rejected: %v", err)
	}
	if err := validateBrandingProjectScope("project", "project-a", "project-b"); err == nil {
		t.Fatal("project-scoped access crossed project boundary")
	}
	if err := validateBrandingProjectScope("tenant", "", "project-b"); err != nil {
		t.Fatalf("tenant-scoped access rejected: %v", err)
	}
}
