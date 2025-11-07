package utils

import (
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02",          // ISO date
	"2006-01-02 15:04:05", // common ts
	"2006/01/02",
}

func parseDateFlexible(s string) (t time.Time, layout string, err error) {
	for _, layout = range dateLayouts {
		if tt, e := time.Parse(layout, s); e == nil {
			return tt, layout, nil
		}
	}
	return time.Time{}, "", errors.New("no date layout matched")
}

func BuildSearch(container *models.ContainerModel, key string) ([]bson.M, error) {
	if container == nil {
		return nil, errors.New("container is nil")
	}
	if key == "" {
		// caller decides whether to return all docs or 400
		return nil, nil
	}

	km := regexp.QuoteMeta(key)

	intVal, intErr := strconv.Atoi(key)
	floatVal, floatErr := strconv.ParseFloat(key, 64)
	boolVal, boolErr := strconv.ParseBool(key)
	dateVal, matchedLayout, dateErr := parseDateFlexible(key)

	var ors []bson.M

	for _, f := range container.Fields {
		// skip hashed; respect IsSearchable only when explicitly false (backward-compat)
		if f.IsHashed || (f.IsSearchable == false && f.Type != "string") {
			// strings are commonly expected to search unless explicitly hashed
			if f.Type != "string" {
				continue
			}
		}

		switch f.Type {
		case "string", "url":
			ors = append(ors, bson.M{
				f.Name: primitive.Regex{Pattern: ".*" + km + ".*", Options: "i"},
			})

		case "int", "autoIncrementId":
			if intErr == nil {
				ors = append(ors, bson.M{f.Name: intVal})
			}

		case "float":
			if floatErr == nil {
				ors = append(ors, bson.M{f.Name: floatVal})
			}

		case "decimal":
			// Try Decimal128 and also float64 to be robust against stored type differences
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

		case "date":
			if dateErr == nil {
				// If layout is date-only, search the whole day range; otherwise exact timestamp
				if matchedLayout == "2006-01-02" || matchedLayout == "2006/01/02" {
					start := time.Date(dateVal.Year(), dateVal.Month(), dateVal.Day(), 0, 0, 0, 0, time.UTC)
					end := start.Add(24 * time.Hour)
					ors = append(ors, bson.M{f.Name: bson.M{"$gte": start, "$lt": end}})
				} else {
					ors = append(ors, bson.M{f.Name: dateVal})
				}
			}

		case "objectId":
			if id, err := primitive.ObjectIDFromHex(key); err == nil {
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
