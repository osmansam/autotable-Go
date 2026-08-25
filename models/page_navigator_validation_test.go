package models

import (
	"strings"
	"testing"
)

func validPageNavigatorConfig() *PageNavigatorConfig {
	return &PageNavigatorConfig{
		Enabled:  true,
		Mode:     PageNavigatorModeAutomatic,
		ShowHome: true,
		Overrides: []PageNavigatorOverride{{
			PageID: "507f1f77bcf86cd799439011",
			Label:  "Catalog",
		}},
		AdditionalItems: []PageNavigatorAdditionalItem{{
			ID:    "docs",
			Label: "Docs",
			Destination: PageNavigatorDestination{
				Type: PageNavigatorDestinationExternal,
				URL:  "https://docs.example.com",
			},
			OpenInNewTab: true,
		}},
	}
}

func TestValidatePageNavigatorConfigAcceptsLegacyAndValidConfiguration(t *testing.T) {
	for _, page := range []*PageModel{
		{Name: "Legacy"},
		{Name: "Automatic", PageNavigator: validPageNavigatorConfig()},
		{Name: "Custom", PageNavigator: &PageNavigatorConfig{
			Enabled: true,
			Mode:    PageNavigatorModeCustom,
			AdditionalItems: []PageNavigatorAdditionalItem{{
				ID: "orders", Label: "Orders",
				Destination: PageNavigatorDestination{
					Type:   PageNavigatorDestinationPage,
					PageID: "507f1f77bcf86cd799439012",
				},
			}},
		}},
	} {
		if err := ValidatePageNavigatorConfig(page); err != nil {
			t.Fatalf("ValidatePageNavigatorConfig(%q) error = %v", page.Name, err)
		}
	}
}

func TestValidatePageNavigatorConfigRejectsInvalidValues(t *testing.T) {
	longLabel := strings.Repeat("x", PageNavigatorMaxLabelLength+1)
	tooManyOverrides := make([]PageNavigatorOverride, PageNavigatorMaxOverrides+1)
	tooManyItems := make([]PageNavigatorAdditionalItem, PageNavigatorMaxAdditionalItems+1)
	tests := []struct {
		name    string
		config  *PageNavigatorConfig
		wantErr string
	}{
		{name: "mode", config: &PageNavigatorConfig{Enabled: true, Mode: "freeform"}, wantErr: "mode"},
		{name: "home label", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, HomeLabel: longLabel}, wantErr: "homeLabel"},
		{name: "override label", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, Overrides: []PageNavigatorOverride{{PageID: "page", Label: longLabel}}}, wantErr: "override label"},
		{name: "too many overrides", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, Overrides: tooManyOverrides}, wantErr: "overrides"},
		{name: "too many items", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: tooManyItems}, wantErr: "additionalItems"},
		{name: "duplicate id", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{
			{ID: "same", Label: "One", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationExternal, URL: "https://one.example"}},
			{ID: " same ", Label: "Two", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationExternal, URL: "https://two.example"}},
		}}, wantErr: "duplicate"},
		{name: "empty label", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{{ID: "item", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationExternal, URL: "https://example.com"}}}}, wantErr: "label"},
		{name: "missing page id", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{{ID: "page", Label: "Page", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationPage}}}}, wantErr: "pageId"},
		{name: "mixed page destination", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{{ID: "page", Label: "Page", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationPage, PageID: "page", URL: "https://example.com"}}}}, wantErr: "url"},
		{name: "missing external url", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{{ID: "external", Label: "External", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationExternal}}}}, wantErr: "url"},
		{name: "mixed external destination", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{{ID: "external", Label: "External", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationExternal, PageID: "page", URL: "https://example.com"}}}}, wantErr: "pageId"},
		{name: "unsafe external url", config: &PageNavigatorConfig{Enabled: true, Mode: PageNavigatorModeAutomatic, AdditionalItems: []PageNavigatorAdditionalItem{{ID: "external", Label: "External", Destination: PageNavigatorDestination{Type: PageNavigatorDestinationExternal, URL: "javascript:alert(1)"}}}}, wantErr: "absolute http(s)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePageNavigatorConfig(&PageModel{Name: tc.name, PageNavigator: tc.config})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidatePageNavigatorConfigValidatesSubPage(t *testing.T) {
	page := &PageModel{Name: "Parent", SubPage: &PageModel{
		Name:          "Child",
		PageNavigator: &PageNavigatorConfig{Enabled: true, Mode: "invalid"},
	}}
	if err := ValidatePageNavigatorConfig(page); err == nil || !strings.Contains(err.Error(), "subPage") {
		t.Fatalf("error = %v, want nested page context", err)
	}
}
