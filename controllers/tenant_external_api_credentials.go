package controllers

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

func ListTenantExternalAPICredentials(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	tenant, _ := c.Locals("tenantID").(string)
	credentials, err := repositories.NewDynamicRepository().ListTenantExternalAPICredentials(ctx, tenant)
	if err != nil {
		return tenantCredentialError(c, err)
	}
	for i := range credentials {
		credentials[i] = externalAPICredentialResponse(credentials[i])
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "External API credentials retrieved", Data: &fiber.Map{"credentials": credentials}})
}
func tenantCredentialError(c *fiber.Ctx, err error) error {
	status := 400
	if errors.Is(err, mongo.ErrNoDocuments) {
		status = 404
	}
	return c.Status(status).JSON(responses.GeneralResponse{Status: status, Message: "External API credential operation failed", Data: &fiber.Map{"error": err.Error()}})
}
func tenantCredentialMutation(c *fiber.Ctx, operation string) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	tenant, _ := c.Locals("tenantID").(string)
	user, _ := c.Locals("tenantUserID").(string)
	grantor, _ := primitive.ObjectIDFromHex(user)
	repo := repositories.NewDynamicRepository()
	var err error
	switch operation {
	case "promote":
		err = repo.PromoteExternalAPICredential(ctx, tenant, c.Params("id"), grantor)
	case "grant":
		err = repo.GrantExternalAPICredential(ctx, tenant, c.Params("id"), c.Params("projectId"), grantor)
	case "disconnect":
		var credential *models.ExternalAPICredential
		credential, err = repo.GetTenantExternalAPICredential(ctx, tenant, c.Params("id"))
		if err == nil && credential.EffectiveScope() != models.ExternalAPICredentialScopeTenant {
			err = fiber.NewError(400, "tenant credential required")
		}
		if err == nil {
			err = repo.DisconnectExternalAPICredential(ctx, tenant, c.Params("id"), c.Params("projectId"))
		}
	case "revoke":
		var result *mongo.UpdateResult
		result, err = repo.RevokeTenantExternalAPICredential(ctx, tenant, c.Params("id"), time.Now())
		if err == nil && result.MatchedCount == 0 {
			err = mongo.ErrNoDocuments
		}
	}
	if err != nil {
		return tenantCredentialError(c, err)
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "External API credential updated"})
}
func PromoteExternalAPICredential(c *fiber.Ctx) error { return tenantCredentialMutation(c, "promote") }
func GrantExternalAPICredential(c *fiber.Ctx) error   { return tenantCredentialMutation(c, "grant") }
func DisconnectExternalAPICredential(c *fiber.Ctx) error {
	return tenantCredentialMutation(c, "disconnect")
}
func RevokeTenantExternalAPICredential(c *fiber.Ctx) error {
	return tenantCredentialMutation(c, "revoke")
}
