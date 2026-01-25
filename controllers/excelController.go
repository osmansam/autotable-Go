package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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

// UploadExcel handles Excel file upload and creates a container with data
func UploadExcel(c *fiber.Ctx) error {
	// Get tenant and project context
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Invalid tenant or project",
			Data:    nil,
		})
	}

	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Excel file is required",
			Data:    nil,
		})
	}

	// Get schema name from form (optional, will use filename if not provided)
	schemaName := c.FormValue("schemaName")
	if schemaName == "" {
		// Use filename without extension as schema name
		schemaName = strings.TrimSuffix(file.Filename, ".xlsx")
		schemaName = strings.TrimSuffix(schemaName, ".xls")
		schemaName = sanitizeFieldName(schemaName)
	}

	// Open the uploaded file
	uploadedFile, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Failed to open uploaded file",
			Data:    nil,
		})
	}
	defer uploadedFile.Close()

	// Read Excel file
	xlFile, err := excelize.OpenReader(uploadedFile)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Failed to read Excel file",
			Data:    nil,
		})
	}
	defer xlFile.Close()

	// Get the first sheet
	sheetName := xlFile.GetSheetName(0)
	if sheetName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Excel file has no sheets",
			Data:    nil,
		})
	}

	// Read all rows
	rows, err := xlFile.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Excel file must have at least a header row and one data row",
			Data:    nil,
		})
	}

	// First row is headers
	headers := rows[0]
	dataRows := rows[1:]

	// Analyze data and create field definitions
	fields, err := analyzeExcelData(headers, dataRows)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Failed to analyze Excel data",
			Data:    nil,
		})
	}

	// Create container model
	containerID := primitive.NewObjectID()
	container := models.ContainerModel{
		ID:         containerID,
		SchemaName: schemaName,
		Fields:     fields,
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

	// Get project-specific container collection
	containersCollection := utils.GetContainerCollectionForProject(tenantID, projectID)
	
	_, err = containersCollection.InsertOne(c.Context(), container)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to create container: %v", err),
			Data:    nil,
		})
	}

	// Invalidate Redis cache for containers
	ctx := context.Background()
	cacheKey := fmt.Sprintf("containers:all:tenant_%s:project_%s", tenantID, projectID)
	configs.RedisClient.Del(ctx, cacheKey)

	// Emit WebSocket event for container change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitContainerChanged(userIDStr)

	// Create dynamic collection for data
	dataCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	// Prepare data for insertion
	var documents []interface{}
	for _, row := range dataRows {
		doc := bson.M{
			"_id":        primitive.NewObjectID(),
			"tenantID":   tenantID,
			"projectID":  projectID,
			"createdAt":  time.Now(),
			"updatedAt":  time.Now(),
		}

		// Map each cell to corresponding field
		for i, cell := range row {
			if i >= len(fields) {
				break
			}
			field := fields[i]
			value := convertValue(cell, field.Type)
			doc[field.Name] = value
		}

		documents = append(documents, doc)
	}

	// Insert all data
	var rowsInserted int
	if len(documents) > 0 {
		result, err := dataCollection.InsertMany(c.Context(), documents)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  fiber.StatusInternalServerError,
				Message: "Failed to insert data",
				Data:    nil,
			})
		}
		rowsInserted = len(result.InsertedIDs)
	}

	return c.Status(fiber.StatusCreated).JSON(responses.GeneralResponse{
		Status:  fiber.StatusCreated,
		Message: "Excel data successfully imported",
		Data: map[string]interface{}{
			"containerId":  containerID.Hex(),
			"schemaName":   schemaName,
			"rowsInserted": rowsInserted,
			"fields":       fields,
		},
	})
}

// analyzeExcelData analyzes headers and data rows to determine field types
func analyzeExcelData(headers []string, dataRows [][]string) ([]models.Field, error) {
	var fields []models.Field

	for i, header := range headers {
		if header == "" {
			continue
		}

		fieldName := sanitizeFieldName(header)
		fieldType := "string" // default type

		// Sample data from column to detect type
		var columnSamples []string
		sampleSize := 10
		if len(dataRows) < sampleSize {
			sampleSize = len(dataRows)
		}

		for j := 0; j < sampleSize && j < len(dataRows); j++ {
			if i < len(dataRows[j]) {
				columnSamples = append(columnSamples, dataRows[j][i])
			}
		}

		// Detect field type from samples
		if len(columnSamples) > 0 {
			fieldType = detectFieldType(columnSamples)
		}

		field := models.Field{
			Name:         fieldName,
			Type:         fieldType,
			Unique:       false,
			IsSearchable: true, // Enable search for Excel-imported fields
		}

		fields = append(fields, field)
	}

	return fields, nil
}

// detectFieldType analyzes column samples to determine the best field type
// Returns types compatible with validation.go: string, int, float, bool, date, url, uuid, ip, etc.
func detectFieldType(samples []string) string {
	if len(samples) == 0 {
		return "string"
	}

	// Filter out empty samples
	var nonEmptySamples []string
	for _, s := range samples {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			nonEmptySamples = append(nonEmptySamples, trimmed)
		}
	}

	if len(nonEmptySamples) == 0 {
		return "string"
	}

	// Count matches for each type
	intCount := 0
	floatCount := 0
	boolCount := 0
	emailCount := 0
	dateCount := 0
	urlCount := 0
	uuidCount := 0
	ipCount := 0

	for _, sample := range nonEmptySamples {
		if isBool(sample) {
			boolCount++
		}
		if isInteger(sample) {
			intCount++
		} else if isNumeric(sample) {
			// Only count as float if it's NOT an integer
			floatCount++
		}
		if isEmail(sample) {
			emailCount++
		}
		if isURL(sample) {
			urlCount++
		}
		if isUUID(sample) {
			uuidCount++
		}
		if isIPAddress(sample) {
			ipCount++
		}
		if isDate(sample) {
			dateCount++
		}
	}

	total := len(nonEmptySamples)

	// Priority: bool > int > float > uuid > ip > url > email > date > string
	// Require all samples to match for numeric/bool types (strict)
	if boolCount == total {
		return "bool"
	}
	if intCount == total {
		return "int"
	}
	if floatCount == total {
		return "float"
	}
	if uuidCount == total {
		return "uuid"
	}
	if ipCount == total {
		return "ip"
	}
	if urlCount == total {
		return "url"
	}
	if emailCount == total {
		return "string" // email is validated via tag, type is still string
	}
	if dateCount == total {
		return "date"
	}

	return "string"
}

// sanitizeFieldName converts a header string to a valid field name (camelCase)
func sanitizeFieldName(header string) string {
	// Remove special characters and convert to camelCase
	words := strings.FieldsFunc(header, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})

	if len(words) == 0 {
		return "field"
	}

	// First word lowercase, rest capitalized
	result := strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		if len(words[i]) > 0 {
			result += strings.ToUpper(string(words[i][0])) + strings.ToLower(words[i][1:])
		}
	}

	return result
}

// isBool checks if a string is a valid boolean
func isBool(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return lower == "true" || lower == "false" || lower == "1" || lower == "0" || lower == "yes" || lower == "no"
}

// isInteger checks if a string is a valid integer
func isInteger(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// isNumeric checks if a string is a valid number (int or float)
func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// isEmail checks if a string is a valid email
func isEmail(s string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(s)
}

// isURL checks if a string is a valid URL
func isURL(s string) bool {
	urlRegex := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
	return urlRegex.MatchString(s)
}

// isUUID checks if a string is a valid UUID
func isUUID(s string) bool {
	uuidRegex := regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	return uuidRegex.MatchString(s)
}

// isIPAddress checks if a string is a valid IP address
func isIPAddress(s string) bool {
	ipRegex := regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	if ipRegex.MatchString(s) {
		return true
	}
	// IPv6 simple check
	ipv6Regex := regexp.MustCompile(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`)
	return ipv6Regex.MatchString(s)
}

// isDate checks if a string is a valid date
func isDate(s string) bool {
	dateFormats := []string{
		"2006-01-02",
		"01/02/2006",
		"02/01/2006",
		"2006/01/02",
		"Jan 02, 2006",
		"02-Jan-2006",
		time.RFC3339,
	}

	for _, format := range dateFormats {
		_, err := time.Parse(format, s)
		if err == nil {
			return true
		}
	}

	return false
}

// convertValue converts a string value to the appropriate type
func convertValue(value string, fieldType string) interface{} {
	if value == "" {
		return nil
	}

	switch fieldType {
	case "bool", "boolean":
		lower := strings.ToLower(strings.TrimSpace(value))
		return lower == "true" || lower == "1" || lower == "yes"
	case "int":
		if num, err := strconv.Atoi(value); err == nil {
			return num
		}
	case "float", "decimal":
		if num, err := strconv.ParseFloat(value, 64); err == nil {
			return num
		}
	case "date":
		dateFormats := []string{
			"2006-01-02",
			"01/02/2006",
			"02/01/2006",
			"2006/01/02",
			"Jan 02, 2006",
			"02-Jan-2006",
			time.RFC3339,
		}
		for _, format := range dateFormats {
			if t, err := time.Parse(format, value); err == nil {
				return t
			}
		}
	}

	// For string, url, uuid, ip, and other string-based types, return as-is
	return value
}
