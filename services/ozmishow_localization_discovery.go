package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var ozmishowLocalizationCollections = []string{"sitePages", "visualBlocks", "reusableBlocks"}

var visualNodeTextProperties = map[string][]string{
	"heading":        {"text"},
	"text":           {"text"},
	"image":          {"alt"},
	"button":         {"label"},
	"theme-switcher": {"accessibleLabel", "labels.light", "labels.dark", "labels.system"},
	"menu-toggle":    {"label"},
}

var (
	localizationPageCollection      = utils.GetPageCollectionForProject
	localizationContainerCollection = utils.GetContainerCollectionForProject
	localizationDynamicCollection   = utils.GetDynamicCollectionForProject
)

func CanonicalOzmishowTranslationKey(resourceType, resourceID, localKey string) string {
	return "ozmishow:" + url.QueryEscape(resourceType) + ":" + url.QueryEscape(resourceID) + ":" + url.QueryEscape(localKey)
}

func DiscoverOzmishowRecordStrings(resourceType string, record bson.M, sourceLocale string) ([]models.SourceString, error) {
	resourceID, ok := ozmishowRecordID(record)
	if !ok {
		return nil, fmt.Errorf("%s record is missing a stable id or _id", resourceType)
	}
	values := make(map[string]string)
	issues := make([]error, 0)
	if localization, found := stringMap(record["localization"]); found {
		if dictionary, found := stringMap(localization[sourceLocale]); found {
			for localKey, value := range dictionary {
				text, stringValue := value.(string)
				if !stringValue {
					issues = append(issues, fmt.Errorf("%s %q localization key %q is not a string", resourceType, resourceID, localKey))
					continue
				}
				if text != "" {
					values[localKey] = text
				}
			}
		}
	}
	if resourceType == "visualBlocks" {
		fallbacks, visualIssues := discoverVisualDocumentTexts(record, resourceID)
		issues = append(issues, visualIssues...)
		for localKey, text := range fallbacks {
			if _, explicit := values[localKey]; !explicit {
				values[localKey] = text
			}
		}
	}
	if resourceType == "sitePages" {
		fallbacks, sidebarIssues := discoverSitePageSidebarTexts(record, resourceID)
		issues = append(issues, sidebarIssues...)
		for localKey, text := range fallbacks {
			if _, explicit := values[localKey]; !explicit {
				values[localKey] = text
			}
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]models.SourceString, 0, len(keys))
	for _, localKey := range keys {
		text := values[localKey]
		result = append(result, models.SourceString{
			TranslationKey: CanonicalOzmishowTranslationKey(resourceType, resourceID, localKey),
			ResourceType:   resourceType,
			ResourceID:     resourceID,
			PropertyPath:   localKey,
			SourceText:     text,
			SourceHash:     LocalizationSourceHash(sourceLocale, text),
			Context:        resourceType + " " + resourceID + " localization field " + localKey,
		})
	}
	return result, errors.Join(issues...)
}

func discoverVisualDocumentTexts(record bson.M, resourceID string) (map[string]string, []error) {
	document, ok := stringMap(record["document"])
	if !ok {
		return map[string]string{}, nil
	}
	return discoverVisualDocumentTextValues(document, "visual."+resourceID, "visualBlocks "+fmt.Sprintf("%q", resourceID))
}

func discoverVisualDocumentTextValues(document map[string]interface{}, keyPrefix, issueContext string) (map[string]string, []error) {
	result := make(map[string]string)
	root, ok := stringMap(document["root"])
	if !ok {
		return result, nil
	}
	issues := make([]error, 0)
	var visit func(map[string]interface{})
	visit = func(node map[string]interface{}) {
		nodeType, _ := node["type"].(string)
		properties := visualNodeTextProperties[nodeType]
		nodeID, _ := node["id"].(string)
		if len(properties) > 0 && strings.TrimSpace(nodeID) == "" {
			issues = append(issues, fmt.Errorf("%s has a localizable %s node without an id", issueContext, nodeType))
		} else {
			for _, property := range properties {
				text, found := nestedString(node, property)
				if !found || strings.TrimSpace(text) == "" {
					continue
				}
				key := keyPrefix + ".node." + nodeID + "." + property
				result[key] = text
			}
		}
		for _, child := range interfaceSlice(node["children"]) {
			mapped, found := stringMap(child)
			if !found {
				issues = append(issues, fmt.Errorf("%s contains a malformed child node", issueContext))
				continue
			}
			visit(mapped)
		}
	}
	visit(root)
	return result, issues
}

func discoverSitePageSidebarTexts(record bson.M, resourceID string) (map[string]string, []error) {
	result := make(map[string]string)
	issues := make([]error, 0)
	for _, rawBlock := range interfaceSlice(record["blocks"]) {
		block, ok := stringMap(rawBlock)
		if !ok || block["kind"] != "component" || block["componentKey"] != "legacy-component" {
			continue
		}
		props, ok := stringMap(block["props"])
		if !ok {
			continue
		}
		container, ok := stringMap(props["container"])
		if !ok || container["componentName"] != "SidebarContent" {
			continue
		}
		blockID, _ := block["id"].(string)
		if strings.TrimSpace(blockID) == "" {
			issues = append(issues, fmt.Errorf("sitePages %q has a SidebarContent block without an id", resourceID))
			continue
		}
		documents := interfaceSlice(container["twoPictureArray"])
		if len(documents) == 0 {
			continue
		}
		sidebar, ok := stringMap(documents[0])
		if !ok {
			issues = append(issues, fmt.Errorf("sitePages %q SidebarContent block %q has a malformed document", resourceID, blockID))
			continue
		}
		for _, rawItem := range interfaceSlice(sidebar["items"]) {
			item, ok := stringMap(rawItem)
			if !ok {
				issues = append(issues, fmt.Errorf("sitePages %q SidebarContent block %q contains a malformed item", resourceID, blockID))
				continue
			}
			itemID, _ := item["id"].(string)
			if strings.TrimSpace(itemID) == "" {
				issues = append(issues, fmt.Errorf("sitePages %q SidebarContent block %q has an item without an id", resourceID, blockID))
				continue
			}
			keyPrefix := "block." + blockID + ".interactive." + itemID
			for _, property := range []string{"label", "group"} {
				text, found := item[property].(string)
				if found && strings.TrimSpace(text) != "" {
					result[keyPrefix+"."+property] = text
				}
			}
			document, ok := stringMap(item["document"])
			if !ok {
				continue
			}
			visualTexts, visualIssues := discoverVisualDocumentTextValues(
				document,
				keyPrefix,
				fmt.Sprintf("sitePages %q SidebarContent block %q item %q", resourceID, blockID, itemID),
			)
			for key, text := range visualTexts {
				result[key] = text
			}
			issues = append(issues, visualIssues...)
		}
	}
	return result, issues
}

func nestedString(value map[string]interface{}, property string) (string, bool) {
	var current interface{} = value
	for _, segment := range strings.Split(property, ".") {
		mapped, ok := stringMap(current)
		if !ok {
			return "", false
		}
		current, ok = mapped[segment]
		if !ok {
			return "", false
		}
	}
	text, ok := current.(string)
	return text, ok
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case bson.A:
		return []interface{}(typed)
	case []interface{}:
		return typed
	default:
		return nil
	}
}

func discoverOzmishowRecords(resourceType string, records []bson.M, sourceLocale string) ([]models.SourceString, []error) {
	counts := make(map[string]int, len(records))
	missing := make(map[int]error)
	for index, record := range records {
		resourceID, ok := ozmishowRecordID(record)
		if !ok {
			missing[index] = fmt.Errorf("%s record %d is missing a stable id or _id", resourceType, index)
			continue
		}
		counts[resourceID]++
	}
	duplicateReported := map[string]bool{}
	result := make([]models.SourceString, 0)
	issues := make([]error, 0)
	for index, record := range records {
		if issue, ok := missing[index]; ok {
			issues = append(issues, issue)
			continue
		}
		resourceID, _ := ozmishowRecordID(record)
		if counts[resourceID] > 1 {
			if !duplicateReported[resourceID] {
				issues = append(issues, fmt.Errorf("%s has duplicate resource id %q; all duplicates were skipped", resourceType, resourceID))
				duplicateReported[resourceID] = true
			}
			continue
		}
		sources, err := DiscoverOzmishowRecordStrings(resourceType, record, sourceLocale)
		result = append(result, sources...)
		if err != nil {
			issues = append(issues, err)
		}
	}
	return result, issues
}

func DiscoverAllProjectStrings(ctx context.Context, tenantID, projectID primitive.ObjectID, sourceLocale string) ([]models.SourceString, error) {
	byKey := map[string]models.SourceString{}
	fatalIssues := make([]error, 0)
	recordIssues := make([]error, 0)
	add := func(items []models.SourceString) {
		for _, item := range items {
			if _, duplicate := byKey[item.TranslationKey]; duplicate {
				recordIssues = append(recordIssues, fmt.Errorf("duplicate translation key %q was skipped", item.TranslationKey))
				continue
			}
			byKey[item.TranslationKey] = item
		}
	}

	var pages []models.PageModel
	if err := readLocalizationCollection(ctx, localizationPageCollection(tenantID.Hex(), projectID.Hex()), &pages); err != nil {
		fatalIssues = append(fatalIssues, fmt.Errorf("discover pages: %w", err))
	} else {
		for _, page := range pages {
			add(DiscoverPageStrings(page, sourceLocale))
		}
	}

	var containers []models.ContainerModel
	if err := readLocalizationCollection(ctx, localizationContainerCollection(tenantID.Hex(), projectID.Hex()), &containers); err != nil {
		fatalIssues = append(fatalIssues, fmt.Errorf("discover containers: %w", err))
	} else {
		for _, container := range containers {
			add(DiscoverContainerStrings(container, sourceLocale))
		}
	}

	for _, resourceType := range ozmishowLocalizationCollections {
		var records []bson.M
		collection := localizationDynamicCollection(tenantID.Hex(), projectID.Hex(), resourceType)
		if err := readLocalizationCollection(ctx, collection, &records); err != nil {
			fatalIssues = append(fatalIssues, fmt.Errorf("discover %s: %w", resourceType, err))
			continue
		}
		items, itemIssues := discoverOzmishowRecords(resourceType, records, sourceLocale)
		add(items)
		recordIssues = append(recordIssues, itemIssues...)
	}
	for _, issue := range recordIssues {
		log.Printf("localization_discovery skipped tenant_id=%s project_id=%s issue=%q", tenantID.Hex(), projectID.Hex(), issue.Error())
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]models.SourceString, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result, errors.Join(fatalIssues...)
}

func HasOzmishowLocalizedContent(ctx context.Context, tenantID, projectID primitive.ObjectID) (bool, error) {
	for _, resourceType := range ozmishowLocalizationCollections {
		var records []bson.M
		collection := localizationDynamicCollection(tenantID.Hex(), projectID.Hex(), resourceType)
		if err := readLocalizationCollection(ctx, collection, &records); err != nil {
			return false, err
		}
		for _, record := range records {
			localization, ok := stringMap(record["localization"])
			if !ok {
				continue
			}
			for _, rawDictionary := range localization {
				dictionary, ok := stringMap(rawDictionary)
				if !ok {
					continue
				}
				for _, value := range dictionary {
					if text, ok := value.(string); ok && text != "" {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

func readLocalizationCollection(ctx context.Context, collection *mongo.Collection, destination interface{}) error {
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		if isLocalizationNamespaceNotFound(err) {
			return nil
		}
		return err
	}
	defer cursor.Close(ctx)
	if err := cursor.All(ctx, destination); err != nil {
		if isLocalizationNamespaceNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func isLocalizationNamespaceNotFound(err error) bool {
	var commandError mongo.CommandError
	return errors.As(err, &commandError) && (commandError.Code == 26 || commandError.Name == "NamespaceNotFound")
}

func ozmishowRecordID(record bson.M) (string, bool) {
	for _, key := range []string{"id", "_id"} {
		switch value := record[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return value, true
			}
		case primitive.ObjectID:
			if !value.IsZero() {
				return value.Hex(), true
			}
		}
	}
	return "", false
}

func stringMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case bson.M:
		return map[string]interface{}(typed), true
	case map[string]interface{}:
		return typed, true
	case bson.D:
		return typed.Map(), true
	default:
		return nil, false
	}
}
