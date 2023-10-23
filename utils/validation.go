package utils

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/osmansam/autotableGo/models"
)

func ValidateContainerModel(item map[string]interface{}, containerModel models.ContainerModel) error {
	for _, field := range containerModel.Fields {
		err := validateField(item, field)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateField(item map[string]interface{}, field models.Field) error {
	err := validateFieldBase(item, field.Name, field.Type, field.Tag)
	if err != nil {
		return err
	}

	if field.Type == "array" {
		arrayItems, ok := item[field.Name].([]interface{})
		if !ok {
			return fmt.Errorf("Field %s should be of type array", field.Name)
		}
		for _, obj := range arrayItems {
			objMap, ok := obj.(map[string]interface{})
			if !ok {
				return fmt.Errorf("Element in array %s is not a valid map", field.Name)
			}
			for _, childField := range field.Children {
				err := validateField(objMap, childField)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateFieldBase(item map[string]interface{}, fieldName string, fieldType string, tag string) error {
	fieldValue, exists := item[fieldName]
	if !exists {
		return fmt.Errorf("Field %s is missing", fieldName)
	}

	switch fieldType {
	case "objectId":
	val, ok := fieldValue.(string)
	if !ok {
		return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
	}
	// Check if the ObjectId string has the correct length and is a valid hexadecimal string
	if len(val) != 24 || !isValidHex(val) {
		return fmt.Errorf("Field %s should be a valid ObjectId", fieldName)
	}

	case "string":
		val, ok := fieldValue.(string)
		if !ok {
			return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
		}
		if strings.Contains(tag, "minlength=") {
			minLenStr := strings.Split(strings.Split(tag, "minlength=")[1], ",")[0]
			minLen, err := strconv.Atoi(minLenStr)
			if err != nil {
				return fmt.Errorf("Invalid minlength value specified for field %s", fieldName)
			}
			if len(val) < minLen {
				return fmt.Errorf("Field %s should have a string length greater than or equal to %d", fieldName, minLen)
			}
		}
		if strings.Contains(tag, "maxlength=") {
			maxLenStr := strings.Split(strings.Split(tag, "maxlength=")[1], ",")[0]
			maxLen, err := strconv.Atoi(maxLenStr)
			if err != nil {
				return fmt.Errorf("Invalid maxlength value specified for field %s", fieldName)
			}
			if len(val) > maxLen {
				return fmt.Errorf("Field %s should have a string length less than or equal to %d", fieldName, maxLen)
			}
		}

	case "int":
		   var val int
    switch v := fieldValue.(type) {
    case int:
        val = v
    case float64:
        val = int(v)  // Convert float64 to int
    default:
        log.Println("Field value:", fieldValue)
        return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
    }
		if strings.Contains(tag, "min=") {
			minStr := strings.Split(strings.Split(tag, "min=")[1], "\"")[0]

			min, err := strconv.Atoi(minStr)
			if err != nil {
				return fmt.Errorf("Invalid min value specified for field %s", fieldName)
			}
			if val < min {
				return fmt.Errorf("Field %s should have a value greater than or equal to %d", fieldName, min)
			}
		}
		if strings.Contains(tag, "max=") {
			maxStr := strings.Split(strings.Split(tag, "max=")[1], "\"")[0]

			max, err := strconv.Atoi(maxStr)
			if err != nil {
				return fmt.Errorf("Invalid max value specified for field %s", fieldName)
			}
			if val > max {
				return fmt.Errorf("Field %s should have a value less than or equal to %d", fieldName, max)
			}
		}
	}

	if strings.Contains(tag, "validate:\"required\"") && (fieldValue == nil || fmt.Sprintf("%v", fieldValue) == "") {
		return fmt.Errorf("Field %s is required but not provided", fieldName)
	}

	return nil
}

// isValidHex checks if the given string is a valid hexadecimal value
func isValidHex(s string) bool {
	for _, c := range s {
		if !('0' <= c && c <= '9') && !('a' <= c && c <= 'f') && !('A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}
