package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// convertQueryValueToFieldType converts query parameter values to the specified field type.
func ConvertQueryValueToFieldType(fieldName, fieldType, queryValue string) (interface{}, error) {
    // Check if the value contains a comma, indicating multiple values
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
            return intValues, nil
        case "string":
            var strValues []string
            for _, v := range values {
                strValues = append(strValues, strings.TrimSpace(v))
            }
            return strValues, nil
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

// TODO: Add more field types as required