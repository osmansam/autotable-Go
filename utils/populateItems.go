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
			// Validation: only allow population if targetField.Type is "objectId", "objectIdArray", "autoIncrementId" or "string" (with ObjectSchemaName)
			if field.Type == "objectId" || field.Type == "objectIdArray" || field.Type == "objectidArray" || field.Type == "autoIncrementId" || (field.Type == "string" && field.ObjectSchemaName != "") {
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
						} else if field.Type == "objectIdArray" || field.Type == "objectidArray" {
							// Handle array of ObjectIDs with robust type checking
							objectIds := extractObjectIDs(idVal)
							
							if len(objectIds) == 0 {
								// If no valid IDs found, skip population but don't error
								continue
							}

							// Get populated documents for all IDs
							populatedDocs, err := GetPopulatedDocuments(ctx, field.ObjectSchemaName, objectIds, pop.PopulatedFields)
							if err != nil {
								log.Printf("Failed to populate field %s for ids %v: %v", field.Name, objectIds, err)
								continue
							}

							// Create a map for quick lookup
							docMap := make(map[string]map[string]interface{})
							for _, doc := range populatedDocs {
								if id, ok := doc["_id"].(primitive.ObjectID); ok {
									docMap[id.Hex()] = doc
								}
							}

							// Reconstruct the array in original order
							var orderedDocs []map[string]interface{}
							for _, id := range objectIds {
								if doc, found := docMap[id.Hex()]; found {
									orderedDocs = append(orderedDocs, doc)
								}
							}

							items[i][field.Name] = orderedDocs
							continue
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

// GetPopulatedDocuments retrieves multiple documents by their IDs and returns only specified fields
func GetPopulatedDocuments(ctx context.Context, collectionName string, ids []primitive.ObjectID, fields []string) ([]map[string]interface{}, error) {
    if len(ids) == 0 {
        return []map[string]interface{}{}, nil
    }

    coll := configs.GetCollection(collectionName)

    projection := bson.M{}
    for _, field := range fields {
        projection[field] = 1
    }

    // Query for all IDs at once
    filter := bson.M{"_id": bson.M{"$in": ids}}

    cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(projection))
    if err != nil {
        return nil, err
    }
    defer cursor.Close(ctx)

    var results []map[string]interface{}
    for cursor.Next(ctx) {
        var result bson.M
        if err := cursor.Decode(&result); err != nil {
            log.Printf("Failed to decode document: %v", err)
            continue
        }
        results = append(results, result)
    }

    if err := cursor.Err(); err != nil {
        return nil, err
    }

    return results, nil
}

// extractObjectIDs robustly extracts ObjectIDs from various input types
func extractObjectIDs(val interface{}) []primitive.ObjectID {
    var objectIds []primitive.ObjectID
    
    switch v := val.(type) {
    case primitive.A: // primitive.A is []interface{}
        for _, item := range v {
            if oid, ok := item.(primitive.ObjectID); ok {
                objectIds = append(objectIds, oid)
            } else if str, ok := item.(string); ok {
                if oid, err := primitive.ObjectIDFromHex(str); err == nil {
                    objectIds = append(objectIds, oid)
                }
            }
        }
    case []interface{}:
        for _, item := range v {
            if oid, ok := item.(primitive.ObjectID); ok {
                objectIds = append(objectIds, oid)
            } else if str, ok := item.(string); ok {
                if oid, err := primitive.ObjectIDFromHex(str); err == nil {
                    objectIds = append(objectIds, oid)
                }
            }
        }
    case []string:
        for _, str := range v {
            if oid, err := primitive.ObjectIDFromHex(str); err == nil {
                objectIds = append(objectIds, oid)
            }
        }
    case []primitive.ObjectID:
        objectIds = v
    case primitive.ObjectID:
        objectIds = append(objectIds, v)
    case string:
        if oid, err := primitive.ObjectIDFromHex(v); err == nil {
            objectIds = append(objectIds, oid)
        }
    }
    return objectIds
}
