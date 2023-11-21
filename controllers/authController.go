package controllers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var userCollection *mongo.Collection=configs.GetCollection(configs.DB, "user")
var refreshTokenCollection *mongo.Collection=configs.GetCollection(configs.DB, "token")

// register a new user
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
// login and create a token
func Login(c *fiber.Ctx) error {
	var user models.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  fiber.StatusBadRequest,
			Message: "Failed to parse the request body.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	storedUser := models.User{}
	err := userCollection.FindOne(context.TODO(), bson.M{"username": user.Username}).Decode(&storedUser)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  fiber.StatusUnauthorized,
			Message: "Invalid login credentials.",
			Data:    nil,
		})
	}

	isValid := utils.CheckPasswordHash(user.Password, storedUser.Password)
	if !isValid {
		return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  fiber.StatusUnauthorized,
			Message: "Invalid login credentials.",
			Data:    nil,
		})
	}

	accessToken, err := utils.CreateJWT(storedUser.ID.Hex())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Could not generate access token.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Check if a refresh token already exists for the user
	existingToken := models.Token{}
	err = refreshTokenCollection.FindOne(context.TODO(), bson.M{"user": storedUser.ID}).Decode(&existingToken)

	if err == nil { // If a refresh token is found
		// Update the existing token's validity, expiration, IP, and UserAgent
		update := bson.M{
			"$set": bson.M{
				"isValid": true,
				"expires_at": time.Now().Add(30 * 24 * time.Hour),
				"ip":       ipAddress,
				"user_agent": userAgent,
			},
		}
		_, err = refreshTokenCollection.UpdateOne(context.TODO(), bson.M{"_id": existingToken.ID}, update)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  fiber.StatusInternalServerError,
				Message: "Could not update refresh token.",
				Data:    &fiber.Map{"data": err.Error()},
			})
		}

		return c.JSON(responses.GeneralResponse{
			Status:  fiber.StatusOK,
			Message: "Login successful.",
			Data:    &fiber.Map{"accessToken": accessToken, "refreshToken": existingToken.RefreshToken},
		})
	}

	// If no refresh token exists, create a new one
	refreshToken := utils.GenerateRefreshToken()
	tokenDocument := models.Token{
		RefreshToken: refreshToken,
		User:         storedUser.ID,
		IsValid:      true,
		Ip:           ipAddress,
		UserAgent:    userAgent,
		ExpiresAt:    time.Now().Add(30 * 24 * time.Hour),
	}

	_, err = refreshTokenCollection.InsertOne(context.TODO(), tokenDocument)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Could not save refresh token.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	return c.JSON(responses.GeneralResponse{
		Status:  fiber.StatusOK,
		Message: "Login successful.",
		Data:    &fiber.Map{"accessToken": accessToken, "refreshToken": refreshToken},
	})
}

// RefreshToken refreshes the access token
func Refresh(c *fiber.Ctx) error {
	refreshTokenStr := c.Get("Authorization")
	refreshToken := strings.TrimPrefix(refreshTokenStr, "Bearer ")

	storedToken := models.Token{}
	err := refreshTokenCollection.FindOne(context.TODO(), bson.M{"refreshtoken": refreshToken}).Decode(&storedToken)

	if err != nil || !storedToken.IsValid || storedToken.ExpiresAt.Before(time.Now()) {
		return c.Status(fiber.StatusUnauthorized).JSON(responses.GeneralResponse{
			Status:  fiber.StatusUnauthorized,
			Message: "Invalid refresh token.",
			Data:    nil,
		})
	}

	newAccessToken, err := utils.CreateJWT(storedToken.User.Hex())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Could not generate access token.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	return c.JSON(responses.GeneralResponse{
		Status:  fiber.StatusOK,
		Message: "Token refreshed successfully.",
		Data:    &fiber.Map{"accessToken": newAccessToken},
	})
}
//log out the user
func Logout(c *fiber.Ctx) error {
	refreshTokenStr := c.Get("Authorization")
	refreshToken := strings.TrimPrefix(refreshTokenStr, "Bearer ")
	update := bson.M{"$set": bson.M{"isValid": false}}
	_, err := refreshTokenCollection.UpdateOne(context.TODO(), bson.M{"refreshToken": refreshToken}, update)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  fiber.StatusInternalServerError,
			Message: "Failed to logout.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}
	return c.JSON(responses.GeneralResponse{
		Status:  fiber.StatusOK,
		Message: "Logout successful.",
		Data:    nil,
	})
}
