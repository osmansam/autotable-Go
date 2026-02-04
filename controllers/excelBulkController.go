package controllers

import (
	"context"
	"fmt"
	"mime/multipart"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FileAnalysis holds the analysis result for a single Excel file
type FileAnalysis struct {
	FileName    string
	SchemaName  string
	Headers     []string
	DataRows    [][]string
	Fields      []models.Field
	File        *multipart.FileHeader
}

// RelationshipDetection holds detected relationship information
type RelationshipDetection struct {
	SourceSchema     string
	SourceField      string
	TargetSchema     string
	TargetFieldIndex int
	Confidence       string // "high", "medium", "low"
}

// UploadMultipleExcel handles multiple Excel file uploads with automatic relationship detection
func UploadMultipleExcel(c *fiber.Ctx) error {
	// Get tenant and project context
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Invalid tenant or project",
			Data:    nil,
		})
	}

	// Get all uploaded files
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Failed to parse form data",
			Data:    nil,
		})
	}

	files := form.File["files"]
	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "At least one Excel file is required",
			Data:    nil,
		})
	}

	// Step 1: Analyze all files
	var fileAnalyses []FileAnalysis
	for _, file := range files {
		analysis, err := analyzeExcelFile(file)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
				Status:  fiber.StatusBadRequest,
				Message: fmt.Sprintf("Failed to analyze file %s: %v", file.Filename, err),
				Data:    nil,
			})
		}
		fileAnalyses = append(fileAnalyses, *analysis)
	}

	// Step 2: Detect relationships between files
	relationships := detectRelationships(fileAnalyses)

	// Step 3: Determine creation order (referenced tables first)
	orderedAnalyses := orderByDependencies(fileAnalyses, relationships)

	// Step 4: Update field types based on detected relationships
	updateFieldsWithRelationships(orderedAnalyses, relationships)

	// Step 5: Create containers and insert data
	createdContainers := make(map[string]string) // schemaName -> containerID
	var results []map[string]interface{}

	containersCollection := utils.GetContainerCollectionForProject(tenantID, projectID)

	for _, analysis := range orderedAnalyses {
		// Create container
		containerID := primitive.NewObjectID()
		container := models.ContainerModel{
			ID:         containerID,
			SchemaName: analysis.SchemaName,
			Fields:     analysis.Fields,
			Routes: models.Routes{
				CreateDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "POST"},
				GetAllDynamicModelItems:               models.RouteSpec{IsActive: true, Method: "GET"},
				CreateMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "POST"},
				GetAllDynamicModelItemsWithPagination: models.RouteSpec{IsActive: true, Method: "GET"},
				GetPipeline:                           models.RouteSpec{IsActive: true, Method: "GET"},
				TestPipeline:                          models.RouteSpec{IsActive: true, Method: "POST"},
				HandleSearchDynamicModelItem:          models.RouteSpec{IsActive: true, Method: "GET"},
				HandleFilterDynamicModelItem:          models.RouteSpec{IsActive: true, Method: "GET"},
				DeleteDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "DELETE"},
				UpdateDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "PUT"},
				UpdateMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "PUT"},
				GetDynamicModelItem:                   models.RouteSpec{IsActive: true, Method: "GET"},
				DeleteMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "DELETE"},
				ExportDynamicModelItems:               models.RouteSpec{IsActive: true, Method: "GET"},
				GetItemsForSelection:                  models.RouteSpec{IsActive: true, Method: "GET"},
			},
			Redis: models.Redis{
				IsRedisCached:        false,
				CacheTime:            0,
				TriggeredRedisCaches: []string{},
			},
			Pipelines:        []models.PipelineStage{},
			DynamicFunctions: []models.DynamicFunction{},
			DynamicApis:      []models.DynamicApiModel{},
			IsAuthContainer:  false,
			IsRegisterActive: false,
			PopulatedRoutes:  []string{},
			Indexes:          []models.Index{},
		}

		_, err := containersCollection.InsertOne(c.Context(), container)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  fiber.StatusInternalServerError,
				Message: fmt.Sprintf("Failed to create container %s: %v", analysis.SchemaName, err),
				Data:    nil,
			})
		}

		createdContainers[analysis.SchemaName] = containerID.Hex()

		// Insert data
		dataCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, analysis.SchemaName)
		var documents []interface{}

		for _, row := range analysis.DataRows {
			doc := bson.M{
				"_id":       primitive.NewObjectID(),
				"tenantID":  tenantID,
				"projectID": projectID,
				"createdAt": time.Now(),
				"updatedAt": time.Now(),
			}

			for i, cell := range row {
				if i >= len(analysis.Fields) {
					break
				}
				field := analysis.Fields[i]
				value := convertValueWithReference(cell, field)
				doc[field.Name] = value
			}

			documents = append(documents, doc)
		}

		var rowsInserted int
		if len(documents) > 0 {
			result, err := dataCollection.InsertMany(c.Context(), documents)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
					Status:  fiber.StatusInternalServerError,
					Message: fmt.Sprintf("Failed to insert data for %s: %v", analysis.SchemaName, err),
					Data:    nil,
				})
			}
			rowsInserted = len(result.InsertedIDs)
		}

		results = append(results, map[string]interface{}{
			"schemaName":   analysis.SchemaName,
			"containerId":  containerID.Hex(),
			"rowsInserted": rowsInserted,
			"fields":       analysis.Fields,
		})
	}

	// Invalidate Redis cache
	ctx := context.Background()
	cacheKey := fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID)
	configs.RedisClient.Del(ctx, cacheKey)

	// Emit WebSocket event
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitContainerChanged(userIDStr, tenantID, projectID)

	return c.Status(fiber.StatusCreated).JSON(responses.GeneralResponse{
		Status:  fiber.StatusCreated,
		Message: "Multiple Excel files successfully imported with relationships",
		Data: map[string]interface{}{
			"containers":    results,
			"relationships": relationships,
			"totalFiles":    len(files),
		},
	})
}

// analyzeExcelFile analyzes a single Excel file
func analyzeExcelFile(file *multipart.FileHeader) (*FileAnalysis, error) {
	// Open the uploaded file
	uploadedFile, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer uploadedFile.Close()

	// Read Excel file
	xlFile, err := excelize.OpenReader(uploadedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read Excel: %v", err)
	}
	defer xlFile.Close()

	// Get the first sheet
	sheetName := xlFile.GetSheetName(0)
	if sheetName == "" {
		return nil, fmt.Errorf("no sheets found")
	}

	// Read all rows
	rows, err := xlFile.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("must have at least header and one data row")
	}

	headers := rows[0]
	dataRows := rows[1:]

	// Generate schema name from filename
	schemaName := strings.TrimSuffix(file.Filename, ".xlsx")
	schemaName = strings.TrimSuffix(schemaName, ".xls")
	schemaName = sanitizeFieldName(schemaName)

	// Analyze data and create field definitions (without relationships yet)
	fields, err := analyzeExcelData(headers, dataRows)
	if err != nil {
		return nil, err
	}

	return &FileAnalysis{
		FileName:   file.Filename,
		SchemaName: schemaName,
		Headers:    headers,
		DataRows:   dataRows,
		Fields:     fields,
		File:       file,
	}, nil
}

// detectRelationships analyzes all files to find potential relationships
func detectRelationships(analyses []FileAnalysis) []RelationshipDetection {
	var relationships []RelationshipDetection

	// Create a map of schema names for quick lookup
	schemaMap := make(map[string]int)
	for i, analysis := range analyses {
		schemaMap[analysis.SchemaName] = i
	}

	// Pattern for detecting foreign key columns
	foreignKeyPattern := regexp.MustCompile(`(?i)(.+)(id|ID|Id)$`)

	// Analyze each file
	for _, sourceAnalysis := range analyses {
		for i, field := range sourceAnalysis.Fields {
			// Check if field name matches foreign key pattern
			matches := foreignKeyPattern.FindStringSubmatch(field.Name)
			if matches != nil && len(matches) > 1 {
				// Extract the potential target schema name
				potentialTarget := matches[1]
				potentialTargetCamel := sanitizeFieldName(potentialTarget)

				// Look for exact match
				if targetIdx, exists := schemaMap[potentialTargetCamel]; exists {
					relationships = append(relationships, RelationshipDetection{
						SourceSchema:     sourceAnalysis.SchemaName,
						SourceField:      field.Name,
						TargetSchema:     analyses[targetIdx].SchemaName,
						TargetFieldIndex: i,
						Confidence:       "high",
					})
					continue
				}

				// Look for pluralized match (e.g., "userId" -> "users")
				pluralTarget := potentialTargetCamel + "s"
				if targetIdx, exists := schemaMap[pluralTarget]; exists {
					relationships = append(relationships, RelationshipDetection{
						SourceSchema:     sourceAnalysis.SchemaName,
						SourceField:      field.Name,
						TargetSchema:     analyses[targetIdx].SchemaName,
						TargetFieldIndex: i,
						Confidence:       "high",
					})
					continue
				}

				// Look for singularized match (e.g., "categoriesId" -> "category")
				if strings.HasSuffix(potentialTargetCamel, "s") {
					singularTarget := strings.TrimSuffix(potentialTargetCamel, "s")
					if targetIdx, exists := schemaMap[singularTarget]; exists {
						relationships = append(relationships, RelationshipDetection{
							SourceSchema:     sourceAnalysis.SchemaName,
							SourceField:      field.Name,
							TargetSchema:     analyses[targetIdx].SchemaName,
							TargetFieldIndex: i,
							Confidence:       "medium",
						})
					}
				}
			}
		}
	}

	return relationships
}

// orderByDependencies orders the analyses so referenced tables are created first
func orderByDependencies(analyses []FileAnalysis, relationships []RelationshipDetection) []FileAnalysis {
	// Create a dependency map
	dependsOn := make(map[string]map[string]bool)
	for _, analysis := range analyses {
		dependsOn[analysis.SchemaName] = make(map[string]bool)
	}

	// Populate dependencies
	for _, rel := range relationships {
		dependsOn[rel.SourceSchema][rel.TargetSchema] = true
	}

	// Topological sort
	var ordered []FileAnalysis
	visited := make(map[string]bool)

	var visit func(string)
	visit = func(schemaName string) {
		if visited[schemaName] {
			return
		}
		visited[schemaName] = true

		// Visit dependencies first
		for dep := range dependsOn[schemaName] {
			visit(dep)
		}

		// Find and add the analysis
		for _, analysis := range analyses {
			if analysis.SchemaName == schemaName {
				ordered = append(ordered, analysis)
				break
			}
		}
	}

	for _, analysis := range analyses {
		visit(analysis.SchemaName)
	}

	return ordered
}

// updateFieldsWithRelationships updates field types based on detected relationships
func updateFieldsWithRelationships(analyses []FileAnalysis, relationships []RelationshipDetection) {
	for _, rel := range relationships {
		// Find the source analysis
		for i := range analyses {
			if analyses[i].SchemaName == rel.SourceSchema {
				// Find the field and update it to reference type
				for j := range analyses[i].Fields {
					if analyses[i].Fields[j].Name == rel.SourceField {
						analyses[i].Fields[j].Type = "reference"
						analyses[i].Fields[j].ObjectSchemaName = rel.TargetSchema
						break
					}
				}
				break
			}
		}
	}
}

// convertValueWithReference converts a value, handling reference types
func convertValueWithReference(value string, field models.Field) interface{} {
	if value == "" {
		return nil
	}

	// If it's a reference type, try to convert to ObjectID
	if field.Type == "reference" {
		if objectID, err := primitive.ObjectIDFromHex(value); err == nil {
			return objectID
		}
		// If not a valid ObjectID, return as string (will need manual mapping)
		return value
	}

	// Use the existing convertValue function
	return convertValue(value, field.Type)
}
