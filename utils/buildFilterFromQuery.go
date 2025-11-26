// utils/helpers.go
package utils

import (
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func BuildFilterFromQuery(c *fiber.Ctx, container *models.ContainerModel) (bson.M, error) {
    filter := bson.M{}

    for _, f := range container.Fields {
        // skip hashed
        if f.IsHashed {
            continue
        }

        // Get all values for this field name (handles multiple query params like age=gte-32&age=lt-120)
        rawValues := c.Query(f.Name)
        if rawValues == "" {
            continue
        }

        // Check if there are multiple values by getting the request query string
        // and parsing it manually to find all instances of this field
        queryString := string(c.Request().URI().QueryString())
        allValues := []string{}
        
        // Simple approach: split by & and find all matching field names
        for _, param := range splitQuery(queryString) {
            if len(param) > len(f.Name) && param[:len(f.Name)] == f.Name && param[len(f.Name)] == '=' {
                value := param[len(f.Name)+1:]
                // URL-decode the value to handle special characters like @ in emails
                if decodedValue, err := url.QueryUnescape(value); err == nil {
                    value = decodedValue
                }
                allValues = append(allValues, value)
            }
        }

        // If no values found (shouldn't happen), use the single value
        if len(allValues) == 0 {
            allValues = []string{rawValues}
        }

        // Process each value and merge conditions
        fieldFilter := bson.M{}
        var hasGte, hasGt, hasLte, hasLt bool
        var gteVal, gtVal, lteVal, ltVal interface{}
        var simpleValues []interface{} // For collecting multiple simple values
        
        for _, raw := range allValues {
            converted, err := ConvertQueryValueToFieldType(f.Name, f.Type, raw)
            if err != nil {
                return nil, err
            }

            // if it's a map (e.g. {"$gte":…}), collect conditions
            if m, ok := converted.(bson.M); ok {
                for k, v := range m {
                    fieldFilter[k] = v
                    // Track which operators we have
                    switch k {
                    case "$gte":
                        hasGte = true
                        gteVal = v
                    case "$gt":
                        hasGt = true
                        gtVal = v
                    case "$lte":
                        hasLte = true
                        lteVal = v
                    case "$lt":
                        hasLt = true
                        ltVal = v
                    case "$eq":
                        // Single equality from operator
                        simpleValues = append(simpleValues, v)
                    }
                }
            } else {
                // Simple value without operator (e.g., age=32)
                simpleValues = append(simpleValues, converted)
            }
        }

        // Handle multiple simple values (age=32&age=123 -> {age: {$in: [32, 123]}})
        if len(simpleValues) > 1 {
            filter[f.Name] = bson.M{"$in": simpleValues}
            continue
        } else if len(simpleValues) == 1 {
            // Single simple value
            filter[f.Name] = simpleValues[0]
            continue
        }

        // Only add to filter if we have conditions
        if len(fieldFilter) > 0 {
            // Check if we need AND or OR logic
            needsOr := false
            
            // Check for contradictory conditions (needs OR)
            if f.Type == "int" || f.Type == "float" || f.Type == "decimal" {
                // Compare numeric values to detect contradictions
                if (hasGte || hasGt) && (hasLte || hasLt) {
                    var minVal, maxVal float64
                    
                    // Get minimum value
                    if hasGte {
                        minVal = toFloat64(gteVal)
                    } else if hasGt {
                        minVal = toFloat64(gtVal) + 0.0001 // gt is exclusive
                    }
                    
                    // Get maximum value
                    if hasLte {
                        maxVal = toFloat64(lteVal)
                    } else if hasLt {
                        maxVal = toFloat64(ltVal) - 0.0001 // lt is exclusive
                    }
                    
                    // If min > max, conditions are contradictory (no overlap)
                    if minVal > maxVal {
                        needsOr = true
                    }
                }
            }
            
            if needsOr {
                // Create OR conditions: value <= X OR value > Y
                orConditions := []bson.M{}
                if hasLte || hasLt {
                    cond := bson.M{}
                    if hasLte {
                        cond["$lte"] = lteVal
                    }
                    if hasLt {
                        cond["$lt"] = ltVal
                    }
                    orConditions = append(orConditions, bson.M{f.Name: cond})
                }
                if hasGte || hasGt {
                    cond := bson.M{}
                    if hasGte {
                        cond["$gte"] = gteVal
                    }
                    if hasGt {
                        cond["$gt"] = gtVal
                    }
                    orConditions = append(orConditions, bson.M{f.Name: cond})
                }
                
                // Add to filter as $or
                if len(orConditions) > 0 {
                    if existingOr, exists := filter["$or"]; exists {
                        // Append to existing $or
                        filter["$or"] = append(existingOr.([]bson.M), orConditions...)
                    } else {
                        filter["$or"] = orConditions
                    }
                }
            } else {
                // Normal AND logic - just add the field with all conditions
                filter[f.Name] = fieldFilter
            }
        }
    }

    return filter, nil
}

// splitQuery splits a query string by & or &amp;
func splitQuery(query string) []string {
    var result []string
    current := ""
    for i := 0; i < len(query); i++ {
        if query[i] == '&' {
            if current != "" {
                result = append(result, current)
                current = ""
            }
        } else {
            current += string(query[i])
        }
    }
    if current != "" {
        result = append(result, current)
    }
    return result
}

// toFloat64 converts interface{} to float64 for numeric comparison
func toFloat64(val interface{}) float64 {
    switch v := val.(type) {
    case int:
        return float64(v)
    case int32:
        return float64(v)
    case int64:
        return float64(v)
    case float32:
        return float64(v)
    case float64:
        return v
    default:
        return 0
    }
}
