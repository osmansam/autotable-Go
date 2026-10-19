package repositories

import (
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

func reconciliationUpdate(entry models.TranslationEntry, now time.Time) mongo.Pipeline {
	literal := func(value interface{}) bson.M { return bson.M{"$literal": value} }
	// Evaluate the live document atomically. Snapshot text/origin/updatedBy never
	// replace a concurrently saved manual translation, including concurrent upserts.
	status := bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{bson.M{"$type": "$sourceHash"}, "missing"}},
		literal(models.TranslationStatusMissing),
		bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$sourceHash", literal(entry.SourceHash)}}, bson.M{"$ifNull": bson.A{"$status", literal(models.TranslationStatusMissing)}}, literal(models.TranslationStatusOutdated)}},
	}}
	return mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"resourceType": literal(entry.ResourceType), "resourceId": literal(entry.ResourceID), "propertyPath": literal(entry.PropertyPath),
		"sourceText": literal(entry.SourceText), "sourceHash": literal(entry.SourceHash), "isActive": literal(entry.IsActive),
		"orphanedAt": literal(entry.OrphanedAt), "lastDiscovered": literal(entry.LastDiscovered), "updatedAt": literal(now),
		"translatedText": bson.M{"$ifNull": bson.A{"$translatedText", literal("")}},
		"origin":         bson.M{"$ifNull": bson.A{"$origin", literal(models.TranslationOriginUntranslated)}},
		"status":         status, "createdAt": bson.M{"$ifNull": bson.A{"$createdAt", literal(now)}},
	}}}}
}
