package models

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	defaultBrandName    = "AutoTable"
	defaultBrandLogoURL = "/branding/autotable-logo.svg"
	defaultFaviconURL   = "/favicon.ico"
)

var brandingColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type BrandingAsset struct {
	URL      string `bson:"url" json:"url"`
	Provider string `bson:"provider" json:"provider"`
	AssetID  string `bson:"assetId" json:"assetId"`
	Width    int    `bson:"width" json:"width"`
	Height   int    `bson:"height" json:"height"`
	Format   string `bson:"format" json:"format"`
	Bytes    int64  `bson:"bytes" json:"bytes"`
}

type Branding struct {
	DisplayName          *string        `bson:"displayName,omitempty" json:"displayName,omitempty"`
	Logo                 *BrandingAsset `bson:"logo,omitempty" json:"logo,omitempty"`
	CompactLogo          *BrandingAsset `bson:"compactLogo,omitempty" json:"compactLogo,omitempty"`
	Favicon              *BrandingAsset `bson:"favicon,omitempty" json:"favicon,omitempty"`
	LogoAlt              *string        `bson:"logoAlt,omitempty" json:"logoAlt,omitempty"`
	PrimaryColor         *string        `bson:"primaryColor,omitempty" json:"primaryColor,omitempty"`
	LoginBrandingEnabled *bool          `bson:"loginBrandingEnabled,omitempty" json:"loginBrandingEnabled,omitempty"`
	Version              int64          `bson:"version,omitempty" json:"version,omitempty"`
}

type BrandingPatch struct {
	DisplayName          *string  `json:"displayName,omitempty"`
	LogoAlt              *string  `json:"logoAlt,omitempty"`
	PrimaryColor         *string  `json:"primaryColor,omitempty"`
	LoginBrandingEnabled *bool    `json:"loginBrandingEnabled,omitempty"`
	Reset                []string `json:"reset,omitempty"`
}

type RuntimeBranding struct {
	DisplayName          string `json:"displayName"`
	LogoURL              string `json:"logoUrl"`
	CompactLogoURL       string `json:"compactLogoUrl"`
	FaviconURL           string `json:"faviconUrl"`
	LogoAlt              string `json:"logoAlt"`
	PrimaryColor         string `json:"primaryColor"`
	LoginBrandingEnabled bool   `json:"loginBrandingEnabled"`
	Version              int64  `json:"version"`
}

func DefaultRuntimeBranding(projectName string) RuntimeBranding {
	displayName := strings.TrimSpace(projectName)
	if displayName == "" {
		displayName = defaultBrandName
	}
	return RuntimeBranding{
		DisplayName:          displayName,
		LogoURL:              defaultBrandLogoURL,
		CompactLogoURL:       defaultBrandLogoURL,
		FaviconURL:           defaultFaviconURL,
		LogoAlt:              displayName,
		PrimaryColor:         "#2563EB",
		LoginBrandingEnabled: true,
	}
}

func ResolveBranding(projectName string, tenant, project *Branding) RuntimeBranding {
	result := DefaultRuntimeBranding(projectName)
	applyBranding := func(value *Branding) {
		if value == nil {
			return
		}
		if value.DisplayName != nil {
			result.DisplayName = *value.DisplayName
		}
		if value.Logo != nil && value.Logo.URL != "" {
			result.LogoURL = value.Logo.URL
		}
		if value.CompactLogo != nil && value.CompactLogo.URL != "" {
			result.CompactLogoURL = value.CompactLogo.URL
		} else if value.Logo != nil && value.Logo.URL != "" {
			result.CompactLogoURL = value.Logo.URL
		}
		if value.Favicon != nil && value.Favicon.URL != "" {
			result.FaviconURL = value.Favicon.URL
		} else if value.CompactLogo != nil && compatibleFaviconFormat(value.CompactLogo.Format) {
			result.FaviconURL = value.CompactLogo.URL
		} else if value.Logo != nil && compatibleFaviconFormat(value.Logo.Format) {
			result.FaviconURL = value.Logo.URL
		}
		if value.LogoAlt != nil {
			result.LogoAlt = *value.LogoAlt
		}
		if value.PrimaryColor != nil {
			result.PrimaryColor = *value.PrimaryColor
		}
		if value.LoginBrandingEnabled != nil {
			result.LoginBrandingEnabled = *value.LoginBrandingEnabled
		}
		if value.Version > result.Version {
			result.Version = value.Version
		}
	}

	applyBranding(tenant)
	applyBranding(project)
	if project == nil || project.LogoAlt == nil {
		if tenant == nil || tenant.LogoAlt == nil {
			result.LogoAlt = result.DisplayName
		}
	}
	return result
}

func compatibleFaviconFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png", "jpeg", "jpg", "webp", "ico":
		return true
	default:
		return false
	}
}

func ValidateBrandingPatch(patch BrandingPatch) (BrandingPatch, error) {
	if patch.DisplayName != nil {
		value := strings.TrimSpace(*patch.DisplayName)
		if value == "" || len(value) > 100 {
			return BrandingPatch{}, fmt.Errorf("displayName must be between 1 and 100 characters")
		}
		patch.DisplayName = &value
	}
	if patch.LogoAlt != nil {
		value := strings.TrimSpace(*patch.LogoAlt)
		if value == "" || len(value) > 160 {
			return BrandingPatch{}, fmt.Errorf("logoAlt must be between 1 and 160 characters")
		}
		patch.LogoAlt = &value
	}
	if patch.PrimaryColor != nil {
		value := strings.ToUpper(strings.TrimSpace(*patch.PrimaryColor))
		if !brandingColorPattern.MatchString(value) {
			return BrandingPatch{}, fmt.Errorf("primaryColor must be a six-digit hex color")
		}
		patch.PrimaryColor = &value
	}
	allowedReset := map[string]bool{
		"displayName": true, "logoAlt": true, "primaryColor": true,
		"loginBrandingEnabled": true,
	}
	setFields := map[string]bool{
		"displayName":          patch.DisplayName != nil,
		"logoAlt":              patch.LogoAlt != nil,
		"primaryColor":         patch.PrimaryColor != nil,
		"loginBrandingEnabled": patch.LoginBrandingEnabled != nil,
	}
	seen := map[string]bool{}
	for _, field := range patch.Reset {
		if !allowedReset[field] {
			return BrandingPatch{}, fmt.Errorf("unsupported branding reset field %q", field)
		}
		if seen[field] {
			return BrandingPatch{}, fmt.Errorf("duplicate branding reset field %q", field)
		}
		if setFields[field] {
			return BrandingPatch{}, fmt.Errorf("branding field %q cannot be set and reset together", field)
		}
		seen[field] = true
	}
	return patch, nil
}
