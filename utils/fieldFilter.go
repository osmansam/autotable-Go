package utils

import (
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

// FilterFieldsByRole filters fields based on user's role
// Returns only fields the user is authorized to see
// If field.AuthorizeRole is empty, field is visible to everyone
func FilterFieldsByRole(fields []models.Field, userRole string) []models.Field {
	var filteredFields []models.Field

	for _, field := range fields {
		// If IsAuthorized is false, field is visible to everyone
		if !field.IsAuthorized {
			// Recursively filter children
			if len(field.Children) > 0 {
				field.Children = FilterFieldsByRole(field.Children, userRole)
			}
			filteredFields = append(filteredFields, field)
			continue
		}

		// Check if user's role is in the authorized roles
		isAuthorized := false
		for _, role := range field.AuthorizeRole {
			if role == userRole {
				isAuthorized = true
				break
			}
		}

		if isAuthorized {
			// Recursively filter children
			if len(field.Children) > 0 {
				field.Children = FilterFieldsByRole(field.Children, userRole)
			}
			filteredFields = append(filteredFields, field)
		}
	}

	return filteredFields
}

// GetAllowedFieldNames returns a set of field names the user is authorized to see
func GetAllowedFieldNames(fields []models.Field, userRole string) map[string]bool {
	allowedFields := make(map[string]bool)
	
	for _, field := range fields {
		// If IsAuthorized is false, field is visible to everyone
		if !field.IsAuthorized {
			allowedFields[field.Name] = true
			// Add children field names recursively
			if len(field.Children) > 0 {
				childFields := GetAllowedFieldNames(field.Children, userRole)
				for childName := range childFields {
					allowedFields[field.Name+"."+childName] = true
				}
			}
			continue
		}

		// Check if user's role is in the authorized roles
		for _, role := range field.AuthorizeRole {
			if role == userRole {
				allowedFields[field.Name] = true
				// Add children field names recursively
				if len(field.Children) > 0 {
					childFields := GetAllowedFieldNames(field.Children, userRole)
					for childName := range childFields {
						allowedFields[field.Name+"."+childName] = true
					}
				}
				break
			}
		}
	}

	return allowedFields
}

// FilterDocumentFields removes unauthorized fields from a document (Strict Mode)
// Only fields present in 'fields' (and allowed) or '_id' are kept.
// Unknown fields are discarded.
func FilterDocumentFields(doc bson.M, fields []models.Field, userRole string) bson.M {
	allowedFields := GetAllowedFieldNames(fields, userRole)
	filteredDoc := bson.M{}

	for key, value := range doc {
		// Always include _id field
		if key == "_id" {
			filteredDoc[key] = value
			continue
		}

		// Check if field is allowed
		if allowedFields[key] {
			// If value is a nested document, filter it recursively
			if nestedDoc, ok := value.(bson.M); ok {
				// Find the field definition
				for _, field := range fields {
					if field.Name == key && len(field.Children) > 0 {
						filteredDoc[key] = FilterDocumentFields(nestedDoc, field.Children, userRole)
						break
					}
				}
				// If no children definition found, include as-is
				if _, exists := filteredDoc[key]; !exists {
					filteredDoc[key] = value
				}
			} else {
				filteredDoc[key] = value
			}
		}
	}

	return filteredDoc
}

// FilterDocumentFieldsRelaxed removes unauthorized fields from a document (Relaxed Mode)
// Unknown fields (not in 'fields') are KEPT.
// Only fields explicitly defined in 'fields' AND restricted are removed.
func FilterDocumentFieldsRelaxed(doc bson.M, fields []models.Field, userRole string) bson.M {
	allowedFields := GetAllowedFieldNames(fields, userRole)
	
	// Map used to quickly check if a field is defined in the schema
	definedFields := make(map[string]models.Field)
	for _, f := range fields {
		definedFields[f.Name] = f
	}

	filteredDoc := bson.M{}

	for key, value := range doc {
		// Always include _id field
		if key == "_id" {
			filteredDoc[key] = value
			continue
		}
		
		fieldDef, isDefined := definedFields[key]

		if isDefined {
			// Field is defined in schema. Check if allowed.
			if allowedFields[key] {
				// Allowed. Check recursion.
				if len(fieldDef.Children) > 0 && value != nil {
					if nestedDoc, ok := value.(bson.M); ok {
						filteredDoc[key] = FilterDocumentFieldsRelaxed(nestedDoc, fieldDef.Children, userRole)
					} else if nestedMap, ok := value.(map[string]interface{}); ok {
						filteredDoc[key] = FilterDocumentFieldsRelaxed(bson.M(nestedMap), fieldDef.Children, userRole)
					} else {
						filteredDoc[key] = value
					}
				} else {
					filteredDoc[key] = value
				}
			}
			// If not allowed, it is skipped (removed)
		} else {
			// Field NOT defined in schema. Keep it (Relaxed mode).
			filteredDoc[key] = value
		}
	}

	return filteredDoc
}


// FilterDocuments filters multiple documents (Strict Mode)
func FilterDocuments(docs []map[string]interface{}, fields []models.Field, userRole string) []map[string]interface{} {
	// Convert to bson.M for filtering
	bsonDocs := make([]bson.M, len(docs))
	for i, doc := range docs {
		bsonDocs[i] = bson.M(doc)
	}

	// Filter
	filteredBsonDocs := make([]bson.M, len(bsonDocs))
	for i, doc := range bsonDocs {
		filteredBsonDocs[i] = FilterDocumentFields(doc, fields, userRole)
	}

	// Convert back to map[string]interface{}
	filteredDocs := make([]map[string]interface{}, len(filteredBsonDocs))
	for i, doc := range filteredBsonDocs {
		filteredDocs[i] = map[string]interface{}(doc)
	}

	return filteredDocs
}

// FilterDocumentsRelaxed filters multiple documents (Relaxed Mode)
func FilterDocumentsRelaxed(docs []map[string]interface{}, fields []models.Field, userRole string) []map[string]interface{} {
	// Convert to bson.M for filtering
	bsonDocs := make([]bson.M, len(docs))
	for i, doc := range docs {
		bsonDocs[i] = bson.M(doc)
	}

	// Filter
	filteredBsonDocs := make([]bson.M, len(bsonDocs))
	for i, doc := range bsonDocs {
		filteredBsonDocs[i] = FilterDocumentFieldsRelaxed(doc, fields, userRole)
	}

	// Convert back to map[string]interface{}
	filteredDocs := make([]map[string]interface{}, len(filteredBsonDocs))
	for i, doc := range filteredBsonDocs {
		filteredDocs[i] = map[string]interface{}(doc)
	}

	return filteredDocs
}
