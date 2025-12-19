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
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CreateProjectInput represents the project creation payload
type CreateProjectInput struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
	Slug string `json:"slug" validate:"required,min=2,max=50"`
}

// UpdateProjectInput represents the project update payload
type UpdateProjectInput struct {
	Name     *string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Slug     *string `json:"slug,omitempty" validate:"omitempty,min=2,max=50"`
	IsActive *bool   `json:"isActive,omitempty"`
}

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
	match, _ := regexp.MatchString(`^[a-z][a-z0-9-]*[a-z0-9]$`, slug)
	return match && !regexp.MustCompile(`--`).MatchString(slug)
}

// CreateProject creates a new project within a tenant
func CreateProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	// Check if slug is unique within tenant
	var existingProject models.Project
	err = projectsCollection.FindOne(ctx, bson.M{
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

	// Create project
	newProject := models.Project{
		ID:        primitive.NewObjectID(),
		TenantID:  tenantOID,
		Name:      input.Name,
		Slug:      input.Slug,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = projectsCollection.InsertOne(ctx, newProject)
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

	_, err = projectMembershipsCollection.InsertOne(ctx, projectMembership)
	if err != nil {
		// Rollback: delete the project
		projectsCollection.DeleteOne(ctx, bson.M{"_id": newProject.ID})
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
	db := configs.ConnectDB().Database(projectsCollection.Database().Name())
	containersCol := db.Collection(containersCollectionName)

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

	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Project created successfully",
		Data: &fiber.Map{
			"project":                newProject,
			"membership":             projectMembership,
			"containersCollection":   containersCollectionName,
		},
	})
}

// GetAllProjects lists all projects in the tenant
func GetAllProjects(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	cursor, err := projectsCollection.Find(ctx, bson.M{
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

// GetProject retrieves a single project by ID
func GetProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	tenantOID, _ := primitive.ObjectIDFromHex(tenantID)

	// Find project and verify it belongs to the tenant
	var project models.Project
	err = projectsCollection.FindOne(ctx, bson.M{
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
	memberCount, _ := projectMembershipsCollection.CountDocuments(ctx, bson.M{
		"projectId": projectOID,
		"status":    models.MembershipStatusActive,
	})

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	tenantOID, _ := primitive.ObjectIDFromHex(tenantID)

	// Check if project exists and belongs to tenant
	var existingProject models.Project
	err = projectsCollection.FindOne(ctx, bson.M{
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
		err = projectsCollection.FindOne(ctx, bson.M{
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

		updateDoc["slug"] = *input.Slug
	}

	if input.IsActive != nil {
		updateDoc["isActive"] = *input.IsActive
	}

	// Update project
	result, err := projectsCollection.UpdateOne(
		ctx,
		bson.M{"_id": projectOID},
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
	projectsCollection.FindOne(ctx, bson.M{"_id": projectOID}).Decode(&updatedProject)

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Project updated successfully",
		Data:    &fiber.Map{"project": updatedProject},
	})
}

// DeleteProject deletes a project (admin only)
func DeleteProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	tenantOID, _ := primitive.ObjectIDFromHex(tenantID)

	// Check if project exists
	var project models.Project
	err = projectsCollection.FindOne(ctx, bson.M{
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

	// Delete project memberships
	projectMembershipsCollection.DeleteMany(ctx, bson.M{"projectId": projectOID})

	// Delete project
	_, err = projectsCollection.DeleteOne(ctx, bson.M{"_id": projectOID})
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
		Data:    &fiber.Map{
			"warning": "Project collections were not automatically deleted. Contact admin for cleanup if needed.",
		},
	})
}
