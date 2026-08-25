package models

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	PageNavigatorMaxLabelLength     = 100
	PageNavigatorMaxOverrides       = 20
	PageNavigatorMaxAdditionalItems = 20
)

func validatePageNavigatorLabel(field, value string, required bool) error {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return fmt.Errorf("pageNavigator %s is required", field)
	}
	if len([]rune(value)) > PageNavigatorMaxLabelLength {
		return fmt.Errorf("pageNavigator %s exceeds %d characters", field, PageNavigatorMaxLabelLength)
	}
	return nil
}

func validatePageNavigatorDestination(destination PageNavigatorDestination) error {
	pageID := strings.TrimSpace(destination.PageID)
	rawURL := strings.TrimSpace(destination.URL)
	switch destination.Type {
	case PageNavigatorDestinationPage:
		if pageID == "" {
			return fmt.Errorf("pageNavigator page destination requires pageId")
		}
		if rawURL != "" {
			return fmt.Errorf("pageNavigator page destination must not include url")
		}
	case PageNavigatorDestinationExternal:
		if pageID != "" {
			return fmt.Errorf("pageNavigator external destination must not include pageId")
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("pageNavigator external destination requires an absolute http(s) url")
		}
	default:
		return fmt.Errorf("pageNavigator destination type %q is invalid", destination.Type)
	}
	return nil
}

func validateSinglePageNavigatorConfig(config *PageNavigatorConfig) error {
	if config == nil {
		return nil
	}
	if config.Mode != PageNavigatorModeAutomatic && config.Mode != PageNavigatorModeCustom {
		return fmt.Errorf("pageNavigator mode %q is invalid", config.Mode)
	}
	if err := validatePageNavigatorLabel("homeLabel", config.HomeLabel, false); err != nil {
		return err
	}
	if len(config.Overrides) > PageNavigatorMaxOverrides {
		return fmt.Errorf("pageNavigator overrides exceeds %d items", PageNavigatorMaxOverrides)
	}
	for index, override := range config.Overrides {
		if strings.TrimSpace(override.PageID) == "" {
			return fmt.Errorf("pageNavigator override %d requires pageId", index)
		}
		if err := validatePageNavigatorLabel("override label", override.Label, false); err != nil {
			return err
		}
	}
	if len(config.AdditionalItems) > PageNavigatorMaxAdditionalItems {
		return fmt.Errorf("pageNavigator additionalItems exceeds %d items", PageNavigatorMaxAdditionalItems)
	}
	seen := make(map[string]struct{}, len(config.AdditionalItems))
	for index, item := range config.AdditionalItems {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("pageNavigator additionalItems[%d].id is required", index)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("pageNavigator contains duplicate additional item id %q", id)
		}
		seen[id] = struct{}{}
		if err := validatePageNavigatorLabel(fmt.Sprintf("additionalItems[%d].label", index), item.Label, true); err != nil {
			return err
		}
		if err := validatePageNavigatorDestination(item.Destination); err != nil {
			return fmt.Errorf("pageNavigator additionalItems[%d]: %w", index, err)
		}
	}
	return nil
}

func ValidatePageNavigatorConfig(page *PageModel) error {
	if page == nil {
		return nil
	}
	if err := validateSinglePageNavigatorConfig(page.PageNavigator); err != nil {
		return err
	}
	if page.SubPage != nil {
		if err := ValidatePageNavigatorConfig(page.SubPage); err != nil {
			return fmt.Errorf("subPage: %w", err)
		}
	}
	return nil
}
