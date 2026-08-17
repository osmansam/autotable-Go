package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
	"golang.org/x/text/language"
)

var (
	fieldSeparatorsPattern = regexp.MustCompile(`[_-]+`)
	fieldCamelCasePattern  = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	fieldWhitespacePattern = regexp.MustCompile(`\s+`)
)

func localizationFieldLabel(name string) string {
	label := fieldSeparatorsPattern.ReplaceAllString(name, " ")
	label = fieldCamelCasePattern.ReplaceAllString(label, "$1 $2")
	label = strings.TrimSpace(fieldWhitespacePattern.ReplaceAllString(label, " "))
	if label == "" {
		return ""
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

func LocalizationSourceHash(locale, text string) string {
	canonicalLocale := strings.ToLower(locale)
	if tag, err := language.Parse(locale); err == nil {
		canonicalLocale = tag.String()
	}
	sum := sha256.Sum256([]byte(canonicalLocale + "\x00" + text))
	return hex.EncodeToString(sum[:])
}

func BuildTranslationReconciliation(sources []models.SourceString, existing []models.TranslationEntry, now time.Time) []models.TranslationEntry {
	sourceByKey := make(map[string]models.SourceString, len(sources))
	for _, source := range sources {
		sourceByKey[source.TranslationKey] = source
	}
	changes := make([]models.TranslationEntry, 0, len(existing))
	for _, entry := range existing {
		source, found := sourceByKey[entry.TranslationKey]
		if !found {
			if entry.IsActive {
				orphanedAt := now
				entry.IsActive = false
				entry.OrphanedAt = &orphanedAt
				entry.UpdatedAt = now
				changes = append(changes, entry)
			}
			continue
		}
		if entry.SourceHash != source.SourceHash {
			entry.Status = models.TranslationStatusOutdated
		}
		entry.ResourceType = source.ResourceType
		entry.ResourceID = source.ResourceID
		entry.PropertyPath = source.PropertyPath
		entry.SourceText = source.SourceText
		entry.SourceHash = source.SourceHash
		entry.IsActive = true
		entry.OrphanedAt = nil
		entry.LastDiscovered = now
		entry.UpdatedAt = now
		changes = append(changes, entry)
	}
	return changes
}

func DiscoverPageStrings(page models.PageModel, sourceLocale string) []models.SourceString {
	pageID := page.ID.Hex()
	result := make([]models.SourceString, 0)
	appendSource := func(key, resourceType, resourceID, propertyPath, sourceText, context string) {
		if sourceText == "" {
			return
		}
		result = append(result, models.SourceString{
			TranslationKey: key,
			ResourceType:   resourceType,
			ResourceID:     resourceID,
			PropertyPath:   propertyPath,
			SourceText:     sourceText,
			SourceHash:     LocalizationSourceHash(sourceLocale, sourceText),
			Context:        context,
		})
	}

	appendSource("page:"+pageID+".name", "page", pageID, "name", page.Name, "Page or navigation title")
	for _, filter := range page.Filters {
		if filter.ID == "" {
			continue
		}
		appendSource(
			fmt.Sprintf("page:%s.filter:%s.label", pageID, filter.ID),
			"pageFilter", filter.ID, "label", filter.Label, "Filter label on page "+page.Name,
		)
	}

	var visitComponent func(models.ComponentBlock)
	visitComponent = func(component models.ComponentBlock) {
		if component.ID == "" {
			return
		}
		prefix := fmt.Sprintf("page:%s.component:%s", pageID, component.ID)
		appendSource(prefix+".title", "component", component.ID, "title", component.Title, "Component title on page "+page.Name)
		if component.Table != nil {
			for _, column := range component.Table.Columns {
				if column.Field == "" {
					continue
				}
				appendSource(
					fmt.Sprintf("%s.column:%s.displayName", prefix, column.Field),
					"component", component.ID, "table.columns."+column.Field+".displayName", column.DisplayName,
					"Table column heading for field "+column.Field+" on page "+page.Name,
				)
			}
		}
		for _, tab := range component.Tabs {
			for _, child := range tab.Components {
				visitComponent(child)
			}
		}
	}

	var visitSections func([]models.Section)
	visitSections = func(sections []models.Section) {
		for _, section := range sections {
			if section.Component != nil {
				visitComponent(*section.Component)
			}
			if section.Grid != nil {
				for _, cell := range section.Grid.Cells {
					for _, component := range cell.Components {
						visitComponent(component)
					}
				}
			}
			for _, cell := range section.Cells {
				for _, component := range cell.Components {
					visitComponent(component)
				}
			}
			if section.Tabs != nil {
				for _, tab := range section.Tabs.Tabs {
					appendSource(
						fmt.Sprintf("page:%s.tab:%s.label", pageID, tab.ID),
						"pageTab", tab.ID, "label", tab.Label, "Tab label on page "+page.Name,
					)
					visitSections(tab.Sections)
				}
			}
		}
	}
	visitSections(page.Sections)
	if page.SubPage != nil {
		result = append(result, DiscoverPageStrings(*page.SubPage, sourceLocale)...)
	}
	return result
}

func DiscoverContainerStrings(container models.ContainerModel, sourceLocale string) []models.SourceString {
	containerID := container.ID.Hex()
	result := make([]models.SourceString, 0)
	appendSource := func(key, resourceType, resourceID, propertyPath, sourceText, context string) {
		if sourceText == "" {
			return
		}
		result = append(result, models.SourceString{
			TranslationKey: key,
			ResourceType:   resourceType,
			ResourceID:     resourceID,
			PropertyPath:   propertyPath,
			SourceText:     sourceText,
			SourceHash:     LocalizationSourceHash(sourceLocale, sourceText),
			Context:        context,
		})
	}

	var visitFields func([]models.Field, string)
	visitFields = func(fields []models.Field, parentPath string) {
		for _, field := range fields {
			if field.Name == "" {
				continue
			}
			fieldPath := parentPath + "field:" + field.Name
			keyPrefix := fmt.Sprintf("container:%s.%s", containerID, fieldPath)
			fieldLabel := localizationFieldLabel(field.Name)
			if field.Frontend != nil && field.Frontend.DisplayName != "" {
				fieldLabel = field.Frontend.DisplayName
			}
			appendSource(keyPrefix+".displayName", "containerField", field.Name, fieldPath+".frontend.displayName", fieldLabel, "Display label for field "+field.Name)
			if field.Frontend != nil {
				for _, action := range field.Frontend.Actions {
					discoverActionStrings(appendSource, keyPrefix, fieldPath, action, container.SchemaName)
				}
			}
			if field.PopulationSettings != nil {
				appendSource(keyPrefix+".population.displayLabel", "containerField", field.Name, fieldPath+".populationSettings.displayLabel", field.PopulationSettings.DisplayLabel, "Lookup display label for field "+field.Name)
			}
			visitFields(field.Children, fieldPath+".")
		}
	}
	visitFields(container.Fields, "")
	if container.Frontend != nil {
		appendSource("container:"+containerID+".displayName", "container", containerID, "frontend.displayName", container.Frontend.DisplayName, "Display name for schema "+container.SchemaName)
		for _, action := range container.Frontend.Actions {
			discoverActionStrings(appendSource, "container:"+containerID, "frontend", action, container.SchemaName)
		}
	}
	return result
}

type appendSourceString func(key, resourceType, resourceID, propertyPath, sourceText, context string)

func discoverActionStrings(appendSource appendSourceString, ownerKey, ownerPath string, action models.ActionConfig, contextName string) {
	actionID := action.ID
	if actionID == "" {
		actionID = action.Key
	}
	if actionID == "" {
		return
	}
	prefix := ownerKey + ".action:" + actionID
	propertyPrefix := ownerPath + ".actions." + actionID
	context := "Action on " + contextName
	appendSource(prefix+".name", "action", actionID, propertyPrefix+".name", action.Name, context)
	appendSource(prefix+".label", "action", actionID, propertyPrefix+".label", action.Label, context)
	appendSource(prefix+".buttonName", "action", actionID, propertyPrefix+".buttonName", action.ButtonName, context)
	appendSource(prefix+".confirmTitle", "action", actionID, propertyPrefix+".confirmTitle", action.ConfirmTitle, context)
	appendSource(prefix+".confirmText", "action", actionID, propertyPrefix+".confirmText", action.ConfirmText, context)
	if action.FormFields == nil {
		return
	}
	for _, field := range *action.FormFields {
		fieldID := field.ID
		if fieldID == "" {
			fieldID = field.FormKey
		}
		if fieldID == "" {
			continue
		}
		fieldPrefix := prefix + ".formField:" + fieldID
		fieldPath := propertyPrefix + ".formFields." + fieldID
		appendSource(fieldPrefix+".label", "actionFormField", fieldID, fieldPath+".label", field.Label, "Form field for "+context)
		appendSource(fieldPrefix+".placeholder", "actionFormField", fieldID, fieldPath+".placeholder", field.Placeholder, "Form field for "+context)
		appendSource(fieldPrefix+".validationMessage", "actionFormField", fieldID, fieldPath+".validationMessage", field.ValidationMessage, "Validation message for "+context)
		for _, option := range field.StaticOptions {
			if option.ID == "" {
				continue
			}
			appendSource(fieldPrefix+".option:"+option.ID+".label", "actionFormOption", option.ID, fieldPath+".staticOptions."+option.ID+".label", option.Label, "Option for "+context)
		}
	}
}
