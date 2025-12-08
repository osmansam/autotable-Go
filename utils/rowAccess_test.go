package utils

import (
	"reflect"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func TestGetRowAccessFilter(t *testing.T) {
	// Setup user scenario
	roleID := "6935fe5dc525ee70f84a0735"
	container := &models.ContainerModel{
		SchemaName: "konu",
		RowAccess: &models.RowAccessRule{
			Conditions: []models.Condition{
				{
					Field:    "uzunluk",
					Operator: ">",
					Value:    8,
					Roles:    []string{roleID},
				},
			},
		},
	}

	userRole := roleID
	userMap := map[string]interface{}{
		"id":   "someUserId",
		"role": roleID,
	}

	// Calculate expected filter
	expectedFilter := bson.M{
		"uzunluk": bson.M{"$gt": 8},
	}

	// Execute function
	filter, err := GetRowAccessFilter(container, userRole, userMap)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify result
	if !reflect.DeepEqual(filter, expectedFilter) {
		t.Errorf("Expected filter %v, got %v", expectedFilter, filter)
	}
}
