package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CreateDynamicPipeline constructs a MongoDB aggregation pipeline from a PipelineStage.
func CreateDynamicPipeline(input models.PipelineStage) (mongo.Pipeline, error) {
	var pipeline mongo.Pipeline

	// Unmarshal the JSON string into a slice of bson.M
	var stages []bson.M
	if err := json.Unmarshal([]byte(input.PipelineJSON), &stages); err != nil {
		return nil, fmt.Errorf("error parsing pipeline JSON: %w", err)
	}

	// Convert slice of bson.M to mongo.Pipeline
	for _, stage := range stages {
		// Convert bson.M (map) to bson.D (ordered document)
		stageD := bson.D{}
		for key, value := range stage {
			stageD = append(stageD, bson.E{Key: key, Value: value})
		}
		pipeline = append(pipeline, stageD)
	}

	return pipeline, nil
}

// ExecuteDynamicPipeline executes a pipeline against a MongoDB collection.
func ExecuteDynamicPipeline(ctx context.Context, collection *mongo.Collection, pipelineStage models.PipelineStage) ([]bson.M, error) {
	pipeline, err := CreateDynamicPipeline(pipelineStage)
	if err != nil {
		return nil, fmt.Errorf("error creating pipeline: %w", err)
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("error executing pipeline: %w", err)
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err = cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("error reading pipeline results: %w", err)
	}

	return results, nil
}

// ReplacePlaceholdersWithQueryParams replaces placeholders in a pipeline JSON string with query parameters.
func ReplacePlaceholdersWithQueryParams(pipelineJSON string, c *fiber.Ctx) string {
	modifiedJSON := pipelineJSON

	// Regular expression to find placeholders like {{placeholder}}
	re := regexp.MustCompile(`\{\{(.+?)\}\}`)
	matches := re.FindAllStringSubmatch(modifiedJSON, -1)

	for _, match := range matches {
		if len(match) > 1 {
			placeholder := match[1] // Placeholder name
			queryValue := c.Query(placeholder)

			// Check if the query value is an integer
			if _, err := strconv.Atoi(queryValue); err == nil {
				// It's an integer, so replace the placeholder without quotes
				modifiedJSON = strings.ReplaceAll(modifiedJSON, fmt.Sprintf("\"{{%s}}\"", placeholder), queryValue)
			} else if queryValue != "" {
				// Replace placeholder with query parameter value (as string)
				modifiedJSON = strings.ReplaceAll(modifiedJSON, fmt.Sprintf("{{%s}}", placeholder), queryValue)
			}
		}
	}

	return modifiedJSON
}

// ReplacePlaceholdersWithProjectContext replaces project-scoped placeholders in pipeline JSON.
func ReplacePlaceholdersWithProjectContext(pipelineJSON, tenantID, projectID string) string {
	modifiedJSON := strings.ReplaceAll(pipelineJSON, "{{tenantID}}", tenantID)
	modifiedJSON = strings.ReplaceAll(modifiedJSON, "{{projectID}}", projectID)

	re := regexp.MustCompile(`\{\{(?:projectCollection|collection):([a-zA-Z0-9_-]+)\}\}`)
	return re.ReplaceAllStringFunc(modifiedJSON, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return GetProjectCollectionName(tenantID, projectID, parts[1])
	})
}
