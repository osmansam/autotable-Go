package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
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