package middlewares

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
)

func DefaultBodySizeLimit() fiber.Handler {
	return BodySizeLimit("default", configs.GetDefaultBodySizeLimit())
}

func BulkWriteBodySizeLimit() fiber.Handler {
	return BodySizeLimit("bulk_write", configs.GetBulkWriteBodySizeLimit())
}

func BulkUpdateBodySizeLimit() fiber.Handler {
	return BodySizeLimit("bulk_update", configs.GetBulkUpdateBodySizeLimit())
}

func BulkDeleteBodySizeLimit() fiber.Handler {
	return BodySizeLimit("bulk_delete", configs.GetBulkDeleteBodySizeLimit())
}

func ExportBodySizeLimit() fiber.Handler {
	return BodySizeLimit("export", configs.GetExportBodySizeLimit())
}

func UploadBodySizeLimit() fiber.Handler {
	return BodySizeLimit("upload", configs.GetUploadBodySizeLimit())
}

func BodySizeLimit(limitID string, maxBytes int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if maxBytes < 1 {
			return c.Next()
		}

		contentLength := c.Request().Header.ContentLength()
		if contentLength > maxBytes || len(c.Request().Body()) > maxBytes {
			c.Set("X-Body-Limit", strconv.Itoa(maxBytes))
			return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
				"error":       "Request body too large",
				"limit":       maxBytes,
				"bodyLimitId": limitID,
			})
		}

		return c.Next()
	}
}
