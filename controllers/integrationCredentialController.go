package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func integrationCredentialsCollection() *mongo.Collection {
	return configs.GetCollection("integration_credentials")
}

type CreateIntegrationCredentialInput struct {
	Name        string                         `json:"name" validate:"required"`
	Permissions []models.IntegrationPermission `json:"permissions" validate:"required"`
	ExpiresAt   time.Time                      `json:"expiresAt" validate:"required"`
}

func CreateIntegrationCredential(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil || tenantID == "" || projectID == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing tenant or project context",
			Data:    &fiber.Map{"error": errString(err)},
		})
	}

	var input CreateIntegrationCredentialInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	if err := validateIntegrationCredentialInput(input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation failed",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	tenantObjID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid tenant ID"})
	}
	projectObjID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid project ID"})
	}

	token, err := utils.GenerateIntegrationToken()
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate integration token",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	now := time.Now()
	credential := models.IntegrationCredential{
		ID:          primitive.NewObjectID(),
		TenantID:    tenantObjID,
		ProjectID:   projectObjID,
		Name:        strings.TrimSpace(input.Name),
		TokenHash:   utils.HashIntegrationToken(token),
		Permissions: input.Permissions,
		ExpiresAt:   input.ExpiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if userID, ok := c.Locals("tenantUserID").(string); ok && userID != "" {
		if createdBy, err := primitive.ObjectIDFromHex(userID); err == nil {
			credential.CreatedBy = createdBy
		}
	}

	if _, err := integrationCredentialsCollection().InsertOne(ctx, credential); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create integration credential",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Integration credential created",
		Data: &fiber.Map{
			"credential": credential,
			"token":      token,
		},
	})
}

func ListIntegrationCredentials(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil || tenantID == "" || projectID == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing tenant or project context",
			Data:    &fiber.Map{"error": errString(err)},
		})
	}
	tenantObjID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid tenant ID"})
	}
	projectObjID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid project ID"})
	}

	cursor, err := integrationCredentialsCollection().Find(ctx, bson.M{"tenantId": tenantObjID, "projectId": projectObjID})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to list integration credentials",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	defer cursor.Close(ctx)

	credentials := make([]models.IntegrationCredential, 0)
	if err := cursor.All(ctx, &credentials); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode integration credentials",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Integration credentials retrieved",
		Data:    &fiber.Map{"credentials": credentials},
	})
}

func RevokeIntegrationCredential(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil || tenantID == "" || projectID == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing tenant or project context",
			Data:    &fiber.Map{"error": errString(err)},
		})
	}
	tenantObjID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid tenant ID"})
	}
	projectObjID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid project ID"})
	}
	credentialID, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid credential ID"})
	}

	now := time.Now()
	result, err := integrationCredentialsCollection().UpdateOne(ctx,
		bson.M{"_id": credentialID, "tenantId": tenantObjID, "projectId": projectObjID},
		bson.M{"$set": bson.M{"revokedAt": now, "updatedAt": now}},
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to revoke integration credential",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	if result.MatchedCount == 0 {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Integration credential not found",
		})
	}

	return c.JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Integration credential revoked",
	})
}

func validateIntegrationCredentialInput(input CreateIntegrationCredentialInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fiber.NewError(http.StatusBadRequest, "name is required")
	}
	if len(input.Permissions) == 0 {
		return fiber.NewError(http.StatusBadRequest, "permissions are required")
	}
	if !input.ExpiresAt.After(time.Now()) {
		return fiber.NewError(http.StatusBadRequest, "expiresAt must be in the future")
	}
	for _, permission := range input.Permissions {
		if !isValidIntegrationPermission(permission) {
			return fiber.NewError(http.StatusBadRequest, "invalid integration permission")
		}
	}
	return nil
}

func isValidIntegrationPermission(permission models.IntegrationPermission) bool {
	switch strings.TrimSpace(permission.Kind) {
	case models.IntegrationPermissionKindDynamicRoute:
		return strings.TrimSpace(permission.SchemaName) != "" && strings.TrimSpace(permission.Route) != "" && strings.TrimSpace(permission.Method) != ""
	case models.IntegrationPermissionKindWorkflow, models.IntegrationPermissionKindAPI, models.IntegrationPermissionKindPipeline:
		return strings.TrimSpace(permission.SchemaName) != "" && strings.TrimSpace(permission.Name) != "" && strings.TrimSpace(permission.Method) != ""
	default:
		return false
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
