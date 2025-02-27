package utils

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
)

// ValidateContainerModel validates the container model fields
func ValidateContainerModel(item map[string]interface{}, containerModel models.ContainerModel) error {
    // First, enforce that login credential fields exist.
    for _, field := range containerModel.Fields {
        if field.IsLoginCredential {
            if val, exists := item[field.Name]; !exists || fmt.Sprintf("%v", val) == "" {
                return fmt.Errorf("Field %s is required as a login credential", field.Name)
            }
        }
    }
    for _, field := range containerModel.Fields {
        if err := validateField(item, field); err != nil {
            return err
        }
    }
    return nil
}

func validateField(item map[string]interface{}, field models.Field) error {
    // Base validation
    if err := validateFieldBase(item, field.Name, field.Type, field.Tag); err != nil {
        return err
    }

    // Validate nested fields if the field is an array
    if field.Type == "array" {
        if err := validateArrayField(item, field); err != nil {
            return err
        }
    }
    return nil
}

func validateArrayField(item map[string]interface{}, field models.Field) error {
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
            if err := validateField(objMap, childField); err != nil {
                return err
            }
        }
    }
    return nil
}

func validateFieldBase(item map[string]interface{}, fieldName, fieldType, tag string) error {
    rules := extractValidationRules(tag)

    // If the field is marked as auto, skip required check.
    if auto, ok := rules["auto"].(bool); ok && auto {
        // Optionally, remove "required" so that further validation doesn’t complain.
        delete(rules, "required")
    }
    
    fieldValue := item[fieldName]

    // Check for required field
    if required, ok := rules["required"].(bool); ok && required {
        if fieldValue == nil || fmt.Sprintf("%v", fieldValue) == "" {
            if msg, ok := rules["requiredMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s is required but not provided", fieldName)
        }
    } else if fieldValue == nil || fmt.Sprintf("%v", fieldValue) == "" {
        // If the field is not required and the value is empty, return without error
        return nil
    }

    // Perform specific field type validations
    switch fieldType {
    case "objectId":
        val, ok := fieldValue.(string)
        if !ok {
            return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
        }
        if len(val) != 24 || !isValidHex(val) {
            return fmt.Errorf("Field %s should be a valid ObjectId", fieldName)
        }

    case "string":
        val, ok := fieldValue.(string)
        if !ok {
            return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
        }
        if emailRule, exists := rules["email"]; exists && emailRule == true {
            re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s should be a valid email address", fieldName)
            }
        }
        if minLength, ok := rules["minlength"].(int); ok && len(val) < minLength {
            if msg, ok := rules["minlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have a string length greater than or equal to %d", fieldName, minLength)
        }
        if maxLength, ok := rules["maxlength"].(int); ok && len(val) > maxLength {
            if msg, ok := rules["maxlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have a string length less than or equal to %d", fieldName, maxLength)
        }
	case "float", "decimal":
		var val float64
		switch v := fieldValue.(type) {
		case float64:
			val = v
		case int:
			val = float64(v)
		case string:
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("Field %s should be a valid float", fieldName)
			}
			val = parsed
		default:
			return fmt.Errorf("Field %s should be of type float/decimal", fieldName)
		}
		// Validate min and max if provided
		if min, ok := rules["min"].(int); ok && val < float64(min) {
			if msg, ok := rules["minMessage"].(string); ok && msg != "" {
				return fmt.Errorf(msg)
			}
			return fmt.Errorf("Field %s should have a value greater than or equal to %d", fieldName, min)
		}
		if max, ok := rules["max"].(int); ok && val > float64(max) {
			if msg, ok := rules["maxMessage"].(string); ok && msg != "" {
				return fmt.Errorf(msg)
			}
			return fmt.Errorf("Field %s should have a value less than or equal to %d", fieldName, max)
		}

	// UUID
	case "uuid":
		val, ok := fieldValue.(string)
		if !ok {
			return fmt.Errorf("Field %s should be a valid UUID string", fieldName)
		}
		re := regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
		if !re.MatchString(val) {
			return fmt.Errorf("Field %s should be a valid UUID", fieldName)
		}

	// URL
	case "url":
		val, ok := fieldValue.(string)
		if !ok {
			return fmt.Errorf("Field %s should be a valid URL string", fieldName)
		}
		parsedUrl, err := url.ParseRequestURI(val)
		if err != nil || parsedUrl.Scheme == "" || parsedUrl.Host == "" {
			return fmt.Errorf("Field %s should be a valid URL", fieldName)
		}

	// IP Address
	case "ip", "ipaddress":
		val, ok := fieldValue.(string)
		if !ok {
			return fmt.Errorf("Field %s should be a valid IP address string", fieldName)
		}
		if net.ParseIP(val) == nil {
			return fmt.Errorf("Field %s should be a valid IP address", fieldName)
		}

	// Enum: assumes the tag provides allowed values (e.g., enum="red|green|blue")
	case "enum":
		val, ok := fieldValue.(string)
		if !ok {
			return fmt.Errorf("Field %s should be a string for enum validation", fieldName)
		}
		allowed, ok := rules["enum"].([]string)
		if !ok {
			// Fallback if stored as []interface{}
			if list, ok := rules["enum"].([]interface{}); ok {
				var enumList []string
				for _, item := range list {
					if str, ok := item.(string); ok {
						enumList = append(enumList, str)
					}
				}
				allowed = enumList
			}
		}
		found := false
		for _, option := range allowed {
			if option == val {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Field %s must be one of the allowed values: %v", fieldName, allowed)
		}
    case "int":
        var val int
        switch v := fieldValue.(type) {
        case int:
            val = v
        case float64:
            val = int(v) // Convert float64 to int
        default:
            return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
        }
        if min, ok := rules["min"].(int); ok && val < min {
            if msg, ok := rules["minMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have a value greater than or equal to %d", fieldName, min)
        }
        if max, ok := rules["max"].(int); ok && val > max {
            if msg, ok := rules["maxMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have a value less than or equal to %d", fieldName, max)
        }
        
    case "bool", "boolean":
    if _, ok := fieldValue.(bool); !ok {
        return fmt.Errorf("Field %s should be of type boolean", fieldName)
    }
   case "date":
    var dateVal time.Time
    var err error

    switch v := fieldValue.(type) {
    case string:
        dateVal, err = time.Parse(time.RFC3339, v)
        if err != nil {
            dateVal, err = time.Parse("2006-01-02", v) // Try without timestamp
        }
        if err != nil {
            return fmt.Errorf("Field %s should be a valid date in RFC3339 (e.g., 2023-01-01T00:00:00Z) or YYYY-MM-DD format", fieldName)
        }

    case int64:
        dateVal = time.Unix(v, 0)

    case int, float64:
        unixTimestamp := int64(v.(int))
        dateVal = time.Unix(unixTimestamp, 0)

    default:
        return fmt.Errorf("Field %s should be a valid date (RFC3339 string, YYYY-MM-DD, or Unix timestamp)", fieldName)
    }

    dateVal = time.Date(dateVal.Year(), dateVal.Month(), dateVal.Day(), 0, 0, 0, 0, time.UTC)

    // Validate minDate and maxDate
    if err := validateDateConstraints(fieldName, dateVal, rules); err != nil {
        return err
    }
    }
    // Check for required field
    if required, ok := rules["required"].(bool); ok && required && (fieldValue == nil || fmt.Sprintf("%v", fieldValue) == "") {
        if msg, ok := rules["requiredMessage"].(string); ok && msg != "" {
            return fmt.Errorf(msg)
        }
        return fmt.Errorf("Field %s is required but not provided", fieldName)
    }

    return nil
}


// extractValidationRules parses the tag and extracts validation rules and custom messages
func extractValidationRules(tag string) map[string]interface{} {
	rules := make(map[string]interface{})

	// Split the tag into parts to extract individual rules
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "minlength=") {
			minLength, message := extractRuleAndMessage(part, "minlength", "minlengthMessage")
			rules["minlength"] = minLength
			rules["minlengthMessage"] = message
		}
		if strings.Contains(part, "maxlength=") {
			maxLength, message := extractRuleAndMessage(part, "maxlength", "maxlengthMessage")
			rules["maxlength"] = maxLength
			rules["maxlengthMessage"] = message
		}
        if strings.Contains(part, "enum=") {
            enumStr := strings.SplitN(part, "enum=", 2)[1]
            // Remove backslashes so that \" becomes "
            enumStr = strings.ReplaceAll(enumStr, "\\", "")
            // Then trim the surrounding quotes
            enumStr = strings.Trim(enumStr, "\"")
            allowed := strings.Split(enumStr, "|")
            rules["enum"] = allowed
            fmt.Printf("DEBUG: Allowed enum values: %v\n", allowed)
        }
		if strings.Contains(part, "min=") {
			min, message := extractRuleAndMessage(part, "min", "minMessage")
			rules["min"] = min
			rules["minMessage"] = message
		}
		if strings.Contains(part, "max=") {
			max, message := extractRuleAndMessage(part, "max", "maxMessage")
			rules["max"] = max
			rules["maxMessage"] = message
		}
		if strings.Contains(part, "required") {
			rules["required"] = true
			// Extract custom message for required, if provided
			_, message := extractRuleAndMessage(part, "required", "requiredMessage")
			rules["requiredMessage"] = message
		}
        if strings.Contains(part, "email") {
			rules["email"] = true
		}
        if strings.Contains(part, "auto") {
            rules["auto"] = true
        }
	}
	return rules
}

// extractRuleAndMessage extracts a validation rule and its custom message from a part of the tag
func extractRuleAndMessage(part, ruleKey, messageKey string) (int, string) {
    var ruleValue int
    var message string

    // Extract rule value
    if start := strings.Index(part, ruleKey+"="); start != -1 {
        ruleStr := part[start+len(ruleKey+"="):]
        if end := strings.Index(ruleStr, "\""); end != -1 {
            ruleStr = ruleStr[:end]
            ruleValue, _ = strconv.Atoi(ruleStr) // Error ignored as it is handled in the validation logic
        }
    }

    // Extract custom message
    if start := strings.Index(part, messageKey+"="); start != -1 {
        messageStr := part[start+len(messageKey+"="):]
        if end := strings.Index(messageStr, "\""); end != -1 {
            message = messageStr[:end]
        }
    }

    return ruleValue, message
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

// validateDateConstraints checks if a date value falls within the defined min/max range
func validateDateConstraints(fieldName string, dateVal time.Time, rules map[string]interface{}) error {
    if minDateStr, ok := rules["minDate"].(string); ok {
        minDate, err := time.Parse("2006-01-02", minDateStr)
        if err != nil {
            return fmt.Errorf("Invalid minDate format for field %s, should be YYYY-MM-DD", fieldName)
        }
        if dateVal.Before(minDate) {
            if msg, ok := rules["minDateMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should not be earlier than %s", fieldName, minDateStr)
        }
    }

    if maxDateStr, ok := rules["maxDate"].(string); ok {
        maxDate, err := time.Parse("2006-01-02", maxDateStr)
        if err != nil {
            return fmt.Errorf("Invalid maxDate format for field %s, should be YYYY-MM-DD", fieldName)
        }
        if dateVal.After(maxDate) {
            if msg, ok := rules["maxDateMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should not be later than %s", fieldName, maxDateStr)
        }
    }

    return nil
}
