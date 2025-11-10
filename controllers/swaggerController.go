package controllers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
)

type SwaggerSpec struct {
	OpenAPI    string                 `json:"openapi"`
	Info       SwaggerInfo            `json:"info"`
	Servers    []SwaggerServer        `json:"servers"`
	Paths      map[string]interface{} `json:"paths"`
	Components SwaggerComponents      `json:"components"`
}

type SwaggerInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type SwaggerServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type SwaggerComponents struct {
	Schemas map[string]interface{} `json:"schemas"`
}

func GenerateDynamicSwagger(c *fiber.Ctx) error {
	allContainers, err := utils.GetAllContainerModels()
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to retrieve container models")
	}

	spec := SwaggerSpec{
		OpenAPI: "3.0.0",
		Info: SwaggerInfo{
			Title:       "AutoTable Dynamic API",
			Description: "Dynamically generated API documentation for all schemas",
			Version:     "1.0.0",
		},
		Servers: []SwaggerServer{
			{
				URL:         c.Protocol() + "://" + c.Get("Host"),
				Description: "Current server",
			},
		},
		Paths:      make(map[string]interface{}),
		Components: SwaggerComponents{Schemas: make(map[string]interface{})},
	}

	// Validate schema names to avoid overwriting paths/components
	dupErr := validateUniqueSchemaNames(allContainers)
	if dupErr != nil {
		// Still publish a minimal spec so Swagger UI loads, plus an error payload for visibility
		spec.Paths["/api/v1/dynamic"] = map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"Dynamic API"},
				"summary":     "Dynamic endpoint",
				"description": "Dynamic endpoint present, but per-schema docs were skipped due to duplicate or empty schema names. See /api/swagger.json error field.",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "OK"},
				},
			},
		}
		payload := map[string]interface{}{
			"error": dupErr.Error(),
			"hint":  "Ensure each ContainerModel.SchemaName is non-empty and unique. Duplicate names cause paths/components to overwrite each other.",
			"schemasDetected": extractSchemaNames(allContainers),
		}
		// Return combined object (OpenAPI + diagnostic)
		return c.JSON(map[string]interface{}{
			"openapi":    spec.OpenAPI,
			"info":       spec.Info,
			"servers":    spec.Servers,
			"paths":      spec.Paths,
			"components": spec.Components,
			"error":      payload,
		})
	}

	// Build component schemas
	for _, container := range allContainers {
		generateSchemaDefinition(&spec, container)
	}

	// Per-container endpoints with all dynamic routes
	for _, container := range allContainers {
		addPathsForContainer(&spec, container)
	}

	spec.Info.Title = "� COMPLETE VERSION - AutoTable Dynamic API"
	spec.Info.Description = fmt.Sprintf("✅ COMPLETE WITH ALL 15 DYNAMIC ROUTES - Works with ALL %d schemas! Generated with 10 path patterns and 15 HTTP operations.", len(allContainers))

	return c.JSON(spec)
}

func validateUniqueSchemaNames(containers []models.ContainerModel) error {
	seen := make(map[string]int)
	var empties []int
	for i, c := range containers {
		name := strings.TrimSpace(c.SchemaName)
		if name == "" {
			empties = append(empties, i)
			continue
		}
		seen[name]++
	}
	var dups []string
	for k, v := range seen {
		if v > 1 {
			dups = append(dups, fmt.Sprintf("%s (x%d)", k, v))
		}
	}
	if len(dups) == 0 && len(empties) == 0 {
		return nil
	}
	sort.Strings(dups)
	msg := []string{}
	if len(dups) > 0 {
		msg = append(msg, "Duplicate schema names: "+strings.Join(dups, ", "))
	}
	if len(empties) > 0 {
		msg = append(msg, fmt.Sprintf("Empty schema names at indices: %v", empties))
	}
	return fmt.Errorf(strings.Join(msg, " | "))
}

func extractSchemaNames(containers []models.ContainerModel) []string {
	out := make([]string, 0, len(containers))
	for _, c := range containers {
		out = append(out, c.SchemaName)
	}
	return out
}

func generateSchemaDefinition(spec *SwaggerSpec, container models.ContainerModel) {
	properties := make(map[string]interface{})

	for _, field := range container.Fields {
		fieldDef := map[string]interface{}{}
		switch field.Type {
		case "string":
			fieldDef["type"] = "string"
		case "number":
			fieldDef["type"] = "number"
		case "boolean":
			fieldDef["type"] = "boolean"
		case "date":
			fieldDef["type"] = "string"
			fieldDef["format"] = "date-time"
		case "objectId":
			fieldDef["type"] = "string"
			fieldDef["pattern"] = "^[0-9a-fA-F]{24}$"
			if field.ObjectSchemaName != "" {
				fieldDef["description"] = fmt.Sprintf("Reference to %s", field.ObjectSchemaName)
			}
		case "autoIncrementId":
			fieldDef["type"] = "integer"
		case "image":
			fieldDef["type"] = "string"
			fieldDef["format"] = "uri"
		case "password":
			fieldDef["type"] = "string"
			fieldDef["format"] = "password"
		default:
			fieldDef["type"] = "string"
		}
		properties[field.Name] = fieldDef
	}

	properties["_id"] = map[string]interface{}{
		"type":        "string",
		"pattern":     "^[0-9a-fA-F]{24}$",
		"description": "MongoDB ObjectID",
		"readOnly":    true,
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	spec.Components.Schemas[container.SchemaName] = schema

	inputProperties := make(map[string]interface{})
	for key, value := range properties {
		if key != "_id" {
			inputProperties[key] = value
		}
	}

	inputSchema := map[string]interface{}{
		"type":       "object",
		"properties": inputProperties,
	}

	spec.Components.Schemas[container.SchemaName+"Input"] = inputSchema
}

func addPathsForContainer(spec *SwaggerSpec, container models.ContainerModel) {
	base := "/api/v1/" + container.SchemaName
	idPath := base + "/{id}"

	itemRef := map[string]interface{}{
		"$ref": "#/components/schemas/" + container.SchemaName,
	}
	inputRef := map[string]interface{}{
		"$ref": "#/components/schemas/" + container.SchemaName + "Input",
	}

	idParam := map[string]interface{}{
		"name":        "id",
		"in":          "path",
		"required":    true,
		"description": "MongoDB ObjectID of the item",
		"schema": map[string]interface{}{
			"type":    "string",
			"pattern": "^[0-9a-fA-F]{24}$",
		},
	}

	spec.Paths[base] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "List " + container.SchemaName,
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Array of " + container.SchemaName,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type":  "array",
								"items": itemRef,
							},
						},
					},
				},
			},
		},
		"post": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Create " + container.SchemaName,
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": inputRef,
					},
				},
			},
			"responses": map[string]interface{}{
				"201": map[string]interface{}{
					"description": "Created " + container.SchemaName,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": itemRef,
						},
					},
				},
			},
		},
	}

	spec.Paths[idPath] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":       []string{container.SchemaName},
			"summary":    "Get " + container.SchemaName + " by id",
			"parameters": []map[string]interface{}{idParam},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "OK",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": itemRef,
						},
					},
				},
			},
		},
		"patch": map[string]interface{}{
			"tags":       []string{container.SchemaName},
			"summary":    "Update " + container.SchemaName + " by id",
			"parameters": []map[string]interface{}{idParam},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": inputRef,
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Updated",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": itemRef,
						},
					},
				},
			},
		},
		"delete": map[string]interface{}{
			"tags":       []string{container.SchemaName},
			"summary":    "Delete " + container.SchemaName + " by id",
			"parameters": []map[string]interface{}{idParam},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Deleted",
				},
			},
		},
	}

	spec.Paths[base+"/page"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Paginated " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "page", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 1}},
				{"name": "limit", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 10}},
				{"name": "searchKey", "in": "query", "schema": map[string]interface{}{"type": "string"}},
				{"name": "sort", "in": "query", "schema": map[string]interface{}{"type": "string"}},
				{"name": "asc", "in": "query", "schema": map[string]interface{}{"type": "boolean"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Paginated response",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"items":       map[string]interface{}{"type": "array", "items": itemRef},
									"totalItems":  map[string]interface{}{"type": "integer"},
									"totalPages":  map[string]interface{}{"type": "integer"},
									"currentPage": map[string]interface{}{"type": "integer"},
								},
							},
						},
					},
				},
			},
		},
	}

	spec.Paths[base+"/search"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Search " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "searchKey", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Search results",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type":  "array",
								"items": itemRef,
							},
						},
					},
				},
			},
		},
	}

	spec.Paths[base+"/multiple"] = map[string]interface{}{
		"post": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Create multiple " + container.SchemaName,
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": inputRef,
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"201": map[string]interface{}{
					"description": "Items created",
				},
			},
		},
		"patch": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Update multiple " + container.SchemaName,
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": inputRef,
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Items updated",
				},
			},
		},
		"delete": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Delete multiple " + container.SchemaName,
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Items deleted",
				},
			},
		},
	}

	// Add additional dynamic endpoints for each schema
	spec.Paths[base+"/filter"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Filter " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "filters", "in": "query", "schema": map[string]interface{}{"type": "string"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Filtered results",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type":  "array",
								"items": itemRef,
							},
						},
					},
				},
			},
		},
	}

	spec.Paths[base+"/pipeline"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Execute pipeline for " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "pipeline", "in": "query", "schema": map[string]interface{}{"type": "string"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Pipeline results",
				},
			},
		},
	}

	spec.Paths[base+"/testPipeline"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Test pipeline for " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "pipeline", "in": "query", "schema": map[string]interface{}{"type": "string"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Pipeline test results",
				},
			},
		},
	}

	spec.Paths[base+"/execute"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Execute code for " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "code", "in": "query", "schema": map[string]interface{}{"type": "string"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Code execution results",
				},
			},
		},
	}

	spec.Paths[base+"/api"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":    []string{container.SchemaName},
			"summary": "Execute API operations for " + container.SchemaName,
			"parameters": []map[string]interface{}{
				{"name": "operation", "in": "query", "schema": map[string]interface{}{"type": "string"}},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "API execution results",
				},
			},
		},
	}
}

func addUniversalDynamicPaths(spec *SwaggerSpec, containers []models.ContainerModel) {
	names := extractSchemaNames(containers)
	
	// Schema parameter used across all endpoints
	schemaParam := map[string]interface{}{
		"name":        "schemaName",
		"in":          "query", 
		"required":    true,
		"description": "Schema to operate on (" + fmt.Sprintf("%d schemas available", len(names)) + ")",
		"schema": map[string]interface{}{
			"type": "string",
			"enum": names,
		},
	}
	
	// Base /api/v1/dynamic endpoint
	spec.Paths["/api/v1/dynamic"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #1 GET items from any schema",
			"description": fmt.Sprintf("⭐ Route 1/15: Base GET endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				{
					"name":        "schemaName",
					"in":          "query",
					"required":    true,
					"description": "Schema to operate on",
					"schema": map[string]interface{}{
						"type": "string",
						"enum": names,
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "OK"},
			},
		},
		"post": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #2 POST create item in any schema",
			"description": fmt.Sprintf("⭐ Route 2/15: Base POST endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				{
					"name":        "schemaName",
					"in":          "query",
					"required":    true,
					"description": "Schema to operate on",
					"schema": map[string]interface{}{
						"type": "string",
						"enum": names,
					},
				},
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"type": "object"},
					},
				},
			},
			"responses": map[string]interface{}{
				"201": map[string]interface{}{"description": "Created"},
			},
		},
	}

	// Add /api/v1/dynamic/multiple endpoints
	spec.Paths["/api/v1/dynamic/multiple"] = map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #3 POST create multiple items",
			"description": fmt.Sprintf("⭐ Route 3/15: Bulk create. Available schemas: %v", names),
			"parameters":  []map[string]interface{}{schemaParam},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{"type": "object"},
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"201": map[string]interface{}{"description": "Multiple items created"},
			},
		},
		"patch": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #4 PATCH update multiple items",
			"description": fmt.Sprintf("⭐ Route 4/15: Bulk update. Available schemas: %v", names),
			"parameters":  []map[string]interface{}{schemaParam},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{"type": "object"},
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Multiple items updated"},
			},
		},
		"delete": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"}, 
			"summary":     "🔥 #5 DELETE multiple items",
			"description": fmt.Sprintf("⭐ Route 5/15: Bulk delete. Available schemas: %v", names),
			"parameters":  []map[string]interface{}{schemaParam},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{"type": "string"},
							"description": "Array of item IDs to delete",
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Multiple items deleted"},
			},
		},
	}

	// Add /api/v1/dynamic/page endpoint
	spec.Paths["/api/v1/dynamic/page"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #6 GET paginated items",
			"description": fmt.Sprintf("⭐ Route 6/15: Pagination endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				schemaParam,
				{
					"name": "page", "in": "query", "required": false,
					"schema": map[string]interface{}{"type": "integer", "default": 1},
					"description": "Page number",
				},
				{
					"name": "limit", "in": "query", "required": false,
					"schema": map[string]interface{}{"type": "integer", "default": 10},
					"description": "Items per page",
				},
				{
					"name": "searchKey", "in": "query", "required": false,
					"schema": map[string]string{"type": "string"},
					"description": "Search term",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Paginated results"},
			},
		},
	}

	// Add /api/v1/dynamic/search endpoint
	spec.Paths["/api/v1/dynamic/search"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #7 GET search items",
			"description": fmt.Sprintf("⭐ Route 7/15: Search endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				schemaParam,
				{
					"name": "searchKey", "in": "query", "required": true,
					"schema": map[string]string{"type": "string"},
					"description": "Search query",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Search results"},
			},
		},
	}

	// Add /api/v1/dynamic/filter endpoint  
	spec.Paths["/api/v1/dynamic/filter"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #8 GET filter items",
			"description": fmt.Sprintf("⭐ Route 8/15: Filter endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				schemaParam,
				{
					"name": "filters", "in": "query", "required": false,
					"schema": map[string]string{"type": "string"},
					"description": "Filter criteria (JSON format)",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Filtered results"},
			},
		},
	}

	// Add /api/v1/dynamic/pipeline endpoint
	spec.Paths["/api/v1/dynamic/pipeline"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #9 GET execute pipeline",
			"description": fmt.Sprintf("⭐ Route 9/15: Pipeline endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				schemaParam,
				{
					"name": "pipeline", "in": "query", "required": false,
					"schema": map[string]string{"type": "string"},
					"description": "Pipeline definition (JSON format)",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Pipeline results"},
			},
		},
	}

	// Add /api/v1/dynamic/testPipeline endpoint
	spec.Paths["/api/v1/dynamic/testPipeline"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"}, 
			"summary":     "🔥 #10 GET test pipeline",
			"description": fmt.Sprintf("⭐ Route 10/15: Test pipeline endpoint. Available schemas: %v", names),
			"parameters": []map[string]interface{}{
				schemaParam,
				{
					"name": "pipeline", "in": "query", "required": false,
					"schema": map[string]string{"type": "string"},
					"description": "Pipeline to test (JSON format)",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Pipeline test results"},
			},
		},
	}

	// Add /api/v1/dynamic/execute endpoint
	spec.Paths["/api/v1/dynamic/execute"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #11 GET execute code",
			"description": "⭐ Route 11/15: Execute dynamic code operations",
			"parameters": []map[string]interface{}{
				{
					"name": "code", "in": "query", "required": false,
					"schema": map[string]string{"type": "string"},
					"description": "Dynamic code to execute",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Execution results"},
			},
		},
	}

	// Add /api/v1/dynamic/api endpoint
	spec.Paths["/api/v1/dynamic/api"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #12 GET execute API",
			"description": "⭐ Route 12/15: Execute custom API operations and integrations",
			"parameters": []map[string]interface{}{
				schemaParam,
				{
					"name": "operation", "in": "query", "required": false,
					"schema": map[string]string{"type": "string"},
					"description": "API operation to execute",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "API execution results"},
			},
		},
	}

	// Add /api/v1/dynamic/{id} endpoints
	idParam := map[string]interface{}{
		"name": "id", "in": "path", "required": true,
		"schema": map[string]string{"type": "string", "pattern": "^[0-9a-fA-F]{24}$"},
		"description": "MongoDB ObjectID",
	}
	
	spec.Paths["/api/v1/dynamic/{id}"] = map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #13 GET item by ID", 
			"description": fmt.Sprintf("⭐ Route 13/15: Get item by ID. Available schemas: %v", names),
			"parameters":  []map[string]interface{}{idParam, schemaParam},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Item found"},
			},
		},
		"patch": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #14 PATCH item by ID",
			"description": fmt.Sprintf("⭐ Route 14/15: Update item by ID. Available schemas: %v", names),
			"parameters":  []map[string]interface{}{idParam, schemaParam},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"type": "object"},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Item updated"},
			},
		},
		"delete": map[string]interface{}{
			"tags":        []string{"🔥 Dynamic API"},
			"summary":     "🔥 #15 DELETE item by ID",
			"description": fmt.Sprintf("⭐ Route 15/15: Delete item by ID. Available schemas: %v", names),
			"parameters":  []map[string]interface{}{idParam, schemaParam},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Item deleted"},
			},
		},
	}
}

func GetSwaggerUI(c *fiber.Ctx) error {
	swaggerHTML := `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <title>AutoTable API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-standalone-preset.js"></script>
    <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: '` + c.Protocol() + `://` + c.Get("Host") + `/api/swagger.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      })
    }
    </script>
  </body>
</html>`
	c.Set("Content-Type", "text/html")
	return c.SendString(swaggerHTML)
}

func ListAllSchemas(c *fiber.Ctx) error {
	allContainers, err := utils.GetAllContainerModels()
	if err != nil {
		return utils.SendErrorResponse(c, err, "Failed to retrieve container models")
	}

	schemas := make([]map[string]interface{}, 0)
	for _, container := range allContainers {
		schema := map[string]interface{}{
			"name":         container.SchemaName,
			"fieldCount":   len(container.Fields),
			"hasRoutes":    true,
			"hasPipelines": len(container.Pipelines) > 0,
			"fields":       container.Fields,
		}
		schemas = append(schemas, schema)
	}

	return c.JSON(map[string]interface{}{
		"schemas": schemas,
		"total":   len(schemas),
	})
}
