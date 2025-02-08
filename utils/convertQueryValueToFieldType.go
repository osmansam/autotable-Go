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
		"eq-":   "$eq", // Exact match
	}

	// Handle date filters with comparison operators
	if fieldType == "date" {
		filter := bson.M{}
		conditionsFound := false

		// Apply range and exact match operators
		for prefix, mongoOp := range operators {
			if strings.HasPrefix(queryValue, prefix) {
				dateStr := strings.TrimPrefix(queryValue, prefix)
				filter[mongoOp] = dateStr
				conditionsFound = true
			}
		}

		// If an operator (`gte-`, `lt-`, etc.) was found, return the filter
		if conditionsFound {
			return filter, nil
		}

		// If no operator was found, return an error (user must provide one)
		return nil, fmt.Errorf("invalid date filter for field %s, please use eq-, gte-, gt-, lte-, or lt-", fieldName)
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
		default:
			return nil, fmt.Errorf("unsupported field type %s for field %s", fieldType, fieldName)
		}
	}

	// Handle single values
	switch fieldType {
	case "string":
		return queryValue, nil
	case "int":
		intValue, err := strconv.Atoi(queryValue)
		if err != nil {
			return nil, fmt.Errorf("invalid integer value for field %s: %w", fieldName, err)
		}
		return intValue, nil
	case "bool":
		boolValue, err := strconv.ParseBool(queryValue)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean value for field %s: %w", fieldName, err)
		}
		return boolValue, nil
	default:
		return nil, fmt.Errorf("unsupported field type %s for field %s", fieldType, fieldName)
	}
}

// parseDate ensures date is parsed correctly and normalized to UTC
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
