package requests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/files"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
)

type DynamicRequestParser struct {
	uploadService *files.UploadService
}

type SearchParams struct {
	SearchKey string
	Sort      bson.D
	Pager     utils.Pager
}

type BatchLimitError struct {
	Operation string
	Requested int
	Max       int
}

func (e *BatchLimitError) Error() string {
	return fmt.Sprintf("%s limit exceeded. Maximum allowed items is %d.", e.Operation, e.Max)
}

func NewDynamicRequestParser(uploadService *files.UploadService) *DynamicRequestParser {
	return &DynamicRequestParser{uploadService: uploadService}
}

func (p *DynamicRequestParser) ParseCreateItem(c *fiber.Ctx, container *models.ContainerModel) (map[string]interface{}, error) {
	return p.parseItem(c, container)
}

func (p *DynamicRequestParser) ParseCreateItems(c *fiber.Ctx, container *models.ContainerModel, maxItems int) ([]map[string]interface{}, error) {
	if strings.Contains(c.Get("Content-Type"), "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			return nil, err
		}

		itemsJSON, exists := form.Value["items"]
		if !exists || len(itemsJSON) == 0 {
			return nil, errors.New("missing items field")
		}

		items, err := parseLimitedItems([]byte(itemsJSON[0]), maxItems, "bulk write")
		if err != nil {
			return nil, err
		}

		if hasImageField(container) {
			for _, field := range container.Fields {
				if field.Type != "image" {
					continue
				}

				files, exists := form.File[field.Name]
				if !exists {
					continue
				}
				if len(files) != len(items) {
					return nil, fmt.Errorf("Expected %d files for field '%s' but got %d", len(items), field.Name, len(files))
				}

				for i, file := range files {
					imageURL, err := p.uploadService.SaveAndUpload(c, file)
					if err != nil {
						return nil, err
					}
					items[i][field.Name] = imageURL
				}
			}
		}

		return items, nil
	}

	return parseLimitedItems(c.Body(), maxItems, "bulk write")
}

func (p *DynamicRequestParser) ParseUpdateItems(c *fiber.Ctx, container *models.ContainerModel, maxItems int) ([]map[string]interface{}, error) {
	if strings.Contains(c.Get("Content-Type"), "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			return nil, err
		}

		itemsJSON, exists := form.Value["items"]
		if !exists || len(itemsJSON) == 0 {
			return nil, errors.New("missing items field")
		}

		items, err := parseLimitedItems([]byte(itemsJSON[0]), maxItems, "bulk update")
		if err != nil {
			return nil, err
		}

		if hasImageField(container) {
			for _, field := range container.Fields {
				if field.Type != "image" {
					continue
				}

				files, exists := form.File[field.Name]
				if !exists {
					continue
				}
				if len(files) != len(items) {
					return nil, fmt.Errorf("Expected %d files for field '%s' but got %d", len(items), field.Name, len(files))
				}

				for i, file := range files {
					imageURL, err := p.uploadService.SaveAndUpload(c, file)
					if err != nil {
						return nil, err
					}
					items[i][field.Name] = imageURL
				}
			}
		}

		return items, nil
	}

	return parseLimitedItems(c.Body(), maxItems, "bulk update")
}

func (p *DynamicRequestParser) ParseDeleteItems(c *fiber.Ctx, maxItems int) ([]map[string]interface{}, error) {
	return parseLimitedItems(c.Body(), maxItems, "bulk delete")
}

func (p *DynamicRequestParser) ParseSearchParams(c *fiber.Ctx) (SearchParams, error) {
	sortDoc, err := utils.ParseSort(c)
	if err != nil {
		return SearchParams{}, fmt.Errorf("invalid sort params: %w", err)
	}

	pager, err := utils.ParsePager(c)
	if err != nil {
		return SearchParams{}, fmt.Errorf("invalid pagination params: %w", err)
	}

	return SearchParams{
		SearchKey: c.Query("search"),
		Sort:      sortDoc,
		Pager:     pager,
	}, nil
}

func (p *DynamicRequestParser) ParseUpdateItem(c *fiber.Ctx, container *models.ContainerModel) (map[string]interface{}, error) {
	return p.parseItem(c, container)
}

func (p *DynamicRequestParser) parseItem(c *fiber.Ctx, container *models.ContainerModel) (map[string]interface{}, error) {
	if hasImageField(container) || strings.Contains(c.Get("Content-Type"), "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			return nil, err
		}

		itemMap := utils.ProcessFormFields(form.Value)
		ConvertFormFieldTypes(itemMap, container)

		for fieldName, formFiles := range form.File {
			for _, file := range formFiles {
				imageURL, err := p.uploadService.SaveAndUpload(c, file)
				if err != nil {
					return nil, err
				}
				itemMap[fieldName] = imageURL
			}
		}

		return itemMap, nil
	}

	var itemMap map[string]interface{}
	if err := c.BodyParser(&itemMap); err != nil {
		return nil, err
	}
	if itemMap == nil {
		itemMap = make(map[string]interface{})
	}
	return itemMap, nil
}

func hasImageField(container *models.ContainerModel) bool {
	for _, field := range container.Fields {
		if field.Type == "image" {
			return true
		}
	}
	return false
}

func parseLimitedItems(data []byte, max int, operation string) ([]map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return nil, errors.New("expected JSON array")
	}

	items := make([]map[string]interface{}, 0, max)
	for decoder.More() {
		if len(items) >= max {
			return nil, &BatchLimitError{
				Operation: operation,
				Requested: max + 1,
				Max:       max,
			}
		}

		var item map[string]interface{}
		if err := decoder.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	token, err = decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != ']' {
		return nil, errors.New("expected end of JSON array")
	}

	return items, nil
}

func ConvertFormFieldTypes(itemMap map[string]interface{}, container *models.ContainerModel) {
	for _, field := range container.Fields {
		value, exists := itemMap[field.Name]
		if !exists {
			continue
		}

		strValue, isString := value.(string)
		if !isString {
			if field.Type == "array" && field.Children != nil {
				if arrayValue, isArray := value.([]interface{}); isArray {
					for _, item := range arrayValue {
						if objMap, isMap := item.(map[string]interface{}); isMap {
							convertNestedFields(objMap, field.Children)
						}
					}
				}
			}
			if field.Type == "object" && field.Children != nil {
				if objValue, isMap := value.(map[string]interface{}); isMap {
					convertNestedFields(objValue, field.Children)
				}
			}
			continue
		}

		switch field.Type {
		case "bool", "boolean":
			if strValue == "true" {
				itemMap[field.Name] = true
			} else if strValue == "false" {
				itemMap[field.Name] = false
			}
		case "int":
			if intValue, err := strconv.Atoi(strValue); err == nil {
				itemMap[field.Name] = intValue
			}
		case "float", "decimal":
			if floatValue, err := strconv.ParseFloat(strValue, 64); err == nil {
				itemMap[field.Name] = floatValue
			}
		case "stringArray":
			itemMap[field.Name] = splitStringArray(strValue)
		case "numberArray", "intArray":
			itemMap[field.Name] = splitNumberArray(strValue)
		}
	}
}

func convertNestedFields(objMap map[string]interface{}, fields []models.Field) {
	for _, field := range fields {
		value, exists := objMap[field.Name]
		if !exists {
			continue
		}

		strValue, isString := value.(string)
		if !isString {
			continue
		}

		switch field.Type {
		case "bool", "boolean":
			if strValue == "true" {
				objMap[field.Name] = true
			} else if strValue == "false" {
				objMap[field.Name] = false
			}
		case "int":
			if intValue, err := strconv.Atoi(strValue); err == nil {
				objMap[field.Name] = intValue
			}
		case "float", "decimal":
			if floatValue, err := strconv.ParseFloat(strValue, 64); err == nil {
				objMap[field.Name] = floatValue
			}
		case "stringArray":
			objMap[field.Name] = splitStringArray(strValue)
		case "numberArray", "intArray":
			objMap[field.Name] = splitNumberArray(strValue)
		}
	}
}

func splitStringArray(value string) []interface{} {
	if value == "" {
		return []interface{}{}
	}
	parts := strings.Split(value, ",")
	result := make([]interface{}, len(parts))
	for i, part := range parts {
		result[i] = strings.TrimSpace(part)
	}
	return result
}

func splitNumberArray(value string) []interface{} {
	if value == "" {
		return []interface{}{}
	}
	parts := strings.Split(value, ",")
	result := make([]interface{}, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if intValue, err := strconv.Atoi(part); err == nil {
			result = append(result, intValue)
		} else if floatValue, err := strconv.ParseFloat(part, 64); err == nil {
			result = append(result, floatValue)
		}
	}
	return result
}
