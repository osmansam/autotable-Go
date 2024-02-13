package utils

import (
	"fmt"
	"strconv"
)

// convertQueryValueToFieldType converts query parameter values to the specified field type.
func ConvertQueryValueToFieldType(fieldName, fieldType, queryValue string) (interface{}, error) {
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