package utils

import (
	"fmt"
	"strings"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ReadOnlyProjectPipeline validates stages and every nested collection reference.
// Unknown stages fail closed rather than inheriting a future MongoDB capability.
func ReadOnlyProjectPipeline(stage models.PipelineStage, tenantID, projectID string) (mongo.Pipeline, error) {
	pipeline, err := CreateDynamicPipeline(stage)
	if err != nil {
		return nil, err
	}
	if err := validateReadOnlyPipeline(pipeline, tenantID, projectID, 0); err != nil {
		return nil, err
	}
	return append(pipeline, bson.D{{Key: "$limit", Value: configs.GetMaxUnboundedReadLimit()}}), nil
}

func validateReadOnlyPipeline(pipeline mongo.Pipeline, tenantID, projectID string, depth int) error {
	if len(pipeline) > 100 || depth > 8 {
		return fmt.Errorf("pipeline exceeds stage/depth limits")
	}
	allowed := map[string]bool{"$match": true, "$group": true, "$project": true, "$sort": true, "$limit": true, "$skip": true, "$count": true, "$unwind": true, "$addFields": true, "$set": true, "$unset": true, "$replaceRoot": true, "$replaceWith": true, "$sortByCount": true, "$bucket": true, "$bucketAuto": true, "$sample": true, "$facet": true, "$lookup": true, "$graphLookup": true, "$unionWith": true}
	for _, stage := range pipeline {
		if len(stage) != 1 || !allowed[stage[0].Key] {
			return fmt.Errorf("pipeline stage is not allowed")
		}
		op, value := stage[0].Key, stage[0].Value
		if err := validatePipelineExpressions(value); err != nil {
			return err
		}
		if op == "$lookup" || op == "$graphLookup" || op == "$unionWith" {
			var collection string
			if text, ok := value.(string); ok && op == "$unionWith" {
				collection = text
			} else if object, ok := value.(map[string]interface{}); ok {
				key := "from"
				if op == "$unionWith" {
					key = "coll"
				}
				collection, _ = object[key].(string)
			}
			prefix := GetProjectCollectionName(tenantID, projectID, "")
			if tenantID == "" || projectID == "" || !strings.HasPrefix(collection, prefix) || len(collection) == len(prefix) {
				return fmt.Errorf("pipeline collection reference is outside the current project")
			}
		}
		if op == "$facet" || op == "$lookup" || op == "$unionWith" {
			if err := validateNestedPipelines(value, tenantID, projectID, depth+1, op == "$facet"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNestedPipelines(value interface{}, tenantID, projectID string, depth int, facet bool) error {
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	for key, child := range object {
		if key != "pipeline" && !facet {
			continue
		}
		items, ok := child.([]interface{})
		if !ok {
			return fmt.Errorf("nested pipeline must be an array")
		}
		var pipeline mongo.Pipeline
		for _, item := range items {
			stage, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Errorf("nested stage must be an object")
			}
			var document bson.D
			for op, value := range stage {
				document = append(document, bson.E{Key: op, Value: value})
			}
			pipeline = append(pipeline, document)
		}
		if err := validateReadOnlyPipeline(pipeline, tenantID, projectID, depth); err != nil {
			return err
		}
	}
	return nil
}

func validatePipelineExpressions(value interface{}) error {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, child := range item {
			if key == "$where" || key == "$function" || key == "$accumulator" {
				return fmt.Errorf("server-side code expressions are not allowed")
			}
			if err := validatePipelineExpressions(child); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range item {
			if err := validatePipelineExpressions(child); err != nil {
				return err
			}
		}
	}
	return nil
}
