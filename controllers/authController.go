package controllers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var userCollection *mongo.Collection = configs.GetCollection(configs.DB, "user")

// Register a new user
func Register(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var user models.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Failed to parse the request body.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}
	// Check if username exists
	count, err := userCollection.CountDocuments(ctx, bson.M{"username": user.Username})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Failed to query the user from the database.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	if count > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Username already exists. Please choose another.",
			Data:    nil,
		})
	}

	// Hash the password
	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Failed to hash password.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	user.Password = hashedPassword
	_, err = userCollection.InsertOne(ctx, user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Failed to create user.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	return c.Status(fiber.StatusCreated).JSON(responses.GeneralResponse{
		Status:  fiber.StatusCreated,
		Message: "User registered successfully.",
		Data:    nil,
	})
}

// Login and create tokens
func Login(c *fiber.Ctx) error {
	var loginReq models.LoginRequest
	if err := c.BodyParser(&loginReq); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Failed to parse the request body.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	storedUser := models.User{}
	err := userCollection.FindOne(context.TODO(), bson.M{"username": loginReq.Username}).Decode(&storedUser)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  fiber.StatusUnauthorized,
			Message: "Invalid login credentials.",
			Data:    nil,
		})
	}

	isValid := utils.CheckPasswordHash(loginReq.Password, storedUser.Password)
	if !isValid {
		return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  fiber.StatusUnauthorized,
			Message: "Invalid login credentials.",
			Data:    nil,
		})
	}

	tokenDetails, err := utils.GenerateTokens(storedUser.ID.Hex(),storedUser.Role)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Could not generate tokens.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Now, you can access the AccessToken and RefreshToken from tokenDetails
	accessToken := tokenDetails.AccessToken
	refreshToken := tokenDetails.RefreshToken

	// Continue with your logic to respond with the tokens
	userData := map[string]interface{}{
		"id":       storedUser.ID.Hex(),
		"username": storedUser.Username,
		"role":     storedUser.Role,
	}

	return c.JSON(responses.GeneralResponse{
		Status:  fiber.StatusOK,
		Message: "Login successful.",
		Data:    &fiber.Map{"accessToken": accessToken, "refreshToken": refreshToken, "user": userData},
	})
	
}
func Refresh(c *fiber.Ctx) error {
	var tokenReq models.TokenRequest
	if err := c.BodyParser(&tokenReq); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Failed to parse the request body.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}


	refreshToken := tokenReq.RefreshToken

	userID,role, err := utils.ParseToken(refreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  fiber.StatusUnauthorized,
			Message: "Invalid refresh token.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// TODO, check if the refresh token belongs to the user and is not expired
	// This step requires storing tokens in DB or a cache with expiration times

	// Generate new tokens
	tokenDetails, err := utils.GenerateTokens(userID,role)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Could not generate tokens.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Respond with new tokens
	return c.JSON(responses.GeneralResponse{
		Status:  fiber.StatusOK,
		Message: "Tokens refreshed successfully.",
		Data:    &fiber.Map{"accessToken": tokenDetails.AccessToken, "refreshToken": tokenDetails.RefreshToken},
	})
}