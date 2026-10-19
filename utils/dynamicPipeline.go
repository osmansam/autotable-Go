package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo/options"
	"regexp"
	"strconv"
	"strings"
	"time"

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
func ExecuteDynamicPipeline(ctx context.Context, collection *mongo.Collection, pipelineStage models.PipelineStage, scope ...string) ([]bson.M, error) {
	tenantID, projectID := "", ""
	if len(scope) == 2 {
		tenantID, projectID = scope[0], scope[1]
	}
	pipeline, err := ReadOnlyProjectPipeline(pipelineStage, tenantID, projectID)
	if err != nil {
		return nil, fmt.Errorf("error creating pipeline: %w", err)
	}

	cursor, err := collection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(false).SetMaxTime(10*time.Second))
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
	var pipeline interface{}
	decoder := json.NewDecoder(strings.NewReader(pipelineJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&pipeline); err != nil {
		return pipelineJSON
	}
	re := regexp.MustCompile(`\{\{(.+?)\}\}`)
	var resolve func(interface{}) interface{}
	resolve = func(value interface{}) interface{} {
		switch item := value.(type) {
		case map[string]interface{}:
			for key, child := range item {
				item[key] = resolve(child)
			}
			return item
		case []interface{}:
			for i, child := range item {
				item[i] = resolve(child)
			}
			return item
		case string:
			match := re.FindStringSubmatch(item)
			if len(match) == 2 && match[0] == item {
				query := c.Query(strings.TrimSpace(match[1]))
				if query != "" {
					if number, err := strconv.ParseInt(query, 10, 64); err == nil {
						return number
					}
					return query
				}
			}
			return re.ReplaceAllStringFunc(item, func(token string) string {
				match := re.FindStringSubmatch(token)
				query := c.Query(strings.TrimSpace(match[1]))
				if query == "" {
					return token
				}
				return query
			})
		}
		return value
	}
	result, err := json.Marshal(resolve(pipeline))
	if err != nil {
		return pipelineJSON
	}
	return string(result)
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
