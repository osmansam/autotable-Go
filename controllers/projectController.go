package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CreateProjectInput represents the project creation payload
type CreateProjectInput struct {
	Name                 string `json:"name" validate:"required,min=2,max=100"`
	Slug                 string `json:"slug" validate:"required,min=2,max=50"`
	TemplateProjectID    string `json:"templateProjectId,omitempty"`
	IncludeTemplateItems *bool  `json:"includeTemplateItems,omitempty"`
}

// UpdateProjectInput represents the project update payload
type UpdateProjectInput struct {
	Name     *string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Slug     *string `json:"slug,omitempty" validate:"omitempty,min=2,max=50"`
	IsActive *bool   `json:"isActive,omitempty"`
}

type UpdateProjectTemplateInput struct {
	IsTemplate           bool   `json:"isTemplate"`
	TemplateIncludeItems *bool  `json:"templateIncludeItems,omitempty"`
	TemplateDescription  string `json:"templateDescription,omitempty"`
}

const (
	projectTemplateScopeTenant = "tenant"
	projectTemplateScopeGlobal = "global"
)

type projectCloneIDMap map[string]map[primitive.ObjectID]primitive.ObjectID

// GetCollectionNameForProject generates a unique collection name for a project
// Format: "tenant_{tenantId}_project_{projectId}_{schemaName}"
func GetCollectionNameForProject(tenantID, projectID, schemaName string) string {
	return fmt.Sprintf("tenant_%s_project_%s_%s", tenantID, projectID, schemaName)
}

// GetProjectPrefix returns the prefix for all collections in a project
// This is useful for listing or cleaning up project collections
func GetProjectPrefix(tenantID, projectID string) string {
	return fmt.Sprintf("tenant_%s_project_%s_", tenantID, projectID)
}

// ValidateSlug checks if a slug is valid (lowercase, alphanumeric, hyphens)
func ValidateSlug(slug string) bool {
	// Only lowercase letters, numbers, and hyphens
	// Must start with a letter
	// No consecutive hyphens
	match := regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`).MatchString(slug)
	return match && !regexp.MustCompile(`--`).MatchString(slug)
}

func projectTemplateVisibilityFilter(tenantOID primitive.ObjectID) bson.M {
	return bson.M{
		"isTemplate": true,
		"isActive":   true,
		"$or": bson.A{
			bson.M{"tenantId": tenantOID, "templateScope": projectTemplateScopeTenant},
			bson.M{"templateScope": projectTemplateScopeGlobal},
		},
	}
}

func projectTemplateVisibleToTenant(project models.Project, tenantOID primitive.ObjectID) bool {
	if !project.IsActive || !project.IsTemplate {
		return false
	}
	if project.TemplateScope == projectTemplateScopeGlobal {
		return true
	}
	return project.TemplateScope == projectTemplateScopeTenant && project.TenantID == tenantOID
}

func projectTemplateUpdateSet(input UpdateProjectTemplateInput, now time.Time) bson.M {
	includeItems := false
	if input.TemplateIncludeItems != nil {
		includeItems = *input.TemplateIncludeItems
	}
	return bson.M{
		"isTemplate":           input.IsTemplate,
		"templateScope":        projectTemplateScopeTenant,
		"templateIncludeItems": includeItems,
		"templateDescription":  input.TemplateDescription,
		"updatedAt":            now,
	}
}

func resolveIncludeTemplateItems(template models.Project, override *bool) bool {
	if override != nil {
		return *override
	}
	return template.TemplateIncludeItems
}

func shouldCloneDynamicRecords(container models.ContainerModel) bool {
	return !container.IsAuthContainer && container.SchemaName != "auth"
}

func isObjectIDField(field models.Field) bool {
	return field.Type == "objectId" || field.Type == "objectid" || field.Type == "reference"
}

func isObjectIDArrayField(field models.Field) bool {
	return field.Type == "objectIdArray" || field.Type == "objectidArray" || field.Type == "referenceArray"
}

func remapCopiedObjectIDReferences(doc bson.M, container models.ContainerModel, idMap projectCloneIDMap) bson.M {
	next := bson.M{}
	for key, value := range doc {
		next[key] = value
	}

	for _, field := range container.Fields {
		if field.ObjectSchemaName == "" {
			continue
		}
		schemaMap, ok := idMap[field.ObjectSchemaName]
		if !ok {
			continue
		}

		value, exists := next[field.Name]
		if !exists {
			continue
		}

		if isObjectIDField(field) {
			if oid, ok := value.(primitive.ObjectID); ok {
				if mapped, found := schemaMap[oid]; found {
					next[field.Name] = mapped
				}
			}
			continue
		}

		if isObjectIDArrayField(field) {
			switch values := value.(type) {
			case bson.A:
				remapped := make(bson.A, len(values))
				for i, item := range values {
					if oid, ok := item.(primitive.ObjectID); ok {
						if mapped, found := schemaMap[oid]; found {
							remapped[i] = mapped
							continue
						}
					}
					remapped[i] = item
				}
				next[field.Name] = remapped
			case []primitive.ObjectID:
				remapped := make([]primitive.ObjectID, len(values))
				for i, oid := range values {
					if mapped, found := schemaMap[oid]; found {
						remapped[i] = mapped
					} else {
						remapped[i] = oid
					}
				}
				next[field.Name] = remapped
			}
		}
	}

	return next
}

// createDefaultSchemas creates the default 'role' and 'auth' schemas for a new project
func createDefaultSchemas(ctx context.Context, tenantID, projectID string) error {
	// Get the containers collection for this project
	containersCollectionName := GetCollectionNameForProject(tenantID, projectID, "containers")
	containersCollection := projectsCollection().Database().Collection(containersCollectionName)

	// 1. Create the 'role' schema first
	log.Println("Creating default 'role' schema for project")
	roleContainer := models.ContainerModel{
		SchemaName: "role",
		Fields: []models.Field{
			{
				Name:  "name",
				Type:  "string",
				Tag:   "",
				Order: 1,
			},
		},
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
			UpdateDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "PATCH"},
			UpdateMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "PATCH"},
			GetDynamicModelItem:                   models.RouteSpec{IsActive: true, Method: "GET"},
			DeleteMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "DELETE"},
			ExportDynamicModelItems:               models.RouteSpec{IsActive: true, Method: "GET"},
			GetItemsForSelection:                  models.RouteSpec{IsActive: true, Method: "GET"},
		},
		Redis:            defaultContainerRedis("role", nil),
		IsAuthContainer:  false,
		PopulatedRoutes:  []string{},
		Pipelines:        []models.PipelineStage{},
		DynamicFunctions: []models.DynamicFunction{},
		Workflows:        []models.DynamicWorkflow{},
		DynamicApis:      []models.DynamicApiModel{},
		Indexes:          []models.Index{},
	}

	if err := utils.EnsureIndexes(ctx, &roleContainer, tenantID, projectID); err != nil {
		return fmt.Errorf("failed to create indexes for role schema: %w", err)
	}
	_, err := containersCollection.InsertOne(ctx, roleContainer)
	if err != nil {
		log.Printf("Failed to create role schema: %v", err)
		return fmt.Errorf("failed to create role schema: %w", err)
	}
	roleCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, "role")
	if _, err := roleCollection.InsertOne(ctx, bson.M{"name": "admin"}); err != nil {
		log.Printf("Failed to create default admin role: %v", err)
		return fmt.Errorf("failed to create default admin role: %w", err)
	}
	log.Println("Role schema successfully created")

	// 2. Create the 'auth' schema with email and role fields
	log.Println("Creating default 'auth' schema for project")
	authContainer := models.ContainerModel{
		SchemaName: "auth",
		Fields: []models.Field{
			{
				Name:              "email",
				Type:              "string",
				Tag:               "required",
				IsLoginCredential: true,
				Unique:            true,
				Order:             1,
			},
			{
				Name:             "role",
				Type:             "objectId",
				Tag:              "required",
				ObjectSchemaName: "role",
				PopulationSettings: &models.PopulationSettings{
					FieldName:           "role",
					PopulatedFields:     []string{"name"},
					DisplayFields:       []string{"name"},
					InputSelectionField: "name",
					DisplayLabel:        "Role",
				},
				Order: 2,
			},
		},
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
			UpdateDynamicModelItem:                models.RouteSpec{IsActive: true, Method: "PATCH"},
			UpdateMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "PATCH"},
			GetDynamicModelItem:                   models.RouteSpec{IsActive: true, Method: "GET"},
			DeleteMultipleDynamicModelItem:        models.RouteSpec{IsActive: true, Method: "DELETE"},
			ExportDynamicModelItems:               models.RouteSpec{IsActive: true, Method: "GET"},
			GetItemsForSelection:                  models.RouteSpec{IsActive: true, Method: "GET"},
		},
		Redis:               defaultContainerRedis("auth", nil),
		IsAuthContainer:     true,
		IsRegisterActive:    true,
		IsGoogleLoginActive: false,
		PopulatedRoutes:     []string{},
		Pipelines:           []models.PipelineStage{},
		DynamicFunctions:    []models.DynamicFunction{},
		Workflows:           []models.DynamicWorkflow{},
		DynamicApis:         []models.DynamicApiModel{},
		Indexes:             []models.Index{},
	}

	if err := utils.EnsureIndexes(ctx, &authContainer, tenantID, projectID); err != nil {
		return fmt.Errorf("failed to create indexes for auth schema: %w", err)
	}
	_, err = containersCollection.InsertOne(ctx, authContainer)
	if err != nil {
		log.Printf("Failed to create auth schema: %v", err)
		return fmt.Errorf("failed to create auth schema: %w", err)
	}
	log.Println("Auth schema successfully created")

	return nil
}

func readProjectContainers(ctx context.Context, tenantID, projectID string) ([]models.ContainerModel, error) {
	cursor, err := utils.GetContainerCollectionForProject(tenantID, projectID).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var containers []models.ContainerModel
	if err := cursor.All(ctx, &containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func insertManyIfAny(ctx context.Context, collection *mongo.Collection, docs []interface{}) error {
	if len(docs) == 0 {
		return nil
	}
	_, err := collection.InsertMany(ctx, docs)
	return err
}

func cloneProjectContainers(ctx context.Context, source models.Project, target models.Project) ([]models.ContainerModel, error) {
	sourceTenantID := source.TenantID.Hex()
	sourceProjectID := source.ID.Hex()
	targetTenantID := target.TenantID.Hex()
	targetProjectID := target.ID.Hex()

	containers, err := readProjectContainers(ctx, sourceTenantID, sourceProjectID)
	if err != nil {
		return nil, err
	}

	targetTenantOID := target.TenantID
	targetProjectOID := target.ID
	docs := make([]interface{}, 0, len(containers))
	for i := range containers {
		containers[i].ID = primitive.NewObjectID()
		containers[i].TenantID = &targetTenantOID
		containers[i].ProjectID = &targetProjectOID
		containers[i].CollectionName = utils.GetProjectCollectionName(targetTenantID, targetProjectID, containers[i].SchemaName)
		docs = append(docs, containers[i])
	}

	if err := insertManyIfAny(ctx, utils.GetContainerCollectionForProject(targetTenantID, targetProjectID), docs); err != nil {
		return nil, err
	}

	for i := range containers {
		if err := utils.EnsureIndexes(ctx, &containers[i], targetTenantID, targetProjectID); err != nil {
			return nil, err
		}
	}

	return containers, nil
}

func cloneProjectPages(ctx context.Context, source models.Project, target models.Project) error {
	sourcePages := utils.GetPageCollectionForProject(source.TenantID.Hex(), source.ID.Hex())
	cursor, err := sourcePages.Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var pages []models.PageModel
	if err := cursor.All(ctx, &pages); err != nil {
		return err
	}

	pageIDMap := map[primitive.ObjectID]primitive.ObjectID{}
	for _, page := range pages {
		if page.ID != primitive.NilObjectID {
			pageIDMap[page.ID] = primitive.NewObjectID()
		}
	}

	docs := make([]interface{}, 0, len(pages))
	for i := range pages {
		if mapped, ok := pageIDMap[pages[i].ID]; ok {
			pages[i].ID = mapped
		} else {
			pages[i].ID = primitive.NewObjectID()
		}
		if pages[i].ParentPageID != nil {
			if mapped, ok := pageIDMap[*pages[i].ParentPageID]; ok {
				pages[i].ParentPageID = &mapped
			}
		}
		docs = append(docs, pages[i])
	}

	return insertManyIfAny(ctx, utils.GetPageCollectionForProject(target.TenantID.Hex(), target.ID.Hex()), docs)
}

func dynamicRecordsForClone(ctx context.Context, tenantID, projectID string, container models.ContainerModel) ([]bson.M, error) {
	if !shouldCloneDynamicRecords(container) {
		return nil, nil
	}
	cursor, err := utils.GetDynamicCollectionForProject(tenantID, projectID, container.SchemaName).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func cloneProjectDynamicItems(ctx context.Context, source models.Project, target models.Project, containers []models.ContainerModel) error {
	sourceTenantID := source.TenantID.Hex()
	sourceProjectID := source.ID.Hex()
	targetTenantID := target.TenantID.Hex()
	targetProjectID := target.ID.Hex()
	idMap := projectCloneIDMap{}
	docsBySchema := map[string][]bson.M{}
	containerBySchema := map[string]models.ContainerModel{}

	for _, container := range containers {
		docs, err := dynamicRecordsForClone(ctx, sourceTenantID, sourceProjectID, container)
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			continue
		}
		containerBySchema[container.SchemaName] = container
		docsBySchema[container.SchemaName] = docs
		idMap[container.SchemaName] = map[primitive.ObjectID]primitive.ObjectID{}
		for _, doc := range docs {
			if oldID, ok := doc["_id"].(primitive.ObjectID); ok {
				idMap[container.SchemaName][oldID] = primitive.NewObjectID()
			}
		}
	}

	for schemaName, docs := range docsBySchema {
		container := containerBySchema[schemaName]
		insertDocs := make([]interface{}, 0, len(docs))
		for _, doc := range docs {
			next := bson.M{}
			for key, value := range doc {
				next[key] = value
			}
			if oldID, ok := next["_id"].(primitive.ObjectID); ok {
				next["_id"] = idMap[schemaName][oldID]
			} else {
				next["_id"] = primitive.NewObjectID()
			}
			next = remapCopiedObjectIDReferences(next, container, idMap)
			insertDocs = append(insertDocs, next)
		}
		if err := insertManyIfAny(ctx, utils.GetDynamicCollectionForProject(targetTenantID, targetProjectID, schemaName), insertDocs); err != nil {
			return err
		}
	}

	return nil
}

func ensureDefaultRoleRecord(ctx context.Context, tenantID, projectID string) error {
	roleCollection := utils.GetDynamicCollectionForProject(tenantID, projectID, "role")
	count, err := roleCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = roleCollection.InsertOne(ctx, bson.M{"name": "admin"})
	return err
}

func cloneProjectFromTemplate(ctx context.Context, source models.Project, target models.Project, includeItems bool) error {
	containers, err := cloneProjectContainers(ctx, source, target)
	if err != nil {
		return err
	}
	if err := cloneProjectPages(ctx, source, target); err != nil {
		return err
	}
	if includeItems {
		if err := cloneProjectDynamicItems(ctx, source, target, containers); err != nil {
			return err
		}
	}
	if err := ensureDefaultRoleRecord(ctx, target.TenantID.Hex(), target.ID.Hex()); err != nil {
		return err
	}
	if err := repositories.NewDynamicRepository().EnsureNotificationIndexes(ctx, target.TenantID.Hex(), target.ID.Hex()); err != nil {
		return err
	}
	return nil
}

func rollbackProjectCreate(ctx context.Context, tenantID string, projectID primitive.ObjectID, schemas []string) {
	projectIDHex := projectID.Hex()
	if _, err := projectMembershipsCollection().DeleteMany(ctx, bson.M{"projectId": projectID}); err != nil {
		log.Printf("Failed to rollback project memberships for project %s: %v", projectIDHex, err)
	}
	if _, err := projectsCollection().DeleteOne(ctx, bson.M{"_id": projectID}); err != nil {
		log.Printf("Failed to rollback project %s: %v", projectIDHex, err)
	}
	for _, resource := range append([]string{"containers", "pages", "role", "auth"}, schemas...) {
		if err := configs.GetCollection(utils.GetProjectCollectionName(tenantID, projectIDHex, resource)).Drop(ctx); err != nil {
			log.Printf("Failed to rollback project collection %s for project %s: %v", resource, projectIDHex, err)
		}
	}
}

func templateProjectSchemas(ctx context.Context, project models.Project) []string {
	containers, err := readProjectContainers(ctx, project.TenantID.Hex(), project.ID.Hex())
	if err != nil {
		return nil
	}
	schemas := make([]string, 0, len(containers))
	for _, container := range containers {
		schemas = append(schemas, container.SchemaName)
	}
	return schemas
}

// CreateProject creates a new project within a tenant
func CreateProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	var input CreateProjectInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Validate input
	if err := utils.ValidateStruct(input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation failed",
			Data:    &fiber.Map{"error": err},
		})
	}

	// Validate slug format
	if !ValidateSlug(input.Slug) {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid slug format. Use lowercase letters, numbers, and hyphens only.",
			Data:    nil,
		})
	}

	// Get user context from middleware
	tenantID := c.Locals("tenantID").(string)
	userID := c.Locals("tenantUserID").(string)

	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
			Data:    nil,
		})
	}

	// Get tenant to retrieve slug
	var tenant models.Tenant
	tenantCollection := configs.GetCollection("tenants")
	err = tenantCollection.FindOne(ctx, bson.M{"_id": tenantOID}).Decode(&tenant)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Tenant not found",
			Data:    nil,
		})
	}

	// Check if slug is unique within tenant
	var existingProject models.Project
	err = projectsCollection().FindOne(ctx, bson.M{
		"tenantId": tenantOID,
		"slug":     input.Slug,
	}).Decode(&existingProject)

	if err == nil {
		return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
			Status:  http.StatusConflict,
			Message: "Project with this slug already exists in this tenant",
			Data:    nil,
		})
	}
	if err != mongo.ErrNoDocuments {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to validate project slug",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	var templateProject *models.Project
	includeTemplateItems := false
	if input.TemplateProjectID != "" {
		templateOID, err := primitive.ObjectIDFromHex(input.TemplateProjectID)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
				Status:  http.StatusBadRequest,
				Message: "Invalid template project ID",
				Data:    nil,
			})
		}
		var foundTemplate models.Project
		if err := projectsCollection().FindOne(ctx, bson.M{"_id": templateOID, "isTemplate": true}).Decode(&foundTemplate); err != nil {
			if err == mongo.ErrNoDocuments {
				return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
					Status:  http.StatusNotFound,
					Message: "Template project not found",
					Data:    nil,
				})
			}
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch template project",
				Data:    &fiber.Map{"error": err.Error()},
			})
		}
		if !projectTemplateVisibleToTenant(foundTemplate, tenantOID) {
			return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
				Status:  http.StatusForbidden,
				Message: "Template project is not available to this tenant",
				Data:    nil,
			})
		}
		templateProject = &foundTemplate
		includeTemplateItems = resolveIncludeTemplateItems(foundTemplate, input.IncludeTemplateItems)
	}

	// Create project
	newProject := models.Project{
		ID:         primitive.NewObjectID(),
		TenantID:   tenantOID,
		TenantSlug: tenant.Slug,
		Name:       input.Name,
		Slug:       input.Slug,
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err = projectsCollection().InsertOne(ctx, newProject)
	if err != nil {
		log.Printf("Failed to create project: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Create project membership for the creator (project admin)
	projectMembership := models.ProjectMembership{
		ID:        primitive.NewObjectID(),
		TenantID:  tenantOID,
		ProjectID: newProject.ID,
		UserID:    userOID,
		Roles:     []string{models.ProjectRoleAdmin},
		Status:    models.MembershipStatusActive,
		CreatedBy: userOID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = projectMembershipsCollection().InsertOne(ctx, projectMembership)
	if err != nil {
		// Rollback: delete the project
		if _, rollbackErr := projectsCollection().DeleteOne(ctx, bson.M{"_id": newProject.ID}); rollbackErr != nil {
			log.Printf("Failed to rollback project creation after membership error: %v", rollbackErr)
		}
		log.Printf("Failed to create project membership: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create project membership",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Initialize the containers collection for this project
	// This creates the collection name pattern that will be used
	containersCollectionName := GetCollectionNameForProject(
		tenantID,
		newProject.ID.Hex(),
		"containers",
	)

	// Create the containers metadata collection with a unique index on schemaName
	containersCol := projectsCollection().Database().Collection(containersCollectionName)

	// Create unique index on schemaName within this project
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "schemaName", Value: 1}},
		Options: options.Index().SetUnique(true),
	}

	_, err = containersCol.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("Warning: Failed to create index on containers collection: %v", err)
		// Don't fail the project creation, just log it
	}

	if templateProject != nil {
		err = cloneProjectFromTemplate(ctx, *templateProject, newProject, includeTemplateItems)
		if err != nil {
			log.Printf("Failed to clone project from template: %v", err)
			rollbackProjectCreate(ctx, tenantID, newProject.ID, templateProjectSchemas(ctx, *templateProject))
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create project from template",
				Data:    &fiber.Map{"error": err.Error()},
			})
		}
	} else {
		// Create default schemas (role and auth)
		err = createDefaultSchemas(ctx, tenantID, newProject.ID.Hex())
		if err != nil {
			log.Printf("Warning: Failed to create default schemas: %v", err)
			// Don't fail the project creation, just log the warning
			// The schemas can be created manually later if needed
		}
		if err := repositories.NewDynamicRepository().EnsureNotificationIndexes(ctx, tenantID, newProject.ID.Hex()); err != nil {
			log.Printf("Warning: Failed to create notification indexes: %v", err)
		}
	}

	email, _ := c.Locals("email").(string)
	tokens, err := utils.GenerateTenantTokens(
		userID,
		email,
		tenantID,
		newProject.ID.Hex(),
		projectMembership.Roles,
		string(models.RoleScopeProject),
	)
	if err != nil {
		log.Printf("Failed to generate project tokens after project creation: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Project created, but failed to generate project tokens",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Project created successfully",
		Data: &fiber.Map{
			"project":              newProject,
			"membership":           projectMembership,
			"containersCollection": containersCollectionName,
			"accessToken":          tokens.AccessToken,
			"refreshToken":         tokens.RefreshToken,
		},
	})
}

// GetAllProjects lists all projects in the tenant
func GetAllProjects(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	// Get tenant from context
	tenantID := c.Locals("tenantID").(string)
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	// Find all projects for this tenant
	cursor, err := projectsCollection().Find(ctx, bson.M{
		"tenantId": tenantOID,
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch projects",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	defer cursor.Close(ctx)

	var projects []models.Project
	if err = cursor.All(ctx, &projects); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode projects",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Projects retrieved successfully",
		Data:    &fiber.Map{"projects": projects},
	})
}

func GetProjectTemplates(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	tenantID := c.Locals("tenantID").(string)
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	cursor, err := projectsCollection().Find(ctx, projectTemplateVisibilityFilter(tenantOID))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch project templates",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	defer cursor.Close(ctx)

	var templates []models.Project
	if err := cursor.All(ctx, &templates); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode project templates",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Project templates retrieved successfully",
		Data:    &fiber.Map{"templates": templates},
	})
}

func UpdateProjectTemplate(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	projectOID, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid project ID",
			Data:    nil,
		})
	}

	tenantID := c.Locals("tenantID").(string)
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	var input UpdateProjectTemplateInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	result, err := projectsCollection().UpdateOne(
		ctx,
		bson.M{"_id": projectOID, "tenantId": tenantOID},
		bson.M{"$set": projectTemplateUpdateSet(input, time.Now())},
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update project template",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	if result.MatchedCount == 0 {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Project not found",
			Data:    nil,
		})
	}

	var project models.Project
	if err := projectsCollection().FindOne(ctx, bson.M{"_id": projectOID, "tenantId": tenantOID}).Decode(&project); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch updated project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Project template updated successfully",
		Data:    &fiber.Map{"project": project},
	})
}

// GetProject retrieves a single project by ID
func GetProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	projectID := c.Params("id")
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid project ID",
			Data:    nil,
		})
	}

	// Get tenant from context
	tenantID := c.Locals("tenantID").(string)
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	// Find project and verify it belongs to the tenant
	var project models.Project
	err = projectsCollection().FindOne(ctx, bson.M{
		"_id":      projectOID,
		"tenantId": tenantOID,
	}).Decode(&project)

	if err == mongo.ErrNoDocuments {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Project not found",
			Data:    nil,
		})
	}

	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Get project members count
	memberCount, err := projectMembershipsCollection().CountDocuments(ctx, bson.M{
		"projectId": projectOID,
		"status":    models.MembershipStatusActive,
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count project members",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Project retrieved successfully",
		Data: &fiber.Map{
			"project":     project,
			"memberCount": memberCount,
		},
	})
}

// UpdateProject updates a project's details
func UpdateProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	projectID := c.Params("id")
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid project ID",
			Data:    nil,
		})
	}

	var input UpdateProjectInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Validate input
	if err := utils.ValidateStruct(input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation failed",
			Data:    &fiber.Map{"error": err},
		})
	}

	// Validate slug if provided
	if input.Slug != nil && !ValidateSlug(*input.Slug) {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid slug format",
			Data:    nil,
		})
	}

	// Get tenant from context
	tenantID := c.Locals("tenantID").(string)
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	// Check if project exists and belongs to tenant
	var existingProject models.Project
	err = projectsCollection().FindOne(ctx, bson.M{
		"_id":      projectOID,
		"tenantId": tenantOID,
	}).Decode(&existingProject)

	if err == mongo.ErrNoDocuments {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Project not found",
			Data:    nil,
		})
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Build update document
	updateDoc := bson.M{
		"updatedAt": time.Now(),
	}

	if input.Name != nil {
		updateDoc["name"] = *input.Name
	}

	if input.Slug != nil {
		// Check if new slug is unique
		var slugCheck models.Project
		err = projectsCollection().FindOne(ctx, bson.M{
			"tenantId": tenantOID,
			"slug":     *input.Slug,
			"_id":      bson.M{"$ne": projectOID},
		}).Decode(&slugCheck)

		if err == nil {
			return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
				Status:  http.StatusConflict,
				Message: "Another project with this slug already exists",
				Data:    nil,
			})
		}
		if err != mongo.ErrNoDocuments {
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Failed to validate project slug",
				Data:    &fiber.Map{"error": err.Error()},
			})
		}

		updateDoc["slug"] = *input.Slug

		// When slug changes, we should also update tenantSlug cache key in Redis
		// The old cache entry will expire naturally after 1 hour
	}

	if input.IsActive != nil {
		updateDoc["isActive"] = *input.IsActive
	}

	// Update project
	result, err := projectsCollection().UpdateOne(
		ctx,
		bson.M{
			"_id":      projectOID,
			"tenantId": tenantOID,
		},
		bson.M{"$set": updateDoc},
	)

	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	if result.ModifiedCount == 0 {
		return c.Status(http.StatusNotModified).JSON(responses.GeneralResponse{
			Status:  http.StatusNotModified,
			Message: "No changes made",
			Data:    nil,
		})
	}

	// Fetch updated project
	var updatedProject models.Project
	if err = projectsCollection().FindOne(ctx, bson.M{
		"_id":      projectOID,
		"tenantId": tenantOID,
	}).Decode(&updatedProject); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch updated project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Project updated successfully",
		Data:    &fiber.Map{"project": updatedProject},
	})
}

// DeleteProject deletes a project (admin only)
func DeleteProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()

	projectID := c.Params("id")
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid project ID",
			Data:    nil,
		})
	}

	// Get tenant from context
	tenantID := c.Locals("tenantID").(string)
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid tenant ID",
			Data:    nil,
		})
	}

	// Check if project exists
	var project models.Project
	err = projectsCollection().FindOne(ctx, bson.M{
		"_id":      projectOID,
		"tenantId": tenantOID,
	}).Decode(&project)

	if err == mongo.ErrNoDocuments {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Project not found",
			Data:    nil,
		})
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Delete project memberships
	if _, err = projectMembershipsCollection().DeleteMany(ctx, bson.M{
		"projectId": projectOID,
		"tenantId":  tenantOID,
	}); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete project memberships",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Delete project
	_, err = projectsCollection().DeleteOne(ctx, bson.M{
		"_id":      projectOID,
		"tenantId": tenantOID,
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete project",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Note: We don't automatically delete all collections for safety
	// You may want to add a background job or manual cleanup process

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Project deleted successfully",
		Data: &fiber.Map{
			"warning": "Project collections were not automatically deleted. Contact admin for cleanup if needed.",
		},
	})
}
