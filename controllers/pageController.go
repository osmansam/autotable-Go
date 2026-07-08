package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// getProjectContext extracts tenant and project IDs from the request context
func getPageProjectContext(c *fiber.Ctx) (tenantID, projectID string, err error) {
	tenantID, projectID, err = utils.GetTenantAndProjectContext(c)
	if err != nil {
		return "", "", err
	}
	if tenantID == "" || projectID == "" {
		return "", "", fmt.Errorf("missing tenant or project context")
	}
	return tenantID, projectID, nil
}

// CreatePage creates a new page in project-specific collection
func CreatePage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get project context
	tenantID, projectID, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	var page models.PageModel

	log.Println("Parsing request body for CreatePage")
	if err := c.BodyParser(&page); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to parse the request body. Ensure the provided JSON is valid.")
	}

	log.Println("Validating parsed data for CreatePage")
	if validationErr := utils.ValidateStruct(page); validationErr != nil {
		log.Printf("Validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Some required fields might be missing or have invalid values.")
	}
	if validationErr := models.ValidatePageTableConfig(&page); validationErr != nil {
		log.Printf("Table config validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Table component configuration contains invalid values.")
	}
	if validationErr := models.ValidatePageRuntimeConfig(&page); validationErr != nil {
		return utils.SendErrorResponse(c, validationErr, "Validation error. Page runtime bindings are invalid.")
	}

	// Get project-specific pages collection
	pageCollection := utils.GetPageCollectionForProject(tenantID, projectID)

	log.Println("Inserting new page into the database")
	result, err := pageCollection.InsertOne(ctx, page)
	if err != nil {
		log.Printf("Failed to insert page: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to insert the page into the database. Please try again later.")
	}

	// Emit WebSocket event for page change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitPageChanged(userIDStr, tenantID, projectID)

	log.Println("Page successfully created")
	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Page successfully created.",
		Data:    &fiber.Map{"data": result},
	})
}

// GetAllPages retrieves all pages from project-specific collection
func GetAllPages(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get project context
	tenantID, projectID, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	var pages []models.PageModel

	// Get project-specific pages collection
	pageCollection := utils.GetPageCollectionForProject(tenantID, projectID)

	log.Println("Retrieving all pages from the database")
	results, err := pageCollection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Failed to retrieve pages: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to retrieve pages from the database. Please try again later.")
	}
	defer results.Close(ctx)

	for results.Next(ctx) {
		var singlePage models.PageModel
		if err = results.Decode(&singlePage); err != nil {
			log.Printf("Error decoding page: %v", err)
			return utils.SendErrorResponse(c, err, "An error occurred while processing the retrieved pages. Please try again later.")
		}

		pages = append(pages, singlePage)
	}

	if err = results.Err(); err != nil {
		log.Printf("Cursor error: %v", err)
		return utils.SendErrorResponse(c, err, "An error occurred while processing the retrieved pages. Please try again later.")
	}

	log.Println("Pages successfully retrieved")
	return c.JSON(pages)
}

// GetAllPagesPublic retrieves all pages with conditional authentication
// This allows public access to pages based on their IsAuthenticated settings
func GetAllPagesPublic(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get project context
	tenantID, projectID, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	var pages []models.PageModel

	// Get project-specific pages collection
	pageCollection := utils.GetPageCollectionForProject(tenantID, projectID)

	log.Println("Retrieving all pages from the database (public access)")
	results, err := pageCollection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Failed to retrieve pages: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to retrieve pages from the database. Please try again later.")
	}
	defer results.Close(ctx)

	// Get user role from context (may be empty if not authenticated)
	userRole, _ := c.Locals("userRole").(string)
	userID, _ := c.Locals("userID").(string)

	for results.Next(ctx) {
		var singlePage models.PageModel
		if err = results.Decode(&singlePage); err != nil {
			log.Printf("Error decoding page: %v", err)
			return utils.SendErrorResponse(c, err, "An error occurred while processing the retrieved pages. Please try again later.")
		}

		// Filter pages based on authentication/authorization settings
		if singlePage.IsAuthenticated {
			// Page requires authentication - check if user is logged in
			if userID == "" {
				continue
			}
			// User is authenticated - now check if specific role authorization is required
			if singlePage.IsAuthorized {
				// Check if user has one of the authorized roles
				isAuthorized := false
				for _, allowedRole := range singlePage.AuthorizeRole {
					if allowedRole == userRole {
						isAuthorized = true
						break
					}
				}
				if !isAuthorized {
					continue
				}
			}
		}

		pages = append(pages, singlePage)
	}

	if err = results.Err(); err != nil {
		log.Printf("Cursor error: %v", err)
		return utils.SendErrorResponse(c, err, "An error occurred while processing the retrieved pages. Please try again later.")
	}

	log.Println("Pages successfully retrieved (public access)")
	return c.JSON(pages)
}

// GetPage retrieves a single page from project-specific collection based on its ID
func GetPage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get project context
	tenantID, projectID, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	pageIdStr := c.Params("id")
	pageId, err := primitive.ObjectIDFromHex(pageIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

	// Get project-specific pages collection
	pageCollection := utils.GetPageCollectionForProject(tenantID, projectID)

	log.Println("Fetching page from the database")
	var page models.PageModel
	err = pageCollection.FindOne(ctx, bson.M{"_id": pageId}).Decode(&page)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Println("No page found with the specified ID")
			return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
				Status:  http.StatusNotFound,
				Message: "No page found with the specified ID.",
				Data:    nil,
			})
		}
		log.Printf("Failed to retrieve page: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to retrieve the page from the database. Please try again later.")
	}

	log.Println("Page successfully retrieved")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Page successfully retrieved.",
		Data:    &fiber.Map{"data": page},
	})
}

// UpdatePage updates an existing page in project-specific collection
func UpdatePage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get project context
	tenantID, projectID, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	var updatedPage models.PageModel

	log.Println("Parsing request body for UpdatePage")
	if err := c.BodyParser(&updatedPage); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to parse the request body. Ensure the provided JSON is valid.")
	}
	log.Printf("%+v", updatedPage)
	log.Println("Validating parsed data for UpdatePage")
	if validationErr := utils.ValidateStruct(updatedPage); validationErr != nil {
		log.Printf("Validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Some required fields might be missing or have invalid values.")
	}
	if validationErr := models.ValidatePageTableConfig(&updatedPage); validationErr != nil {
		log.Printf("Table config validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Table component configuration contains invalid values.")
	}
	if validationErr := models.ValidatePageRuntimeConfig(&updatedPage); validationErr != nil {
		return utils.SendErrorResponse(c, validationErr, "Validation error. Page runtime bindings are invalid.")
	}

	updateIdStr := c.Params("id")
	updateId, err := primitive.ObjectIDFromHex(updateIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

	// Get project-specific pages collection
	pageCollection := utils.GetPageCollectionForProject(tenantID, projectID)

	log.Println("Updating page in the database")
	updateResult, err := pageCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": updatedPage})
	if err != nil {
		log.Printf("Failed to update page: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to update the page in the database. Please try again later.")
	}

	if updateResult.MatchedCount == 0 {
		log.Println("No page found with the specified ID")
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "No page found with the specified ID.",
			Data:    nil,
		})
	}

	// Emit WebSocket event for page change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitPageChanged(userIDStr, tenantID, projectID)

	log.Println("Page successfully updated")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Page successfully updated.",
		Data:    &fiber.Map{"data": updateResult},
	})
}

// DeletePage deletes a page from project-specific collection
func DeletePage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get project context
	tenantID, projectID, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Invalid ID format provided. Please ensure the ID is a valid MongoDB ObjectID.")
	}

	// Get project-specific pages collection
	pageCollection := utils.GetPageCollectionForProject(tenantID, projectID)

	log.Println("Attempting to delete page from the database")
	result, err := pageCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		log.Printf("Failed to delete page: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to delete the page from the database. Please try again later.")
	}

	if result.DeletedCount == 0 {
		log.Println("No page found with the specified ID")
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "No page found with the specified ID.",
			Data:    nil,
		})
	}

	// Emit WebSocket event for page change
	userIDStr, _ := c.Locals("userID").(string)
	ws.EmitPageChanged(userIDStr, tenantID, projectID)

	log.Println("Page successfully deleted")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Page successfully deleted.",
		Data:    &fiber.Map{"data": result},
	})
}
