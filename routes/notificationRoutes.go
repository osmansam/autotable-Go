package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
)

func NotificationRoutes(baseUrl string, app *fiber.App) {
	group := app.Group(baseUrl, middlewares.PublicRateLimit(), middlewares.ProjectAuthentication, middlewares.GeneralRateLimit())
	group.Get("/", middlewares.SearchRateLimit(), controllers.GetNotifications)
	group.Get("/unread", middlewares.SearchRateLimit(), controllers.GetUnreadNotifications)
	group.Post("/:id/mark-read", middlewares.WriteRateLimit(), controllers.MarkNotificationRead)
	group.Post("/mark-read", middlewares.WriteRateLimit(), controllers.MarkNotificationRead)
	group.Post("/mark-all-read", middlewares.WriteRateLimit(), controllers.MarkAllNotificationsRead)
	group.Delete("/:id", middlewares.WriteRateLimit(), controllers.DeleteNotification)
}
