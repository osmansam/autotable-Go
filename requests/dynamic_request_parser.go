package requests

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/files"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

type DynamicRequestParser struct {
	uploadService *files.UploadService
}

func NewDynamicRequestParser(uploadService *files.UploadService) *DynamicRequestParser {
	return &DynamicRequestParser{uploadService: uploadService}
}

func (p *DynamicRequestParser) ParseCreateItem(c *fiber.Ctx, container *models.ContainerModel) (map[string]interface{}, error) {
	return p.parseItem(c, container)
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
