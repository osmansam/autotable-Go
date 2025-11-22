package utils

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var (
	reYMDDash = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)      // YYYY-MM-DD
	reYMDSlsh = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`)      // YYYY/MM/DD
	reMDYDash = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)      // MM-DD-YYYY
	reMDYSlsh = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)      // MM/DD/YYYY
)

// parseDateFlexible tries several layouts and also tells you if the input was
// a date-only value (no time). Date-only is parsed in UTC for consistency with MongoDB.
func parseDateFlexible(s string) (t time.Time, isDateOnly bool, err error) {
	// RFC3339 first (has timezone or Z)
	if tt, e := time.Parse(time.RFC3339, s); e == nil {
		return tt, false, nil
	}

	// "2006-01-02 15:04:05" - try as UTC first
	if tt, e := time.Parse("2006-01-02 15:04:05", s); e == nil {
		return tt, false, nil
	}

	// Date-only formats - parse in UTC
	switch {
	case reYMDDash.MatchString(s): // YYYY-MM-DD
		if tt, e := time.Parse("2006-01-02", s); e == nil {
			return tt, true, nil
		}
	case reYMDSlsh.MatchString(s): // YYYY/MM/DD
		if tt, e := time.Parse("2006/01/02", s); e == nil {
			return tt, true, nil
		}
	case reMDYDash.MatchString(s): // MM-DD-YYYY
		if tt, e := time.Parse("01-02-2006", s); e == nil {
			return tt, true, nil
		}
	case reMDYSlsh.MatchString(s): // MM/DD/YYYY
		if tt, e := time.Parse("01/02/2006", s); e == nil {
			return tt, true, nil
		}
	}

	return time.Time{}, false, errors.New("no date layout matched")
}

func BuildSearch(container *models.ContainerModel, key string) ([]bson.M, error) {
	if container == nil {
		return nil, errors.New("container is nil")
	}
	if key == "" {
		return nil, nil
	}

	km := regexp.QuoteMeta(key)

    // Clean key for numeric conversion
    cleanKey := strings.TrimSpace(key)
    cleanKey = strings.Trim(cleanKey, "\"'")

	intVal, intErr := strconv.Atoi(cleanKey)
	floatVal, floatErr := strconv.ParseFloat(cleanKey, 64)
	boolVal, boolErr := strconv.ParseBool(cleanKey)
	dateVal, dateOnly, dateErr := parseDateFlexible(cleanKey)

	var ors []bson.M

	for _, f := range container.Fields {
		ft := strings.ToLower(strings.TrimSpace(f.Type))

		// Skip hashed fields or fields with IsSearchable=false
		if f.IsHashed || !f.IsSearchable {
			continue
		}

		switch ft {
		case "string", "url":
			ors = append(ors, bson.M{
				f.Name: primitive.Regex{Pattern: ".*" + km + ".*", Options: "i"},
			})

		case "int", "autoincrementid":
			if intErr == nil {
				ors = append(ors, bson.M{f.Name: intVal})
			}

		case "float", "double", "number":
			if floatErr == nil {
				ors = append(ors, bson.M{f.Name: floatVal})
			}

		case "decimal":
			if d128, err := primitive.ParseDecimal128(key); err == nil {
				ors = append(ors, bson.M{f.Name: d128})
			}
			if floatErr == nil {
				ors = append(ors, bson.M{f.Name: floatVal})
			}

		case "bool", "boolean":
			if boolErr == nil {
				ors = append(ors, bson.M{f.Name: boolVal})
			}

		case "date", "datetime", "timestamp":
			// Always try a simple string match first (for partial searches like "11-12")
			fmt.Printf("Date field %q: trying string match for key=%q\n", f.Name, key)
			ors = append(ors, bson.M{
				f.Name: bson.M{
					"$regex":   regexp.QuoteMeta(key),
					"$options": "i",
				},
			})
			
			// If we can parse it as a full date, add normalized pattern too
			if dateErr == nil {
				// Debug logging
				fmt.Printf("Date field %q: also parsed as date=%v, dateOnly=%v\n", f.Name, dateVal, dateOnly)
				
				// Add the normalized YYYY-MM-DD format pattern
				datePattern := fmt.Sprintf("%04d-%02d-%02d", dateVal.Year(), int(dateVal.Month()), dateVal.Day())
				if datePattern != key {
					fmt.Printf("  Adding normalized pattern: %q for field %q\n", datePattern, f.Name)
					ors = append(ors, bson.M{
						f.Name: bson.M{
							"$regex":   datePattern,
							"$options": "i",
						},
					})
				}
			}

		case "objectid", "objectId":
			if id, err := primitive.ObjectIDFromHex(key); err == nil {
				ors = append(ors, bson.M{f.Name: id})
			}

		case "objectidarray", "objectIdArray":
			if id, err := primitive.ObjectIDFromHex(key); err == nil {
				// For array fields, match documents where the array contains this ObjectID
				ors = append(ors, bson.M{f.Name: id})
			}

		case "uuid":
			if uuidRE.MatchString(key) {
				ors = append(ors, bson.M{f.Name: key})
			}

		case "ip", "ipaddress", "enum":
			ors = append(ors, bson.M{f.Name: key})
		}
	}

	return ors, nil
}

// BuildSearchWithReferences creates a search filter that includes searching in populated fields
// by querying referenced collections and adding matching IDs to the filter.
func BuildSearchWithReferences(ctx context.Context, container *models.ContainerModel, key string) ([]bson.M, error) {
	if container == nil {
		return nil, errors.New("container is nil")
	}
	if key == "" {
		return nil, nil
	}

	// Start with the standard search clauses for direct fields
	orClauses, err := BuildSearch(container, key)
	if err != nil {
		return nil, err
	}

	// For each objectId field with PopulationSettings, search in the referenced collection
	km := regexp.QuoteMeta(key)

    // Clean key for numeric conversion
    cleanKey := strings.TrimSpace(key)
    cleanKey = strings.Trim(cleanKey, "\"'")

	intVal, intErr := strconv.Atoi(cleanKey)
	floatVal, floatErr := strconv.ParseFloat(cleanKey, 64)

	for _, field := range container.Fields {
		if (field.Type == "objectId" || field.Type == "objectid") && field.PopulationSettings != nil {
			// Build search filter for the referenced collection
			var refOrClauses []bson.M
			
			for _, popField := range field.PopulationSettings.PopulatedFields {
				// String search
				refOrClauses = append(refOrClauses, bson.M{
					popField: primitive.Regex{Pattern: ".*" + km + ".*", Options: "i"},
				})
				
				// Int search if applicable
				if intErr == nil {
					refOrClauses = append(refOrClauses, bson.M{popField: intVal})
				}
				
				// Float search if applicable
				if floatErr == nil {
					refOrClauses = append(refOrClauses, bson.M{popField: floatVal})
				}
			}
			
			if len(refOrClauses) == 0 {
				continue
			}
			
			// Query the referenced collection to find matching IDs
			refCollection := configs.GetCollection(field.ObjectSchemaName)
			refFilter := bson.M{"$or": refOrClauses}
			
			cursor, err := refCollection.Find(ctx, refFilter, options.Find().SetProjection(bson.M{"_id": 1}))
			if err != nil {
				// Log error but continue with other fields
				fmt.Printf("Error querying referenced collection %s: %v\n", field.ObjectSchemaName, err)
				continue
			}
			
			var matchingIDs []primitive.ObjectID
			for cursor.Next(ctx) {
				var result bson.M
				if err := cursor.Decode(&result); err != nil {
					continue
				}
				if id, ok := result["_id"].(primitive.ObjectID); ok {
					matchingIDs = append(matchingIDs, id)
				}
			}
			cursor.Close(ctx)
		
			// Add the matching IDs to the main search filter
			// Include both ObjectID and string representations to handle data inconsistencies
			if len(matchingIDs) > 0 {
				var idValues []interface{}
				for _, id := range matchingIDs {
					idValues = append(idValues, id)           // ObjectID
					idValues = append(idValues, id.Hex())     // String representation
				}
				orClauses = append(orClauses, bson.M{field.Name: bson.M{"$in": idValues}})
			}
		}
		
		// Handle objectIdArray fields with PopulationSettings
		if (field.Type == "objectIdArray" || field.Type == "objectidarray") && field.PopulationSettings != nil {
			// Build search filter for the referenced collection
			var refOrClauses []bson.M
			
			for _, popField := range field.PopulationSettings.PopulatedFields {
				// String search
				refOrClauses = append(refOrClauses, bson.M{
					popField: primitive.Regex{Pattern: ".*" + km + ".*", Options: "i"},
				})
				
				// Int search if applicable
				if intErr == nil {
					refOrClauses = append(refOrClauses, bson.M{popField: intVal})
				}
				
				// Float search if applicable
				if floatErr == nil {
					refOrClauses = append(refOrClauses, bson.M{popField: floatVal})
				}
			}
			
			if len(refOrClauses) == 0 {
				continue
			}
			
			// Query the referenced collection to find matching IDs
			refCollection := configs.GetCollection(field.ObjectSchemaName)
			refFilter := bson.M{"$or": refOrClauses}
			
			cursor, err := refCollection.Find(ctx, refFilter, options.Find().SetProjection(bson.M{"_id": 1}))
			if err != nil {
				// Log error but continue with other fields
				fmt.Printf("Error querying referenced collection %s: %v\n", field.ObjectSchemaName, err)
				continue
			}
			
			var matchingIDs []primitive.ObjectID
			for cursor.Next(ctx) {
				var result bson.M
				if err := cursor.Decode(&result); err != nil {
					continue
				}
				if id, ok := result["_id"].(primitive.ObjectID); ok {
					matchingIDs = append(matchingIDs, id)
				}
			}
			cursor.Close(ctx)
		
			// Add the matching IDs to the main search filter
			// For array fields, match documents where the array contains any of the matching IDs
			if len(matchingIDs) > 0 {
				var idValues []interface{}
				for _, id := range matchingIDs {
					idValues = append(idValues, id)           // ObjectID
					idValues = append(idValues, id.Hex())     // String representation
				}
				orClauses = append(orClauses, bson.M{field.Name: bson.M{"$in": idValues}})
			}
		}
	}

	return orClauses, nil
}
