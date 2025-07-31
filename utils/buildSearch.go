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
	"2006-01-02",              // ISO date
	"2006-01-02 15:04:05",     // common timestamp
	"2006/01/02",              // alternative date format
}
func BuildSearch(container *models.ContainerModel, key string) ([]bson.M, error) {
    if container == nil {
        return nil, errors.New("container is nil")
    }
    if key == "" {
        return nil, nil
    }

    // escape for regex
    km := regexp.QuoteMeta(key)

    // pre-parse
    intVal, intErr := strconv.Atoi(key)
    floatVal, floatErr := strconv.ParseFloat(key, 64)
    boolVal, boolErr := strconv.ParseBool(key)
    dateVal, dateErr := parseDate(key)

    var ors []bson.M

    for _, f := range container.Fields {
        // skip hashed _and_ non-searchable
        if f.IsHashed || !f.IsSearchable {
            continue
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

        case "float", "decimal":
            if floatErr == nil {
                ors = append(ors, bson.M{f.Name: floatVal})
            }

        case "bool", "boolean":
            if boolErr == nil {
                ors = append(ors, bson.M{f.Name: boolVal})
            }

        case "date":
            if dateErr == nil {
                ors = append(ors, bson.M{f.Name: dateVal})
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

