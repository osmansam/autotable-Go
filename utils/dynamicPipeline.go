package utils

import (
	"context"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func CreateDynamicPipeline(input models.PipelineStageInput) (mongo.Pipeline, error) {
	var pipeline mongo.Pipeline

	if input.Match != "" {
		var matchStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Match), true, &matchStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{"$match", matchStage}})
	}

	if input.Lookup != "" {
		var lookupStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Lookup), true, &lookupStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{"$lookup", lookupStage}})
	}

	if input.Unwind != "" {
		var unwindStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Unwind), true, &unwindStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{"$unwind", unwindStage}})
	}

	if input.Group != "" {
		var groupStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Group), true, &groupStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{"$group", groupStage}})
	}

	if input.Sort != "" {
		var sortStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Sort), true, &sortStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{"$sort", sortStage}})
	}
	if input.AddFields != "" {
		var addFieldsStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.AddFields), true, &addFieldsStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{Key: "$addFields", Value: addFieldsStage}})
	}

	if input.Limit != "" {
		var limitStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Limit), true, &limitStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: limitStage}})
	}

	if input.Skip != "" {
		var skipStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Skip), true, &skipStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{Key: "$skip", Value: skipStage}})
	}

	if input.Facet != "" {
		var facetStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Facet), true, &facetStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{Key: "$facet", Value: facetStage}})
	}

	if input.Merge != "" {
		var mergeStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Merge), true, &mergeStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{Key: "$merge", Value: mergeStage}})
	}

	if input.Out != "" {
		var outStage bson.D
		if err := bson.UnmarshalExtJSON([]byte(input.Out), true, &outStage); err != nil {
			return nil, err
		}
		pipeline = append(pipeline, bson.D{{Key: "$out", Value: outStage}})
	}
	return pipeline, nil
}

func ExecuteDynamicPipeline(ctx context.Context, collection *mongo.Collection, input models.PipelineStageInput) ([]bson.M, error) {
    pipeline, err := CreateDynamicPipeline(input)
    if err != nil {
        return nil, err
    }

    cursor, err := collection.Aggregate(ctx, pipeline)
    if err != nil {
        return nil, err
    }
    defer cursor.Close(ctx)

    var results []bson.M
    if err = cursor.All(ctx, &results); err != nil {
        return nil, err
    }

    return results, nil
}
