package models

import "testing"

func brandingString(value string) *string { return &value }

func TestResolveBrandingUsesFieldLevelPrecedence(t *testing.T) {
	tenant := &Branding{
		DisplayName:  brandingString("Tenant"),
		PrimaryColor: brandingString("#112233"),
		Logo:         &BrandingAsset{URL: "tenant.png"},
	}
	project := &Branding{
		DisplayName: brandingString("Project"),
		CompactLogo: &BrandingAsset{URL: "compact.png"},
	}

	got := ResolveBranding("Stored Project", tenant, project)

	if got.DisplayName != "Project" || got.PrimaryColor != "#112233" || got.LogoURL != "tenant.png" || got.CompactLogoURL != "compact.png" {
		t.Fatalf("unexpected effective branding: %#v", got)
	}
}

func TestResolveBrandingFallsBackForLegacyProject(t *testing.T) {
	got := ResolveBranding("Inventory", nil, nil)

	if got.DisplayName != "Inventory" || got.LogoURL == "" || got.FaviconURL == "" || got.PrimaryColor == "" {
		t.Fatalf("missing platform fallback: %#v", got)
	}
}

func TestResolveBrandingDerivesCompactFaviconAndAlt(t *testing.T) {
	project := &Branding{
		DisplayName: brandingString("Acme"),
		Logo:        &BrandingAsset{URL: "acme.webp", Format: "webp"},
	}

	got := ResolveBranding("Stored Project", nil, project)

	if got.CompactLogoURL != "acme.webp" || got.FaviconURL != "acme.webp" || got.LogoAlt != "Acme" {
		t.Fatalf("derived fallbacks are incorrect: %#v", got)
	}
}

func TestValidateBrandingPatchNormalizesSupportedValues(t *testing.T) {
	patch := BrandingPatch{
		DisplayName:          brandingString("  Acme Inventory  "),
		LogoAlt:              brandingString("  Acme logo  "),
		PrimaryColor:         brandingString("#a1b2c3"),
		LoginBrandingEnabled: func() *bool { value := true; return &value }(),
	}

	got, err := ValidateBrandingPatch(patch)
	if err != nil {
		t.Fatal(err)
	}
	if *got.DisplayName != "Acme Inventory" || *got.LogoAlt != "Acme logo" || *got.PrimaryColor != "#A1B2C3" {
		t.Fatalf("patch was not normalized: %#v", got)
	}
}

func TestValidateBrandingPatchRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		patch BrandingPatch
	}{
		{name: "empty display name", patch: BrandingPatch{DisplayName: brandingString("  ")}},
		{name: "invalid color", patch: BrandingPatch{PrimaryColor: brandingString("blue")}},
		{name: "unknown reset", patch: BrandingPatch{Reset: []string{"version"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ValidateBrandingPatch(tt.patch); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
