package utils

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func PopulateItems(ctx context.Context, container *models.ContainerModel, items []map[string]interface{}) ([]map[string]interface{}, error) {
	for _, field := range container.Fields {
		// Check if the field has population settings
		if field.PopulationSettings != nil {
			// Validation: only allow population if targetField.Type is "objectId", "autoIncrementId" or "string" (with ObjectSchemaName)
			if field.Type == "objectId" || field.Type == "autoIncrementId" || (field.Type == "string" && field.ObjectSchemaName != "") {
				pop := field.PopulationSettings
				for i, item := range items {
					if idVal, exists := item[field.Name]; exists && idVal != nil {
						var populatedDoc map[string]interface{}
						var err error
						if field.Type == "objectId" {
							var objectId primitive.ObjectID
							switch v := idVal.(type) {
							case primitive.ObjectID:
								objectId = v
							case string:
								objectId, err = primitive.ObjectIDFromHex(v)
								if err != nil {
									continue
								}
							default:
								continue
							}
							populatedDoc, err = GetPopulatedDocument(ctx, field.ObjectSchemaName, objectId, pop.PopulatedFields)
						} else {
							// For autoIncrementId and string types, just pass idVal
							populatedDoc, err = GetPopulatedDocument(ctx, field.ObjectSchemaName, idVal, pop.PopulatedFields)
						}
						if err != nil {
							log.Printf("Failed to populate field %s for id %v: %v", field.Name, idVal, err)
							continue
						}
						items[i][field.Name] = populatedDoc
					}
				}
			}
		}
	}
	return items, nil
}

func GetPopulatedDocument(ctx context.Context, collectionName string, id interface{}, fields []string) (map[string]interface{}, error) {
    coll := configs.GetCollection(collectionName)

    projection := bson.M{}
    for _, field := range fields {
        projection[field] = 1
    }

    filter := bson.M{}
    switch v := id.(type) {
    case primitive.ObjectID:
        filter["_id"] = v
    case int, int64, float64:
        filter["_id"] = id
    case string:
        // First, try to convert the string to an integer.
        if intVal, err := strconv.Atoi(v); err == nil {
            filter["_id"] = intVal
        } else {
            // If conversion fails, use the string directly.
            filter["_id"] = v
        }
    default:
        return nil, fmt.Errorf("unsupported id type for population")
    }

    var result bson.M
    err := coll.FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&result)
    if err != nil {
        return nil, err
    }
    return result, nil
}
