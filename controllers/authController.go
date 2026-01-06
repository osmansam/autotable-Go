// controllers/auth.go
package controllers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/oauth2"
)

// Register a new user dynamically based on container configuration.
func Register(c *fiber.Ctx) error {
	log.Println("Dynamic Register endpoint called")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context: " + err.Error(),
		})
	}

	// Assume schemaName is provided as a query parameter
	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Missing schemaName parameter.",
		})
	}

	// Fetch the container model using tenant/project context
	container, err := utils.GetContainerModel(tenantID, projectID, schemaName)
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
	collection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)
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

	// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context: " + err.Error(),
		})
	}

	// Find the auth container (IsAuthContainer = true) in project-specific containers collection
	containersCollection := utils.GetContainerCollectionForProject(tenantID, projectID)
	var container models.ContainerModel
	err = containersCollection.FindOne(ctx, bson.M{"isAuthContainer": true}).Decode(&container)
	if err != nil {
		log.Printf("Failed to find auth container: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Authentication container not configured.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	schemaName := container.SchemaName
	log.Printf("Using auth container schema: %s", schemaName)

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

	collection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)
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
	
	// Try different possible role field names
	if r, ok := storedUser["role"].(string); ok && r != "" {
		role = r
	} else if r, ok := storedUser["roleId"].(string); ok && r != "" {
		role = r
	} else if roleOID, ok := storedUser["role"].(primitive.ObjectID); ok {
		role = roleOID.Hex()
	} else if roleOID, ok := storedUser["roleId"].(primitive.ObjectID); ok {
		role = roleOID.Hex()
	} else if roles, ok := storedUser["roles"].([]interface{}); ok && len(roles) > 0 {
		if roleStr, ok := roles[0].(string); ok {
			role = roleStr
		} else if roleOID, ok := roles[0].(primitive.ObjectID); ok {
			role = roleOID.Hex()
		}
	}
	
	// Get tenant and project slugs from context
	tenantSlug, _ := c.Locals("tenantSlug").(string)
	projectSlug, _ := c.Locals("projectSlug").(string)
	
	// Generate tokens dynamically with tenant and project context
	tokenDetails, err := utils.GenerateTokens(userID, role, tenantID, projectID, tenantSlug, projectSlug)
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

    // Log Login Audit
    authUser := &models.AuditUser{
        ID: func() primitive.ObjectID {
            if oid, err := primitive.ObjectIDFromHex(userID); err == nil {
                return oid
            }
            return primitive.NilObjectID
        }(),
        Roles: []string{role},
        Email: func() string {
            if email, ok := storedUser["email"].(string); ok {
                return email
            }
            return ""
        }(),
    }
    // IP and UserAgent
    ip := c.IP()
    userAgent := c.Get("User-Agent")
    
    if err := utils.LogLogin(ctx, tenantID, projectID, authUser, ip, userAgent); err != nil {
        log.Printf("Failed to log login: %v", err)
    }

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

// GoogleLogin initiates the Google OAuth flow
func GoogleLogin(c *fiber.Ctx) error {
	log.Println("Google OAuth Login initiated")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	oauthConfig := utils.GetGoogleOAuthConfig()
	
	// Generate a cryptographically secure random state for CSRF protection
	state := uuid.New().String()
	
	// Store state in Redis with 5-minute expiration
	redisClient := configs.RedisClient
	stateKey := "oauth:state:" + state
	err := redisClient.Set(ctx, stateKey, "valid", 5*time.Minute).Err()
	if err != nil {
		log.Printf("Failed to store OAuth state: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to initiate OAuth flow",
		})
	}
	
	url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return c.Redirect(url)
}

// GoogleCallback handles the OAuth callback from Google
func GoogleCallback(c *fiber.Ctx) error {
	log.Println("Google OAuth Callback received")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Validate state parameter for CSRF protection
	state := c.Query("state")
	if state == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid OAuth state",
		})
	}

	// Verify state exists in Redis
	redisClient := configs.RedisClient
	stateKey := "oauth:state:" + state
	val, err := redisClient.Get(ctx, stateKey).Result()
	if err != nil || val != "valid" {
		log.Printf("Invalid or expired OAuth state: %v", err)
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid or expired OAuth state. Please try again.",
		})
	}

	// Delete the state from Redis (one-time use)
	redisClient.Del(ctx, stateKey)

	// Get the authorization code from query params
	code := c.Query("code")
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Authorization code not found",
		})
	}

	// Exchange code for token
	oauthConfig := utils.GetGoogleOAuthConfig()
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		log.Printf("Failed to exchange token: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to exchange authorization code",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Get user info from Google
	client := oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Printf("Failed to get user info: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get user information from Google",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	defer resp.Body.Close()

	// Parse user info
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to read user information",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	var googleUser map[string]interface{}
	if err := json.Unmarshal(body, &googleUser); err != nil {
		log.Printf("Failed to parse user info: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to parse user information",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Extract tenant and project context from state or query params
	// Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get project context: " + err.Error(),
		})
	}

	// Find the auth container (IsAuthContainer = true) in project-specific containers collection
	containersCollection := utils.GetContainerCollectionForProject(tenantID, projectID)
	var authContainer models.ContainerModel
	err = containersCollection.FindOne(ctx, bson.M{"isAuthContainer": true}).Decode(&authContainer)
	if err != nil {
		log.Printf("Failed to find auth container: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Authentication container not configured",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	schemaName := authContainer.SchemaName
	log.Printf("Using auth container schema: %s", schemaName)

	// Find email field in the container schema
	var emailFieldName string
	for _, field := range authContainer.Fields {
		if field.Type == "email" || field.Name == "email" {
			emailFieldName = field.Name
			break
		}
	}

	if emailFieldName == "" {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Auth container must have an email field",
		})
	}

	email, ok := googleUser["email"].(string)
	if !ok || email == "" {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Email not provided by Google",
		})
	}

	// Check if user exists in project-specific collection
	collection := utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)
	var existingUser map[string]interface{}
	err = collection.FindOne(ctx, bson.M{emailFieldName: email}).Decode(&existingUser)

	var userID string
	var role string

	if err != nil {
		// User doesn't exist, create new user
		log.Printf("Creating new user with email: %s", email)
		
		newUser := map[string]interface{}{
			emailFieldName: email,
		}

		// Helper function to check if a field exists in the schema
		fieldExists := func(fieldName string) bool {
			for _, field := range authContainer.Fields {
				if field.Name == fieldName {
					return true
				}
			}
			return false
		}

		// Add name if available and field exists in schema
		if name, ok := googleUser["name"].(string); ok && fieldExists("name") {
			newUser["name"] = name
		}

		// Add picture if available and field exists in schema
		if picture, ok := googleUser["picture"].(string); ok && fieldExists("picture") {
			newUser["picture"] = picture
		}

		// Add default role if field exists in schema
		if fieldExists("role") {
			newUser["role"] = "user"
		}

		result, err := collection.InsertOne(ctx, newUser)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create user",
				Data:    &fiber.Map{"error": err.Error()},
			})
		}

		// Delete the cache for this schema after creating a new user
		if authContainer.Redis.IsRedisCached {
			err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, schemaName, &authContainer)
			if err != nil {
				log.Printf("Failed to delete cache for schema: %s, error: %v", schemaName, err)
				// Don't fail the request, just log the error
			}

			// Additionally, delete cache for each schema mentioned in TriggeredRedisCaches
		for _, triggeredSchema := range authContainer.Redis.TriggeredRedisCaches {
			// Fetch the correct container model for the triggered schema
			triggeredContainer, err := utils.GetContainerModel(tenantID, projectID, triggeredSchema)
			if err != nil {
				log.Printf("Error fetching container model for schema %s: %v", triggeredSchema, err)
				continue
			}
			
			err = utils.DeleteCacheForSchema(ctx, tenantID, projectID, triggeredSchema, triggeredContainer)
			if err != nil {
				// Log the error and continue with the next iteration
				log.Printf("Error deleting cache for schema %s: %v", triggeredSchema, err)
				continue
			}
		}
	}

		if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
			userID = oid.Hex()
		}
		// Get role from newUser with improved extraction
		if r, ok := newUser["role"].(string); ok && r != "" {
			role = r
		} else if roleOID, ok := newUser["role"].(primitive.ObjectID); ok {
			role = roleOID.Hex()
		}
		existingUser = newUser
		existingUser["_id"] = result.InsertedID
	} else {
		// User exists, log them in
		log.Printf("User found with email: %s", email)
		
		if id, ok := existingUser["_id"].(primitive.ObjectID); ok {
			userID = id.Hex()
		} else if idStr, ok := existingUser["_id"].(string); ok {
			userID = idStr
		}

		if r, ok := existingUser["role"].(string); ok && r != "" {
			role = r
		} else if roleOID, ok := existingUser["role"].(primitive.ObjectID); ok {
			role = roleOID.Hex()
		} else if roles, ok := existingUser["roles"].([]interface{}); ok && len(roles) > 0 {
			if roleStr, ok := roles[0].(string); ok {
				role = roleStr
			} else if roleOID, ok := roles[0].(primitive.ObjectID); ok {
				role = roleOID.Hex()
			}
		}
	}

	// Get tenant and project slugs from context
	tenantSlug, _ := c.Locals("tenantSlug").(string)
	projectSlug, _ := c.Locals("projectSlug").(string)

	// Generate JWT tokens with tenant and project context
	tokenDetails, err := utils.GenerateTokens(userID, role, tenantID, projectID, tenantSlug, projectSlug)
	if err != nil {
		log.Printf("Failed to generate tokens: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate tokens",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Remove sensitive fields
	delete(existingUser, "password")

    // Log Login Audit
    authUser := &models.AuditUser{
        ID: func() primitive.ObjectID {
            if oid, err := primitive.ObjectIDFromHex(userID); err == nil {
                return oid
            }
            return primitive.NilObjectID
        }(),
        Roles: []string{role},
        Email: func() string {
            if email, ok := existingUser[emailFieldName].(string); ok {
                return email
            }
            return ""
        }(),
    }
    // IP and UserAgent
    ip := c.IP()
    userAgent := c.Get("User-Agent")
    
    if err := utils.LogLogin(ctx, tenantID, projectID, authUser, ip, userAgent); err != nil {
        log.Printf("Failed to log google login: %v", err)
    }

	return c.JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Google login successful",
		Data: &fiber.Map{
			"accessToken":  tokenDetails.AccessToken,
			"refreshToken": tokenDetails.RefreshToken,
			"user":         existingUser,
		},
	})
}


// Logout handles the user logout process.
// Currently it mainly serves to log the logout action for audit purposes.
// In the future, it can handle token blacklisting.
func Logout(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Get user from context (set by middleware)
    // Note: To use this endpoint effectively, it should be protected by authentication middleware
    user := utils.GetUserFromContext(c)
    
    // Extract tenant and project context
    // Extract tenant and project context from URL slugs (falls back to query params or JWT for backward compatibility)
    tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to get project context: " + err.Error(),
        })
    }
    
    // IP and UserAgent
    ip := c.IP()
    userAgent := c.Get("User-Agent")

    if err := utils.LogLogout(ctx, tenantID, projectID, user, ip, userAgent); err != nil {
        log.Printf("Failed to log logout: %v", err)
        // We log the error but still return success to the user as the collection
        // of usage data shouldn't block the user's workflow.
    }

    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
        Status:  http.StatusOK,
        Message: "Logout successful.",
    })
}
