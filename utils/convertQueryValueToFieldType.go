package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// ConvertQueryValueToFieldType converts query parameter values to the appropriate field type.
func ConvertQueryValueToFieldType(fieldName, fieldType, queryValue string) (interface{}, error) {
	// Supported MongoDB operators
	operators := map[string]string{
		"gte-": "$gte",
		"gt-":  "$gt",
		"lte-": "$lte",
		"lt-":  "$lt",
		"eq-":  "$eq", // Exact match
	}


    // Handle date filters with comparison operators or as exact match
    if fieldType == "date" {
        filter := bson.M{}
        conditionsFound := false

        // Apply range and exact match operators for dates if operator is provided
        for prefix, mongoOp := range operators {
            if strings.HasPrefix(queryValue, prefix) {
                dateStr := strings.TrimPrefix(queryValue, prefix)
                parsedDate, err := parseDate(dateStr)
                if err != nil {
                    return nil, fmt.Errorf("invalid date format for field %s: %w", fieldName, err)
                }
                filter[mongoOp] = parsedDate
                conditionsFound = true
            }
        }

        // If no operator was found, try to parse the plain date string as an exact match
        if !conditionsFound {
            parsedDate, err := parseDate(queryValue)
            if err != nil {
                return nil, fmt.Errorf("invalid date format for field %s: %w", fieldName, err)
            }
            filter["$eq"] = parsedDate
        }
        return filter, nil
    }


	// New block: Handle integer filters with comparison operators
	if fieldType == "int" {
		filter := bson.M{}
		conditionsFound := false

		// Apply range and exact match operators for integers
		for prefix, mongoOp := range operators {
			if strings.HasPrefix(queryValue, prefix) {
				intStr := strings.TrimPrefix(queryValue, prefix)
				intValue, err := strconv.Atoi(intStr)
				if err != nil {
					return nil, fmt.Errorf("invalid integer filter for field %s: %w", fieldName, err)
				}
				filter[mongoOp] = intValue
				conditionsFound = true
			}
		}

		// If an operator was found, return the filter
		if conditionsFound {
			return filter, nil
		}
	}

	// Handle multiple values (comma-separated lists) - Only for non-date types
	if strings.Contains(queryValue, ",") {
		values := strings.Split(queryValue, ",")
		switch fieldType {
		case "int":
			var intValues []int
			for _, v := range values {
				intValue, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return nil, fmt.Errorf("invalid integer value in list for field %s: %w", fieldName, err)
				}
				intValues = append(intValues, intValue)
			}
			return bson.M{"$in": intValues}, nil
		case "string":
			var strValues []string
			for _, v := range values {
				strValues = append(strValues, strings.TrimSpace(v))
			}
			return bson.M{"$in": strValues}, nil
		case "bool", "boolean":
			var boolValues []bool
			for _, v := range values {
				boolValue, err := strconv.ParseBool(strings.TrimSpace(v))
				if err != nil {
					return nil, fmt.Errorf("invalid boolean value in list for field %s: %w", fieldName, err)
				}
				boolValues = append(boolValues, boolValue)
			}
			return bson.M{"$in": boolValues}, nil
		default:
			return nil, fmt.Errorf("unsupported field type %s for field %s", fieldType, fieldName)
		}
	}

	// Handle single values (when no comma or operator is provided)
	switch fieldType {
	case "string":
		return queryValue, nil
	case "int":
		intValue, err := strconv.Atoi(queryValue)
		if err != nil {
			return nil, fmt.Errorf("invalid integer value for field %s: %w", fieldName, err)
		}
		return intValue, nil
	case "bool", "boolean":
		boolValue, err := strconv.ParseBool(queryValue)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean value for field %s: %w", fieldName, err)
		}
		return boolValue, nil
	case "stringArray":
		// For string arrays, support $in queries with comma-separated values
		if strings.Contains(queryValue, ",") {
			values := strings.Split(queryValue, ",")
			var strValues []string
			for _, v := range values {
				strValues = append(strValues, strings.TrimSpace(v))
			}
			// Use $in to match documents where the array contains any of these values
			return bson.M{"$in": strValues}, nil
		}
		// Single value - match documents where array contains this value
		return queryValue, nil
	case "numberArray", "intArray":
		// For number arrays, support $in queries with comma-separated values
		if strings.Contains(queryValue, ",") {
			values := strings.Split(queryValue, ",")
			var numValues []interface{}
			for _, v := range values {
				v = strings.TrimSpace(v)
				// Try int first, then float
				if intValue, err := strconv.Atoi(v); err == nil {
					numValues = append(numValues, intValue)
				} else if floatValue, err := strconv.ParseFloat(v, 64); err == nil {
					numValues = append(numValues, floatValue)
				} else {
					return nil, fmt.Errorf("invalid number value in list for field %s: %w", fieldName, err)
				}
			}
			return bson.M{"$in": numValues}, nil
		}
		// Single value - try to parse as int or float
		if intValue, err := strconv.Atoi(queryValue); err == nil {
			return intValue, nil
		} else if floatValue, err := strconv.ParseFloat(queryValue, 64); err == nil {
			return floatValue, nil
		}
		return nil, fmt.Errorf("invalid number value for field %s", fieldName)
	default:
		return nil, fmt.Errorf("unsupported field type %s for field %s", fieldType, fieldName)
	}
}
// parseDate ensures the date is parsed correctly and normalized to UTC
func parseDate(dateStr string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}

	var parsedDate time.Time
	var err error

	for _, format := range formats {
		parsedDate, err = time.Parse(format, dateStr)
		if err == nil {
			// Normalize time to midnight UTC
			return time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date format: %s", dateStr)
}
