package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var pageCollection *mongo.Collection = configs.GetCollection("pages")

// CreatePage creates a new page
func CreatePage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var page models.PageModel

	log.Println("Parsing request body for CreatePage")
	if err := c.BodyParser(&page); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to parse the request body. Ensure the provided JSON is valid.")
	}

	log.Println("Validating parsed data for CreatePage")
	if validationErr := validate.Struct(&page); validationErr != nil {
		log.Printf("Validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Some required fields might be missing or have invalid values.")
	}

	log.Println("Inserting new page into the database")
	result, err := pageCollection.InsertOne(ctx, page)
	if err != nil {
		log.Printf("Failed to insert page: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to insert the page into the database. Please try again later.")
	}

	// Emit WebSocket event for page change
	ws.EmitPageChanged()

	log.Println("Page successfully created")
	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "Page successfully created.",
		Data:    &fiber.Map{"data": result},
	})
}

// GetAllPages retrieves all pages from the database
func GetAllPages(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var pages []models.PageModel

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

// GetPage retrieves a single page from the database based on its ID
func GetPage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pageIdStr := c.Params("id")
	pageId, err := primitive.ObjectIDFromHex(pageIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

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

// UpdatePage updates an existing page's details
func UpdatePage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var updatedPage models.PageModel

	log.Println("Parsing request body for UpdatePage")
	if err := c.BodyParser(&updatedPage); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return utils.SendErrorResponse(c, err, "Failed to parse the request body. Ensure the provided JSON is valid.")
	}

	log.Println("Validating parsed data for UpdatePage")
	if validationErr := validate.Struct(&updatedPage); validationErr != nil {
		log.Printf("Validation error: %v", validationErr)
		return utils.SendErrorResponse(c, validationErr, "Validation error. Some required fields might be missing or have invalid values.")
	}

	updateIdStr := c.Params("id")
	updateId, err := primitive.ObjectIDFromHex(updateIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Provided ID is not in the valid format.")
	}

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
	ws.EmitPageChanged()

	log.Println("Page successfully updated")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Page successfully updated.",
		Data:    &fiber.Map{"data": updateResult},
	})
}

// DeletePage deletes a page from the database
func DeletePage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		log.Printf("Invalid ID format: %v", err)
		return utils.SendErrorResponse(c, err, "Invalid ID format provided. Please ensure the ID is a valid MongoDB ObjectID.")
	}

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
	ws.EmitPageChanged()

	log.Println("Page successfully deleted")
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Page successfully deleted.",
		Data:    &fiber.Map{"data": result},
	})
}
