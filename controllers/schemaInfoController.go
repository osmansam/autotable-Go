package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
)

// GetRoleItems retrieves all items from the "role" schema collection
// This route uses project-scope authentication with role validation
// Users can only access role items within their project
func GetRoleItems(c *fiber.Ctx) error {
	log.Println("GetRoleItems endpoint called")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user info from context (set by TenantAuthenticate middleware)
	roles := c.Locals("roles").([]string)
	tenantID := c.Locals("tenantID").(string)
	projectID := c.Locals("projectID").(string)

	// Hardcoded to only retrieve items from "role" schema
	schemaName := "role"

	// Get the collection for the "role" schema within the project scope
	collection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	// Fetch all items from the role collection
	var roleItems []map[string]interface{}
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Failed to fetch role items, error: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve role items.",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	defer cursor.Close(ctx)

	if err = cursor.All(ctx, &roleItems); err != nil {
		log.Printf("Failed to decode role items, error: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode role items.",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	log.Printf("User with roles %v accessed role items, found %d items", roles, len(roleItems))

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Role items retrieved successfully.",
		Data:    &fiber.Map{"items": roleItems},
	})
}

// GetRoleItemById retrieves a single item from the "role" schema by ID
// This route uses project-scope authentication with role validation
func GetRoleItemById(c *fiber.Ctx) error {
	log.Println("GetRoleItemById endpoint called")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user info from context (set by TenantAuthenticate middleware)
	roles := c.Locals("roles").([]string)
	tenantID := c.Locals("tenantID").(string)
	projectID := c.Locals("projectID").(string)

	// Get ID from URL parameter
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing role item ID.",
		})
	}

	// Hardcoded to only retrieve items from "role" schema
	schemaName := "role"

	// Get the collection for the "role" schema within the project scope
	collection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)

	// Fetch the specific role item
	var roleItem map[string]interface{}
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&roleItem)
	if err != nil {
		log.Printf("Failed to fetch role item with id %s, error: %v", id, err)
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Role item not found.",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	log.Printf("User with roles %v accessed role item with id %s", roles, id)

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Role item retrieved successfully.",
		Data:    &fiber.Map{"item": roleItem},
	})
}
