package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func Get(c *fiber.Ctx) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Schema name is required")
	}

	getIdStr := c.Query("id")
	getId, err := primitive.ObjectIDFromHex(getIdStr)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Invalid ID format")
	}

	currentCollection := configs.GetCollection(configs.DB, schemaName)
	var result bson.M
	if err := currentCollection.FindOne(ctx, bson.M{"_id": getId}).Decode(&result); err != nil {
		return nil, fiber.NewError(http.StatusNotFound, "Item not found")
	}

	return result, nil
}