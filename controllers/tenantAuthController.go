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
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

var usersCollection *mongo.Collection = configs.GetCollection("users")
var tenantsCollection *mongo.Collection = configs.GetCollection("tenants")
var tenantMembershipsCollection *mongo.Collection = configs.GetCollection("tenant_memberships")
var projectMembershipsCollection *mongo.Collection = configs.GetCollection("project_memberships")
var projectsCollection *mongo.Collection = configs.GetCollection("projects")

// TenantRegisterInput represents the registration payload
type TenantRegisterInput struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8"`
	Name        string `json:"name" validate:"required"`
	TenantName  string `json:"tenantName" validate:"required"`
	TenantSlug  string `json:"tenantSlug" validate:"required"`
}

// TenantLoginInput represents the login payload
type TenantLoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	TenantID string `json:"tenantId,omitempty"` // Optional: specify which tenant to login to
}

// ProjectSwitchInput represents switching to a project context
type ProjectSwitchInput struct {
	ProjectID string `json:"projectId" validate:"required"`
}

// TenantRefreshInput represents the refresh token payload
type TenantRefreshInput struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

// TenantRegister creates a new user and tenant (organization)
func TenantRegister(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var input TenantRegisterInput
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

	// Check if user already exists
	var existingUser models.User
	err := usersCollection.FindOne(ctx, bson.M{"email": input.Email}).Decode(&existingUser)
	if err == nil {
		return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
			Status:  http.StatusConflict,
			Message: "User with this email already exists",
			Data:    nil,
		})
	}

	// Check if tenant slug is already taken
	var existingTenant models.Tenant
	err = tenantsCollection.FindOne(ctx, bson.M{"slug": input.TenantSlug}).Decode(&existingTenant)
	if err == nil {
		return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
			Status:  http.StatusConflict,
			Message: "Tenant slug already taken",
			Data:    nil,
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Create user
	newUser := models.User{
		ID:        primitive.NewObjectID(),
		Email:     input.Email,
		Name:      input.Name,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store user with hashed password (we'll add password field to the document)
	userDoc := bson.M{
		"_id":       newUser.ID,
		"email":     newUser.Email,
		"name":      newUser.Name,
		"password":  string(hashedPassword),
		"isActive":  newUser.IsActive,
		"createdAt": newUser.CreatedAt,
		"updatedAt": newUser.UpdatedAt,
	}

	_, err = usersCollection.InsertOne(ctx, userDoc)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create user",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Create tenant
	newTenant := models.Tenant{
		ID:          primitive.NewObjectID(),
		Name:        input.TenantName,
		Slug:        input.TenantSlug,
		OwnerUserID: newUser.ID,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = tenantsCollection.InsertOne(ctx, newTenant)
	if err != nil {
		// Rollback: delete the user
		usersCollection.DeleteOne(ctx, bson.M{"_id": newUser.ID})
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create tenant",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Create tenant membership with owner role
	tenantMembership := models.TenantMembership{
		ID:        primitive.NewObjectID(),
		TenantID:  newTenant.ID,
		UserID:    newUser.ID,
		Roles:     []string{models.TenantRoleOwner},
		Status:    models.MembershipStatusActive,
		CreatedBy: newUser.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = tenantMembershipsCollection.InsertOne(ctx, tenantMembership)
	if err != nil {
		// Rollback: delete tenant and user
		tenantsCollection.DeleteOne(ctx, bson.M{"_id": newTenant.ID})
		usersCollection.DeleteOne(ctx, bson.M{"_id": newUser.ID})
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create tenant membership",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Generate tokens
	tokens, err := utils.GenerateTenantTokens(
		newUser.ID.Hex(),
		newUser.Email,
		newTenant.ID.Hex(),
		"", // No project yet
		[]string{models.TenantRoleOwner},
		string(models.RoleScopeTenant),
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate tokens",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
		Status:  http.StatusCreated,
		Message: "User and tenant created successfully",
		Data: &fiber.Map{
			"user":         newUser,
			"tenant":       newTenant,
			"accessToken":  tokens.AccessToken,
			"refreshToken": tokens.RefreshToken,
		},
	})
}

// TenantLogin authenticates a user and provides tenant-scoped tokens
func TenantLogin(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var input TenantLoginInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Find user by email
	var userDoc bson.M
	err := usersCollection.FindOne(ctx, bson.M{"email": input.Email}).Decode(&userDoc)
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  http.StatusUnauthorized,
			Message: "Invalid credentials",
			Data:    nil,
		})
	}

	// Check if user is active
	isActive, _ := userDoc["isActive"].(bool)
	if !isActive {
		return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
			Status:  http.StatusForbidden,
			Message: "User account is disabled",
			Data:    nil,
		})
	}

	// Verify password
	hashedPassword, _ := userDoc["password"].(string)
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(input.Password))
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  http.StatusUnauthorized,
			Message: "Invalid credentials",
			Data:    nil,
		})
	}

	userID := userDoc["_id"].(primitive.ObjectID)
	email := userDoc["email"].(string)

	// Find user's tenant memberships
	cursor, err := tenantMembershipsCollection.Find(ctx, bson.M{
		"userId": userID,
		"status": models.MembershipStatusActive,
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch memberships",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}
	defer cursor.Close(ctx)

	var memberships []models.TenantMembership
	if err = cursor.All(ctx, &memberships); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode memberships",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	if len(memberships) == 0 {
		return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
			Status:  http.StatusForbidden,
			Message: "User has no active tenant memberships",
			Data:    nil,
		})
	}

	// If tenantID is specified, use that; otherwise use the first membership
	var selectedMembership models.TenantMembership
	if input.TenantID != "" {
		tenantOID, err := primitive.ObjectIDFromHex(input.TenantID)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
				Status:  http.StatusBadRequest,
				Message: "Invalid tenant ID",
				Data:    &fiber.Map{"error": err.Error()},
			})
		}

		found := false
		for _, m := range memberships {
			if m.TenantID == tenantOID {
				selectedMembership = m
				found = true
				break
			}
		}

		if !found {
			return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
				Status:  http.StatusForbidden,
				Message: "User is not a member of the specified tenant",
				Data:    nil,
			})
		}
	} else {
		selectedMembership = memberships[0]
	}

	// Get tenant details
	var tenant models.Tenant
	err = tenantsCollection.FindOne(ctx, bson.M{"_id": selectedMembership.TenantID}).Decode(&tenant)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch tenant",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	if !tenant.IsActive {
		return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
			Status:  http.StatusForbidden,
			Message: "Tenant is disabled",
			Data:    nil,
		})
	}

	// Generate tokens with tenant scope
	tokens, err := utils.GenerateTenantTokens(
		userID.Hex(),
		email,
		tenant.ID.Hex(),
		"", // No project context yet
		selectedMembership.Roles,
		string(models.RoleScopeTenant),
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate tokens",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Get all tenants the user belongs to
	var tenantIDs []primitive.ObjectID
	for _, m := range memberships {
		tenantIDs = append(tenantIDs, m.TenantID)
	}

	var userTenants []models.Tenant
	tenantCursor, err := tenantsCollection.Find(ctx, bson.M{"_id": bson.M{"$in": tenantIDs}})
	if err == nil {
		tenantCursor.All(ctx, &userTenants)
		tenantCursor.Close(ctx)
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Login successful",
		Data: &fiber.Map{
			"accessToken":  tokens.AccessToken,
			"refreshToken": tokens.RefreshToken,
			"user": fiber.Map{
				"id":    userID.Hex(),
				"email": email,
				"name":  userDoc["name"],
			},
			"tenant":       tenant,
			"roles":        selectedMembership.Roles,
			"allTenants":   userTenants,
		},
	})
}

// SwitchToProject switches the user's context to a specific project
func SwitchToProject(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var input ProjectSwitchInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Get current user from context (set by middleware)
	userID := c.Locals("tenantUserID").(string)
	tenantID := c.Locals("tenantID").(string)

	projectOID, err := primitive.ObjectIDFromHex(input.ProjectID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid project ID",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	userOID, _ := primitive.ObjectIDFromHex(userID)
	tenantOID, _ := primitive.ObjectIDFromHex(tenantID)

	// Verify project belongs to tenant
	var project models.Project
	err = projectsCollection.FindOne(ctx, bson.M{
		"_id":      projectOID,
		"tenantId": tenantOID,
	}).Decode(&project)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "Project not found or does not belong to tenant",
			Data:    nil,
		})
	}

	if !project.IsActive {
		return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
			Status:  http.StatusForbidden,
			Message: "Project is disabled",
			Data:    nil,
		})
	}

	// Check user's project membership
	var projectMembership models.ProjectMembership
	err = projectMembershipsCollection.FindOne(ctx, bson.M{
		"userId":    userOID,
		"projectId": projectOID,
		"status":    models.MembershipStatusActive,
	}).Decode(&projectMembership)
	if err != nil {
		return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{
			Status:  http.StatusForbidden,
			Message: "User is not a member of this project",
			Data:    nil,
		})
	}

	email := c.Locals("email").(string)

	// Generate new tokens with project scope
	tokens, err := utils.GenerateTenantTokens(
		userID,
		email,
		tenantID,
		input.ProjectID,
		projectMembership.Roles,
		string(models.RoleScopeProject),
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate tokens",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Switched to project context",
		Data: &fiber.Map{
			"accessToken":  tokens.AccessToken,
			"refreshToken": tokens.RefreshToken,
			"project":      project,
			"roles":        projectMembership.Roles,
		},
	})
}

// TenantRefreshToken refreshes the access token using a valid refresh token
func TenantRefreshToken(c *fiber.Ctx) error {
	var input TenantRefreshInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Parse and validate refresh token
	claims, err := utils.ParseTenantToken(input.RefreshToken)
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  http.StatusUnauthorized,
			Message: "Invalid refresh token",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	// Generate new tokens
	tokens, err := utils.GenerateTenantTokens(
		claims.UserID,
		claims.Email,
		claims.TenantID,
		claims.ProjectID,
		claims.Roles,
		claims.RoleScope,
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate tokens",
			Data:    &fiber.Map{"error": err.Error()},
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Token refreshed successfully",
		Data: &fiber.Map{
			"accessToken":  tokens.AccessToken,
			"refreshToken": tokens.RefreshToken,
		},
	})
}

// TenantLogout logs out the user (client should discard tokens)
func TenantLogout(c *fiber.Ctx) error {
	// In a stateless JWT system, logout is handled client-side by removing tokens
	// Here we can log the logout event for audit purposes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := c.Locals("tenantUserID").(string)
	email := c.Locals("email").(string)

	userOID, _ := primitive.ObjectIDFromHex(userID)

	auditUser := &models.AuditUser{
		ID:    userOID,
		Email: email,
	}

	ip := c.IP()
	userAgent := c.Get("User-Agent")

	if err := utils.LogLogout(ctx, auditUser, ip, userAgent); err != nil {
		log.Printf("Failed to log logout: %v", err)
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Logged out successfully",
		Data:    nil,
	})
}

// GetCurrentUser returns the current authenticated tenant user's info
func GetCurrentUser(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := c.Locals("tenantUserID").(string)
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
			Data:    nil,
		})
	}

	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": userOID}).Decode(&user)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "User not found",
			Data:    nil,
		})
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "User retrieved successfully",
		Data: &fiber.Map{
			"user":      user,
			"tenantID":  c.Locals("tenantID"),
			"projectID": c.Locals("projectID"),
			"roles":     c.Locals("roles"),
			"roleScope": c.Locals("roleScope"),
		},
	})
}
