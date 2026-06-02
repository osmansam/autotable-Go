package utils

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func PopulateItems(ctx context.Context, tenantID, projectID string, container *models.ContainerModel, items []map[string]interface{}) ([]map[string]interface{}, error) {
	for _, field := range container.Fields {
		// Check if the field has population settings
		if field.PopulationSettings != nil {
			// Validation: only allow population if targetField.Type is "objectId", "objectIdArray", "autoIncrementId" or "string" (with ObjectSchemaName)
			if field.Type == "objectId" || field.Type == "objectIdArray" || field.Type == "objectidArray" || field.Type == "autoIncrementId" || (field.Type == "string" && field.ObjectSchemaName != "") {
				pop := field.PopulationSettings
				
				// OPTIMIZATION: Batch population for objectId fields
				if field.Type == "objectId" {
					// Collect all IDs first
					var allIDs []primitive.ObjectID
					itemIndices := make(map[string][]int) // Map ID to item indices (can have duplicates)
					
					for i, item := range items {
						if idVal, exists := item[field.Name]; exists && idVal != nil {
							var objectId primitive.ObjectID
							var err error
							switch v := idVal.(type) {
							case primitive.ObjectID:
								objectId = v
							case string:
								objectId, err = primitive.ObjectIDFromHex(v)
								if err != nil {
									log.Printf("Failed to parse ObjectID for field %s at item %d: %v", field.Name, i, err)
									continue
								}
							default:
								log.Printf("Unexpected type for field %s at item %d: %T", field.Name, i, v)
								continue
							}
							
							idHex := objectId.Hex()
							// Check if this ID is already in the list to avoid duplicate fetches
							if _, exists := itemIndices[idHex]; !exists {
								allIDs = append(allIDs, objectId)
							}
							itemIndices[idHex] = append(itemIndices[idHex], i)
						}
					}
					
					// Fetch all documents in one query
					if len(allIDs) > 0 {
						populatedDocs, err := GetPopulatedDocuments(ctx, tenantID, projectID, field.ObjectSchemaName, allIDs, pop.PopulatedFields)
						if err != nil {
							log.Printf("Failed to batch populate field %s: %v", field.Name, err)
							continue
						}
						
						// Create a lookup map for populated docs
						docMap := make(map[string]map[string]interface{})
						for _, doc := range populatedDocs {
							if id, ok := doc["_id"].(primitive.ObjectID); ok {
								docMap[id.Hex()] = doc
							}
						}
						
						// Map documents back to ALL items that reference each ID
						for idHex, indices := range itemIndices {
							if doc, found := docMap[idHex]; found {
								// Populate all items that reference this ID
								for _, idx := range indices {
									items[idx][field.Name] = doc
								}
							} else {
								// Log when referenced document not found
								log.Printf("Referenced document not found in %s for ID %s (field: %s)", 
									field.ObjectSchemaName, idHex, field.Name)
							}
						}
					}
				} else if field.Type == "objectIdArray" || field.Type == "objectidArray" {
					// Handle array of ObjectIDs - collect all unique IDs first
					allIDsSet := make(map[string]primitive.ObjectID)
					itemArrayIDs := make(map[int][]primitive.ObjectID) // Map item index to its IDs
					
					for i, item := range items {
						if idVal, exists := item[field.Name]; exists && idVal != nil {
							objectIds := extractObjectIDs(idVal)
							if len(objectIds) > 0 {
								itemArrayIDs[i] = objectIds
								for _, id := range objectIds {
									allIDsSet[id.Hex()] = id
								}
							} else {
								log.Printf("Failed to extract ObjectIDs from field %s at item %d, type: %T, value: %v", 
									field.Name, i, idVal, idVal)
							}
						}
					}
					
					// Fetch all documents in one query
					if len(allIDsSet) > 0 {
						var allIDs []primitive.ObjectID
						for _, id := range allIDsSet {
							allIDs = append(allIDs, id)
						}
						
						populatedDocs, err := GetPopulatedDocuments(ctx, tenantID, projectID, field.ObjectSchemaName, allIDs, pop.PopulatedFields)
						if err != nil {
							log.Printf("Failed to batch populate array field %s: %v", field.Name, err)
							continue
						}
						
						// Create a lookup map
						docMap := make(map[string]map[string]interface{})
						for _, doc := range populatedDocs {
							if id, ok := doc["_id"].(primitive.ObjectID); ok {
								docMap[id.Hex()] = doc
							}
						}
						
						// Map documents back to items maintaining order
						for i, objectIds := range itemArrayIDs {
							var orderedDocs []map[string]interface{}
							notFoundCount := 0
							for _, id := range objectIds {
								if doc, found := docMap[id.Hex()]; found {
									orderedDocs = append(orderedDocs, doc)
								} else {
									notFoundCount++
								}
							}
							if notFoundCount > 0 {
								log.Printf("Field %s at item %d: %d referenced documents not found in %s", 
									field.Name, i, notFoundCount, field.ObjectSchemaName)
							}
							items[i][field.Name] = orderedDocs
						}
					}
				} else {
					// For autoIncrementId and string types, process individually (less common)
					for i, item := range items {
						if idVal, exists := item[field.Name]; exists && idVal != nil {
							populatedDoc, err := GetPopulatedDocument(ctx, tenantID, projectID, field.ObjectSchemaName, idVal, pop.PopulatedFields)
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
	}
	return items, nil
}

func GetPopulatedDocument(ctx context.Context, tenantID, projectID, collectionName string, id interface{}, fields []string) (map[string]interface{}, error) {
    coll := dynamicCollectionProvider(tenantID, projectID, collectionName)

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
func GetPopulatedDocuments(ctx context.Context, tenantID, projectID, collectionName string, ids []primitive.ObjectID, fields []string) ([]map[string]interface{}, error) {
    if len(ids) == 0 {
        return []map[string]interface{}{}, nil
    }

    coll := dynamicCollectionProvider(tenantID, projectID, collectionName)

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
