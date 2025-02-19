// controllers/auth.go
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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Register a new user dynamically based on container configuration.
func Register(c *fiber.Ctx) error {
	log.Println("Dynamic Register endpoint called")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Assume schemaName is provided as a query parameter
	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing schemaName parameter.",
		})
	}

	// Fetch the container model (which holds your dynamic configuration)
	container, err := utils.GetContainerModel(schemaName)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	if !container.IsAuthContainer {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "This container is not configured for authentication.",
		})
	}

	// Parse request body into a generic map
	var userData map[string]interface{}
	if err := c.BodyParser(&userData); err != nil {
		log.Println("Failed to parse request body:", err)
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse the request body.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Validate and process login credential fields:
	for _, field := range container.Fields {
		if field.IsLoginCredential {
			rawValue, exists := userData[field.Name]
			if !exists {
				return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
					Status:  http.StatusBadRequest,
					Message: "Missing login credential field: " + field.Name,
				})
			}
			// If the field is set to be hashed, process it accordingly.
			if field.IsHashed {
				strVal, ok := rawValue.(string)
				if !ok {
					return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
						Status:  http.StatusBadRequest,
						Message: "Invalid value type for field: " + field.Name,
					})
				}
				hashedVal, err := utils.HashPassword(strVal)
				if err != nil {
					return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
						Status:  http.StatusInternalServerError,
						Message: "Error hashing field: " + field.Name,
						Data:    &fiber.Map{"data": err.Error()},
					})
				}
				userData[field.Name] = hashedVal
			}
		}
	}

	// **Call the validation to ensure all fields (including login credentials) are valid.**
	if err := utils.ValidateContainerModel(userData, *container); err != nil {
		log.Printf("Validation failed for schema %s: %v", schemaName, err)
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Validation failed.",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Optionally, check for uniqueness on fields marked as Unique.
	collection := configs.GetCollection(schemaName)
	for _, field := range container.Fields {
		if field.Unique {
			if value, exists := userData[field.Name]; exists {
				count, err := collection.CountDocuments(ctx, bson.M{field.Name: value})
				if err != nil {
					return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
						Status:  http.StatusInternalServerError,
						Message: "Error checking uniqueness for field: " + field.Name,
						Data:    &fiber.Map{"data": err.Error()},
					})
				}
				if count > 0 {
					return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
						Status:  http.StatusBadRequest,
						Message: "Duplicate value for field: " + field.Name,
					})
				}
			}
		}
	}

	// Insert the processed user data into the container's collection.
	result, err := collection.InsertOne(ctx, userData)
	if err != nil {
		log.Printf("Failed to create user for schema: %s, error: %v", schemaName, err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create user.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	log.Println("User registered successfully in schema:", schemaName)
	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "User registered successfully.",
		Data:    result,
	})
}

// Login endpoint dynamically verifies user credentials.
func Login(c *fiber.Ctx) error {
	log.Println("Dynamic Login endpoint called")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Expect schemaName as a query parameter to identify the container
	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing schemaName parameter.",
		})
	}

	// Retrieve container model
	container, err := utils.GetContainerModel(schemaName)
	if err != nil {
		log.Printf("Failed to fetch container model for schema: %s, error: %v", schemaName, err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch container model.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	if !container.IsAuthContainer {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "This container is not configured for authentication.",
		})
	}

	// Parse login credentials from request body
	var loginData map[string]interface{}
	if err := c.BodyParser(&loginData); err != nil {
		log.Println("Failed to parse login request body:", err)
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse the request body.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Build a filter using login credential fields that are not hashed
	filter := bson.M{}
	var hashedFields []models.Field // keep track of fields that need to be checked later
	for _, field := range container.Fields {
		if field.IsLoginCredential {
			val, exists := loginData[field.Name]
			if !exists {
				return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
					Status:  http.StatusBadRequest,
					Message: "Missing login credential field: " + field.Name,
				})
			}
			if field.IsHashed {
				// Do not include hashed field in the filter since we'll verify it after retrieval.
				hashedFields = append(hashedFields, field)
			} else {
				filter[field.Name] = val
			}
		}
	}

	collection := configs.GetCollection(schemaName)
	var storedUser map[string]interface{}
	err = collection.FindOne(ctx, filter).Decode(&storedUser)
	if err != nil {
		log.Println("User not found or invalid credentials")
		return c.Status(http.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  http.StatusUnauthorized,
			Message: "Invalid login credentials.",
		})
	}

	// For fields that are hashed (e.g. password), compare the provided raw value with the stored hash.
	for _, field := range hashedFields {
		providedRaw, ok := loginData[field.Name].(string)
		if !ok {
			return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
				Status:  http.StatusBadRequest,
				Message: "Invalid value for field: " + field.Name,
			})
		}
		storedHashed, ok := storedUser[field.Name].(string)
		if !ok {
			return c.Status(http.StatusUnauthorized).JSON(responses.GeneralResponse{
				Status:  http.StatusUnauthorized,
				Message: "Invalid login credentials.",
			})
		}
		if !utils.CheckPasswordHash(providedRaw, storedHashed) {
			return c.Status(http.StatusUnauthorized).JSON(responses.GeneralResponse{
				Status:  http.StatusUnauthorized,
				Message: "Invalid login credentials.",
			})
		}
	}

	// Assume the stored document contains an "_id" field and optionally a "role" field.
	var userID string
	if id, ok := storedUser["_id"].(primitive.ObjectID); ok {
		userID = id.Hex()
	} else if idStr, ok := storedUser["_id"].(string); ok {
		userID = idStr
	} else {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Invalid user data.",
		})
	}
	role := ""
	if r, ok := storedUser["role"].(string); ok {
		role = r
	}
	// Generate tokens dynamically
	tokenDetails, err := utils.GenerateTokens(userID, role)
	if err != nil {
		log.Println("Could not generate tokens for user:", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Could not generate tokens.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Remove any sensitive fields (like hashed passwords) before returning user data.
	delete(storedUser, "password")
	return c.JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Login successful.",
		Data: &fiber.Map{
			"accessToken":  tokenDetails.AccessToken,
			"refreshToken": tokenDetails.RefreshToken,
			"user":         storedUser,
		},
	})
}
