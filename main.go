package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/routes"
)




const portNumber = ":3002"

func main() {

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
	//routes
	routes.ContainerRoutes("api/v1/container", app)
	routes.DynamicRoutes("api/v1/dynamic", app)
	routes.AuthRoutes("api/v1/auth", app)
	log.Println("Server is running on port: ", portNumber)
	app.Listen(portNumber)
}



