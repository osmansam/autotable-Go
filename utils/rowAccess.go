package utils

import (
	"fmt"
    "log"
	"strings"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

// GetRowAccessFilter generates a MongoDB filter based on the user's role and container's RowAccess rules.
func GetRowAccessFilter(container *models.ContainerModel, userRole string, user map[string]interface{}) (bson.M, error) {
	if container.RowAccess == nil || len(container.RowAccess.Conditions) == 0 {
		return nil, nil // No rules defined, no filter
	}

	for _, condition := range container.RowAccess.Conditions {
		// Check if user's role matches any role in this condition
		roleMatched := false
		for _, r := range condition.Roles {
			if r == userRole {
				roleMatched = true
				break
			}
		}

		if roleMatched {
			// Found a matching condition for this user role
			condFilter, err := buildConditionFilter(condition, user)
			if err != nil {
				return nil, err
			}
			matchingConditions = append(matchingConditions, condFilter)
		}
	}

	if len(matchingConditions) == 0 {
		return nil, nil
	}

	if len(matchingConditions) == 1 {
		return matchingConditions[0], nil
	}

	// Combine all matching conditions with AND
	return bson.M{"$and": matchingConditions}, nil
}

func buildConditionFilter(condition models.Condition, user map[string]interface{}) (bson.M, error) {
	value := condition.Value

	// Handle placeholders like {{user.id}}
	if strVal, ok := value.(string); ok && strings.HasPrefix(strVal, "{{") && strings.HasSuffix(strVal, "}}") {
		resolvedVal, err := resolvePlaceholder(strVal, user)
		if err != nil {
			return nil, err
		}
		value = resolvedVal
	}

	fieldName := condition.Field
	switch condition.Operator {
	case "=":
		return bson.M{fieldName: value}, nil
	case "!=":
		return bson.M{fieldName: bson.M{"$ne": value}}, nil
	case ">":
		return bson.M{fieldName: bson.M{"$gt": value}}, nil
	case ">=":
		return bson.M{fieldName: bson.M{"$gte": value}}, nil
	case "<":
		return bson.M{fieldName: bson.M{"$lt": value}}, nil
	case "<=":
		return bson.M{fieldName: bson.M{"$lte": value}}, nil
	case "in":
		return bson.M{fieldName: bson.M{"$in": value}}, nil
	case "nin":
		return bson.M{fieldName: bson.M{"$nin": value}}, nil
	default:
		// Default to equality if unknown? Or error?
		return bson.M{fieldName: value}, nil
	}
}

func resolvePlaceholder(placeholder string, user map[string]interface{}) (interface{}, error) {
	key := strings.TrimSuffix(strings.TrimPrefix(placeholder, "{{"), "}}")
	parts := strings.Split(key, ".")

	if len(parts) == 2 && parts[0] == "user" {
		userKey := parts[1]
		if val, ok := user[userKey]; ok {
			return val, nil
		}
		// Special handling for id/_id
		if userKey == "id" {
			if val, ok := user["_id"]; ok {
				return val, nil
			}
		}
	}
	// Special handling if using OIDs in string format vs primitive
	// For now return as is or error? returning nil might effectively filter against null.
	return nil, fmt.Errorf("could not resolve placeholder: %s", placeholder)
}
