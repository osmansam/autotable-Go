package utils

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/responses"
	"go.mongodb.org/mongo-driver/mongo"
)

func DecodeCursor(cursor *mongo.Cursor, ctx context.Context) ([]map[string]interface{}, error) {
	var items []map[string]interface{}
	for cursor.Next(ctx) {
		var item map[string]interface{}
		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// sendResponse is a helper function to send responses using the same structure.
func SendResponse(c *fiber.Ctx, status int, message string, data interface{}) error {
	return c.Status(status).JSON(responses.GeneralResponse{
		Status:  status,
		Message: message,
		Data:    data,
	})
}

// SendErrorResponse handles errors by logging the internal error and returning a generic message to the client.
func SendErrorResponse(c *fiber.Ctx, err error, genericMessage string) error {
	normalized := NormalizeErrorResponse(err, genericMessage)
	if normalized.Quiet {
		return nil
	}

	if normalized.Status >= 500 {
		log.Printf("Internal error: %v", err)
	}

	return SendResponse(c, normalized.Status, normalized.Message, nil)
}
