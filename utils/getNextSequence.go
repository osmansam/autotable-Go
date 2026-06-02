package utils

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func GetNextSequence(ctx context.Context, schemaName string) (int64, error) {
    countersColl := countersCollectionProvider()
    
    filter := bson.M{"_id": schemaName}
    update := bson.M{"$inc": bson.M{"seq": 1}}
    opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
    
    var result struct {
        Seq int64 `bson:"seq"`
    }
    err := countersColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)
    if err != nil {
        return 0, err
    }
    return result.Seq, nil
}
