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

// ValidatePartialUpdate validates only the fields that are being updated (partial validation for updates)
func ValidatePartialUpdate(updatedFields map[string]interface{}, containerModel models.ContainerModel) error {
    // First, enforce that login credential fields exist if they are being updated
    for _, field := range containerModel.Fields {
        if field.IsLoginCredential {
            if val, exists := updatedFields[field.Name]; exists {
                if fmt.Sprintf("%v", val) == "" {
                    return fmt.Errorf("Field %s is required as a login credential", field.Name)
                }
            }
        }
    }
    
    // Only validate fields that are present in updatedFields
    for _, field := range containerModel.Fields {
        // Check if this field is in the update map
        if _, exists := updatedFields[field.Name]; exists {
            if err := validateField(updatedFields, field); err != nil {
                return err
            }
        }
    }
    return nil
}

func validateField(item map[string]interface{}, field models.Field) error {
    // Base validation
    if err := validateFieldBase(item, field); err != nil {
        return err
    }

    // Validate nested fields if the field is an array
    if field.Type == "array" {
        if err := validateArrayField(item, field); err != nil {
            return err
        }
    }
    
    // Validate nested fields if the field is an object
    if field.Type == "object" {
        if err := validateObjectField(item, field); err != nil {
            return err
        }
    }
    
    // Validate enum list for stringArray and numberArray/intArray
    if (field.Type == "stringArray" || field.Type == "numberArray" || field.Type == "intArray") && len(field.EnumList) > 0 {
        if err := validateArrayEnumList(item, field); err != nil {
            return err
        }
    }
    
    // Validate enum list for regular string, int, float/decimal types
    if (field.Type == "string" || field.Type == "int" || field.Type == "float" || field.Type == "decimal") && len(field.EnumList) > 0 {
        if err := validateEnumList(item, field); err != nil {
            return err
        }
    }
    
    return nil
}

func validateObjectField(item map[string]interface{}, field models.Field) error {
    objValue, ok := item[field.Name].(map[string]interface{})
    if !ok {
        return fmt.Errorf("Field %s should be of type object", field.Name)
    }

    // Validate each child field within the object
    for _, childField := range field.Children {
        if err := validateField(objValue, childField); err != nil {
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

func validateArrayEnumList(item map[string]interface{}, field models.Field) error {
    arrayValue, ok := item[field.Name].([]interface{})
    if !ok {
        // Field is not an array, skip enum validation
        return nil
    }

    // Check if enumList is provided
    if len(field.EnumList) == 0 {
        return nil
    }

    // Validate each element in the array against the enum list
    for i, element := range arrayValue {
        found := false
        
        // For stringArray, compare as strings
        if field.Type == "stringArray" {
            elementStr, ok := element.(string)
            if !ok {
                return fmt.Errorf("Field %s: element at index %d is not a string", field.Name, i)
            }
            
            for _, allowedValue := range field.EnumList {
                if allowedStr, ok := allowedValue.(string); ok && allowedStr == elementStr {
                    found = true
                    break
                }
            }
        } else if field.Type == "numberArray" || field.Type == "intArray" {
            // For numberArray/intArray, compare as numbers
            var elementNum float64
            switch v := element.(type) {
            case int:
                elementNum = float64(v)
            case float64:
                elementNum = v
            default:
                return fmt.Errorf("Field %s: element at index %d is not a number", field.Name, i)
            }
            
            for _, allowedValue := range field.EnumList {
                var allowedNum float64
                switch v := allowedValue.(type) {
                case int:
                    allowedNum = float64(v)
                case float64:
                    allowedNum = v
                default:
                    continue
                }
                
                if allowedNum == elementNum {
                    found = true
                    break
                }
            }
        }
        
        if !found {
            return fmt.Errorf("Field %s: element at index %d (%v) is not in the allowed enum list: %v", field.Name, i, element, field.EnumList)
        }
    }
    
    return nil
}

func validateEnumList(item map[string]interface{}, field models.Field) error {
    fieldValue := item[field.Name]
    
    // Skip if field value is nil or empty
    if fieldValue == nil || fmt.Sprintf("%v", fieldValue) == "" {
        return nil
    }
    
    // Check if enumList is provided
    if len(field.EnumList) == 0 {
        return nil
    }
    
    found := false
    
    // For string type, compare as strings
    if field.Type == "string" {
        valueStr, ok := fieldValue.(string)
        if !ok {
            return fmt.Errorf("Field %s should be a string", field.Name)
        }
        
        for _, allowedValue := range field.EnumList {
            if allowedStr, ok := allowedValue.(string); ok && allowedStr == valueStr {
                found = true
                break
            }
        }
        
        if !found {
            return fmt.Errorf("Field %s value (%v) is not in the allowed enum list: %v", field.Name, valueStr, field.EnumList)
        }
    } else if field.Type == "int" || field.Type == "float" || field.Type == "decimal" {
        // For numeric types, compare as numbers
        var valueNum float64
        switch v := fieldValue.(type) {
        case int:
            valueNum = float64(v)
        case float64:
            valueNum = v
        default:
            return fmt.Errorf("Field %s should be a number", field.Name)
        }
        
        for _, allowedValue := range field.EnumList {
            var allowedNum float64
            switch v := allowedValue.(type) {
            case int:
                allowedNum = float64(v)
            case float64:
                allowedNum = v
            default:
                continue
            }
            
            if allowedNum == valueNum {
                found = true
                break
            }
        }
        
        if !found {
            return fmt.Errorf("Field %s value (%v) is not in the allowed enum list: %v", field.Name, valueNum, field.EnumList)
        }
    }
    
    return nil
}

func validateFieldBase(item map[string]interface{}, field models.Field) error {
    fieldName := field.Name
    fieldType := field.Type
    tag := field.Tag
    
    rules := extractValidationRules(tag)

    // If the field is marked as auto, skip required check.
    if auto, ok := rules["auto"].(bool); ok && auto {
        // Optionally, remove "required" so that further validation doesn't complain.
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
    case "object":
        // Object type validation - just check if it's a map
        _, ok := fieldValue.(map[string]interface{})
        if !ok {
            return fmt.Errorf("Field %s should be of type object", fieldName)
        }
        // Nested validation is handled in validateObjectField
    case "array":
        // Array type validation - just check if it's a slice
        _, ok := fieldValue.([]interface{})
        if !ok {
            return fmt.Errorf("Field %s should be of type array", fieldName)
        }
        // Nested validation is handled in validateArrayField
    case "objectId":
        val, ok := fieldValue.(string)
        if !ok {
            return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
        }
        if len(val) != 24 || !isValidHex(val) {
            return fmt.Errorf("Field %s should be a valid ObjectId", fieldName)
        }
    case "objectIdArray":
        arrayValue, ok := fieldValue.([]interface{})
        if !ok {
            return fmt.Errorf("Field %s should be an array", fieldName)
        }
        for i, item := range arrayValue {
            val, ok := item.(string)
            if !ok {
                return fmt.Errorf("Field %s: element at index %d should be a string ObjectId", fieldName, i)
            }
            if len(val) != 24 || !isValidHex(val) {
                return fmt.Errorf("Field %s: element at index %d should be a valid ObjectId", fieldName, i)
            }
        }
        // Validate array length constraints
        if minLength, ok := rules["minlength"].(int); ok && len(arrayValue) < minLength {
            if msg, ok := rules["minlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have at least %d elements", fieldName, minLength)
        }
        if maxLength, ok := rules["maxlength"].(int); ok && len(arrayValue) > maxLength {
            if msg, ok := rules["maxlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have at most %d elements", fieldName, maxLength)
        }
    case "autoIncrementId":
        // If provided, the value should be an integer (or a numeric string that can be parsed as int)
        switch v := fieldValue.(type) {
        case int:
            // valid
        case float64:
            if v != float64(int(v)) {
                return fmt.Errorf("Field %s should be an integer", fieldName)
            }
        case string:
            if _, err := strconv.Atoi(v); err != nil {
                return fmt.Errorf("Field %s should be an integer", fieldName)
            }
        default:
            return fmt.Errorf("Field %s should be of type autoIncrementId (integer)", fieldName)
        }
    case "string":
        val, ok := fieldValue.(string)
        if !ok {
            return fmt.Errorf("Field %s should be of type %s", fieldName, fieldType)
        }
        
        // Email validation
        if emailRule, exists := rules["email"]; exists && emailRule == true {
            re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s should be a valid email address", fieldName)
            }
        }
        
        // Phone validation (international format)
        if phoneRule, exists := rules["phone"]; exists && phoneRule == true {
            // Matches formats like: +1234567890, +1-234-567-8900, (123) 456-7890, etc.
            re := regexp.MustCompile(`^[\+]?[(]?[0-9]{1,4}[)]?[-\s\.]?[(]?[0-9]{1,4}[)]?[-\s\.]?[0-9]{1,9}$`)
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s should be a valid phone number", fieldName)
            }
        }
        
        // URL validation
        if urlRule, exists := rules["url"]; exists && urlRule == true {
            parsedUrl, err := url.ParseRequestURI(val)
            if err != nil || parsedUrl.Scheme == "" || parsedUrl.Host == "" {
                return fmt.Errorf("Field %s should be a valid URL", fieldName)
            }
        }
        
        // Credit card validation (Luhn algorithm)
        if ccRule, exists := rules["creditcard"]; exists && ccRule == true {
            // Remove spaces and dashes
            cleaned := strings.ReplaceAll(strings.ReplaceAll(val, " ", ""), "-", "")
            if !isValidCreditCard(cleaned) {
                return fmt.Errorf("Field %s should be a valid credit card number", fieldName)
            }
        }
        
        // Alphanumeric validation
        if alphanumRule, exists := rules["alphanumeric"]; exists && alphanumRule == true {
            re := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s should contain only alphanumeric characters", fieldName)
            }
        }
        
        // Alpha validation (letters only)
        if alphaRule, exists := rules["alpha"]; exists && alphaRule == true {
            re := regexp.MustCompile(`^[a-zA-Z]+$`)
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s should contain only alphabetic characters", fieldName)
            }
        }
        
        // Numeric validation (digits only)
        if numericRule, exists := rules["numeric"]; exists && numericRule == true {
            re := regexp.MustCompile(`^[0-9]+$`)
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s should contain only numeric characters", fieldName)
            }
        }
        
        // Lowercase validation
        if lowercaseRule, exists := rules["lowercase"]; exists && lowercaseRule == true {
            if val != strings.ToLower(val) {
                return fmt.Errorf("Field %s should be in lowercase", fieldName)
            }
        }
        
        // Uppercase validation
        if uppercaseRule, exists := rules["uppercase"]; exists && uppercaseRule == true {
            if val != strings.ToUpper(val) {
                return fmt.Errorf("Field %s should be in uppercase", fieldName)
            }
        }
        
        // Pattern (regex) validation
        if pattern, exists := rules["pattern"].(string); exists && pattern != "" {
            re, err := regexp.Compile(pattern)
            if err != nil {
                return fmt.Errorf("Invalid regex pattern for field %s", fieldName)
            }
            if !re.MatchString(val) {
                return fmt.Errorf("Field %s does not match the required pattern", fieldName)
            }
        }
        
        // Length constraints
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
		
		// Positive validation
		if positiveRule, exists := rules["positive"]; exists && positiveRule == true {
			if val <= 0 {
				return fmt.Errorf("Field %s should be a positive number", fieldName)
			}
		}
		
		// Negative validation
		if negativeRule, exists := rules["negative"]; exists && negativeRule == true {
			if val >= 0 {
				return fmt.Errorf("Field %s should be a negative number", fieldName)
			}
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
        
        // Positive validation
        if positiveRule, exists := rules["positive"]; exists && positiveRule == true {
            if val <= 0 {
                return fmt.Errorf("Field %s should be a positive number", fieldName)
            }
        }
        
        // Negative validation
        if negativeRule, exists := rules["negative"]; exists && negativeRule == true {
            if val >= 0 {
                return fmt.Errorf("Field %s should be a negative number", fieldName)
            }
        }
        
        // Min/Max constraints
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
    case "stringArray":
        arrayValue, ok := fieldValue.([]interface{})
        if !ok {
            return fmt.Errorf("Field %s should be an array", fieldName)
        }
        for i, item := range arrayValue {
            if _, ok := item.(string); !ok {
                return fmt.Errorf("Field %s: element at index %d should be a string", fieldName, i)
            }
        }
        // Validate array length constraints
        if minLength, ok := rules["minlength"].(int); ok && len(arrayValue) < minLength {
            if msg, ok := rules["minlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have at least %d elements", fieldName, minLength)
        }
        if maxLength, ok := rules["maxlength"].(int); ok && len(arrayValue) > maxLength {
            if msg, ok := rules["maxlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have at most %d elements", fieldName, maxLength)
        }
    case "numberArray", "intArray":
        arrayValue, ok := fieldValue.([]interface{})
        if !ok {
            return fmt.Errorf("Field %s should be an array", fieldName)
        }
        for i, item := range arrayValue {
            switch item.(type) {
            case int, float64:
                // Valid number type
            default:
                return fmt.Errorf("Field %s: element at index %d should be a number", fieldName, i)
            }
        }
        // Validate array length constraints
        if minLength, ok := rules["minlength"].(int); ok && len(arrayValue) < minLength {
            if msg, ok := rules["minlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have at least %d elements", fieldName, minLength)
        }
        if maxLength, ok := rules["maxlength"].(int); ok && len(arrayValue) > maxLength {
            if msg, ok := rules["maxlengthMessage"].(string); ok && msg != "" {
                return fmt.Errorf(msg)
            }
            return fmt.Errorf("Field %s should have at most %d elements", fieldName, maxLength)
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
		
		// Length constraints
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
		
		// Enum validation
        if strings.Contains(part, "enum=") {
            enumStr := strings.SplitN(part, "enum=", 2)[1]
            enumStr = strings.ReplaceAll(enumStr, "\\", "")
            enumStr = strings.Trim(enumStr, "\"")
            allowed := strings.Split(enumStr, "|")
            rules["enum"] = allowed
        }
        
        // Numeric constraints
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
		
		// Date constraints
		if strings.Contains(part, "minDate=") {
			if start := strings.Index(part, "minDate="); start != -1 {
				dateStr := part[start+len("minDate="):]
				if end := strings.Index(dateStr, "\""); end != -1 {
					dateStr = dateStr[:end]
				}
				rules["minDate"] = dateStr
			}
		}
		if strings.Contains(part, "maxDate=") {
			if start := strings.Index(part, "maxDate="); start != -1 {
				dateStr := part[start+len("maxDate="):]
				if end := strings.Index(dateStr, "\""); end != -1 {
					dateStr = dateStr[:end]
				}
				rules["maxDate"] = dateStr
			}
		}
		
		// Required field
		if strings.Contains(part, "required") {
			rules["required"] = true
			_, message := extractRuleAndMessage(part, "required", "requiredMessage")
			rules["requiredMessage"] = message
		}
		
		// Format validations
        if strings.Contains(part, "email") {
			rules["email"] = true
		}
		if strings.Contains(part, "phone") {
			rules["phone"] = true
		}
		if strings.Contains(part, "url") {
			rules["url"] = true
		}
		if strings.Contains(part, "creditcard") {
			rules["creditcard"] = true
		}
		
		// String format constraints
		if strings.Contains(part, "alphanumeric") {
			rules["alphanumeric"] = true
		}
		if strings.Contains(part, "alpha") {
			rules["alpha"] = true
		}
		if strings.Contains(part, "numeric") {
			rules["numeric"] = true
		}
		if strings.Contains(part, "lowercase") {
			rules["lowercase"] = true
		}
		if strings.Contains(part, "uppercase") {
			rules["uppercase"] = true
		}
		
		// Numeric sign constraints
		if strings.Contains(part, "positive") {
			rules["positive"] = true
		}
		if strings.Contains(part, "negative") {
			rules["negative"] = true
		}
		
		// Pattern (regex) validation
		if strings.Contains(part, "pattern=") {
			if start := strings.Index(part, "pattern="); start != -1 {
				patternStr := part[start+len("pattern="):]
				if end := strings.Index(patternStr, "\""); end != -1 {
					patternStr = patternStr[:end]
				}
				rules["pattern"] = patternStr
			}
		}
		
		// Unique constraint
		if strings.Contains(part, "unique") {
			rules["unique"] = true
		}
		
		// Auto-increment
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
        // Remove surrounding quotes if present
        ruleStr = strings.Trim(ruleStr, "\"")
        // Find the end of the value (either comma or end of string)
        if end := strings.Index(ruleStr, ","); end != -1 {
            ruleStr = ruleStr[:end]
        }
        ruleStr = strings.TrimSpace(ruleStr)
        ruleValue, _ = strconv.Atoi(ruleStr) // Error ignored as it is handled in the validation logic
    }

    // Extract custom message
    if start := strings.Index(part, messageKey+"="); start != -1 {
        messageStr := part[start+len(messageKey+"="):]
        // Remove surrounding quotes if present
        messageStr = strings.Trim(messageStr, "\"")
        // Find the end of the message (either comma or end of string)
        if end := strings.Index(messageStr, ","); end != -1 {
            messageStr = messageStr[:end]
        }
        message = strings.TrimSpace(messageStr)
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
