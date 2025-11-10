package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/websocket/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/routes"
	"github.com/osmansam/autotableGo/ws"
)

func main() {
	portNumber := ":" + os.Getenv("PORT_NUMBER")
	app := fiber.New()
	// Create a new directory if not exists to store images
		if _, err := os.Stat("./temp"); os.IsNotExist(err) {
    os.Mkdir("./temp", 0755)
}
	// Global rate limiting middleware
	app.Use(limiter.New(limiter.Config{
		Max:        100, // Set the maximum number of requests per client
		Expiration: 1 * time.Minute, // Set the time duration for the rate limit
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP() // Use client IP as the identifier
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).SendString("Rate limit exceeded")
		},
	}))

	
	//cors
	app.Use(cors.New())
	//run database
	configs.ConnectDB()
	configs.ConnectRedis()

//websocket wiring
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	// WS endpoint
	app.Get("/ws", websocket.New(ws.HandleWS))
	go ws.RunBroadcaster()
	//routes
	routes.ContainerRoutes("api/v1/container", app)
	routes.DynamicRoutes("api/v1/dynamic", app)
	routes.AuthRoutes("api/v1/auth", app)
	log.Println("Server is running on port: ", portNumber)
	app.Listen(portNumber)
}



