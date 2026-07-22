package controllers

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CreateExternalAPICredentialInput struct {
	Name           string    `json:"name" validate:"required"`
	AuthType       string    `json:"authType" validate:"required"`
	HeaderName     string    `json:"headerName,omitempty"`
	Secret         string    `json:"secret" validate:"required"`
	AllowedDomains []string  `json:"allowedDomains" validate:"required"`
	ExpiresAt      time.Time `json:"expiresAt,omitempty"`
}

type externalAPICredentialValidation struct {
	Name           string
	AuthType       string
	HeaderName     string
	Secret         string
	AllowedDomains []string
	ExpiresAt      time.Time
}

func CreateExternalAPICredential(c *fiber.Ctx) error {
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

	var input CreateExternalAPICredentialInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	normalized, err := validateExternalAPICredentialInput(input, time.Now())
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation failed",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	key := os.Getenv("EXTERNAL_API_CREDENTIAL_KEY")
	if err := utils.ValidateExternalAPIEncryptionKey(key); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "External API credential encryption is not configured",
		})
	}
	encryptedSecret, err := utils.EncryptExternalSecret(normalized.Secret, key)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to encrypt external API credential",
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

	now := time.Now()
	credential := models.ExternalAPICredential{
		ID:              primitive.NewObjectID(),
		TenantID:        tenantObjID,
		ProjectID:       projectObjID,
		Name:            normalized.Name,
		AuthType:        normalized.AuthType,
		HeaderName:      normalized.HeaderName,
		EncryptedSecret: encryptedSecret,
		AllowedDomains:  normalized.AllowedDomains,
		ExpiresAt:       normalized.ExpiresAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if userID, ok := c.Locals("tenantUserID").(string); ok && userID != "" {
		if createdBy, err := primitive.ObjectIDFromHex(userID); err == nil {
			credential.CreatedBy = createdBy
		}
	}

	repository := repositories.NewDynamicRepository()
	if _, err := repository.InsertExternalAPICredential(ctx, credential); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create external API credential",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "External API credential created",
		Data:    &fiber.Map{"credential": externalAPICredentialResponse(credential)},
	})
}

func ListExternalAPICredentials(c *fiber.Ctx) error {
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
	repository := repositories.NewDynamicRepository()
	credentials, err := repository.ListExternalAPICredentials(ctx, tenantID, projectID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to list external API credentials",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	result := make([]models.ExternalAPICredential, 0, len(credentials))
	for _, credential := range credentials {
		result = append(result, externalAPICredentialResponse(credential))
	}
	return c.JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "External API credentials retrieved",
		Data:    &fiber.Map{"credentials": result},
	})
}

func RevokeExternalAPICredential(c *fiber.Ctx) error {
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
	result, err := repositories.NewDynamicRepository().RevokeExternalAPICredential(ctx, tenantID, projectID, c.Params("id"), time.Now())
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Failed to revoke external API credential",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	if result.MatchedCount == 0 {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{Status: http.StatusNotFound, Message: "External API credential not found"})
	}
	return c.JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "External API credential revoked"})
}

func validateExternalAPICredentialInput(input CreateExternalAPICredentialInput, now time.Time) (externalAPICredentialValidation, error) {
	normalized := externalAPICredentialValidation{
		Name:           strings.TrimSpace(input.Name),
		AuthType:       strings.ToLower(strings.TrimSpace(input.AuthType)),
		HeaderName:     strings.TrimSpace(input.HeaderName),
		Secret:         strings.TrimSpace(input.Secret),
		AllowedDomains: make([]string, 0, len(input.AllowedDomains)),
		ExpiresAt:      input.ExpiresAt,
	}
	if normalized.Name == "" {
		return normalized, fiber.NewError(http.StatusBadRequest, "name is required")
	}
	if normalized.AuthType == "" {
		normalized.AuthType = models.ExternalAPIAuthTypeBearer
	}
	switch normalized.AuthType {
	case models.ExternalAPIAuthTypeBearer:
		normalized.HeaderName = ""
	case models.ExternalAPIAuthTypeHeader:
		if normalized.HeaderName == "" {
			return normalized, fiber.NewError(http.StatusBadRequest, "headerName is required for header auth")
		}
		if strings.EqualFold(normalized.HeaderName, "Authorization") {
			return normalized, fiber.NewError(http.StatusBadRequest, "custom header auth cannot use Authorization")
		}
	default:
		return normalized, fiber.NewError(http.StatusBadRequest, "authType must be bearer or header")
	}
	if normalized.Secret == "" {
		return normalized, fiber.NewError(http.StatusBadRequest, "secret is required")
	}
	if len(input.AllowedDomains) == 0 {
		return normalized, fiber.NewError(http.StatusBadRequest, "allowedDomains are required")
	}
	if !input.ExpiresAt.IsZero() && !input.ExpiresAt.After(now) {
		return normalized, fiber.NewError(http.StatusBadRequest, "expiresAt must be in the future")
	}
	seen := map[string]bool{}
	for _, domain := range input.AllowedDomains {
		host, err := normalizeExternalAPICredentialDomain(domain)
		if err != nil {
			return normalized, err
		}
		if !seen[host] {
			normalized.AllowedDomains = append(normalized.AllowedDomains, host)
			seen[host] = true
		}
	}
	if len(normalized.AllowedDomains) == 0 {
		return normalized, fiber.NewError(http.StatusBadRequest, "allowedDomains are required")
	}
	return normalized, nil
}

func externalAPICredentialResponse(credential models.ExternalAPICredential) models.ExternalAPICredential {
	credential.EncryptedSecret = ""
	return credential
}

func normalizeExternalAPICredentialDomain(rawDomain string) (string, error) {
	rawDomain = strings.TrimSpace(rawDomain)
	if rawDomain == "" {
		return "", fiber.NewError(http.StatusBadRequest, "allowed domain is required")
	}
	toParse := rawDomain
	if !strings.Contains(toParse, "://") {
		toParse = "//" + toParse
	}
	parsed, err := url.Parse(toParse)
	if err != nil {
		return "", fiber.NewError(http.StatusBadRequest, "invalid allowed domain")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return "", fiber.NewError(http.StatusBadRequest, "invalid allowed domain")
	}
	return host, nil
}
