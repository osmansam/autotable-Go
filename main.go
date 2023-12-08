package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"plugin"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/routes"
)




const portNumber = ":3002"
// executeDynamicCode executes dynamic code from a request.
func ExecuteDynamicCode(c *fiber.Ctx) error {
    // Structure to hold incoming request data
    type request struct {
        Code string `json:"code"`
    }
    var req request
	 if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
    }
   functionName := c.Query("functionName")
    pluginFileName := "temp_" + functionName + ".so"
    fileName := "temp_" + functionName + ".go"

    // Delete existing files if they exist to ensure fresh compilation
    if _, err := os.Stat(fileName); err == nil {
        os.Remove(fileName)
        os.Remove(pluginFileName)
    }

    // Write the new code to a .go file
    err := ioutil.WriteFile(fileName, []byte(req.Code), 0644)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to write code"})
    }

    // Compile the new code into a plugin
    out, err := exec.Command("go", "build", "-buildmode=plugin", fileName).CombinedOutput()
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to compile code", "output": string(out)})
    }

    // Load the newly compiled plugin
    p, err := plugin.Open(pluginFileName)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to open plugin"})
    }

    // Lookup for the function in the new plugin
    f, err := p.Lookup(functionName)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to find function in code"})
    }

    // Execute the function
    if executeFunc, ok := f.(func(*fiber.Ctx) (interface{}, error)); ok {
        result, err := executeFunc(c)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Function execution failed", "details": err.Error()})
        }
        return c.JSON(fiber.Map{"result": result})
    } else {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid function signature"})
    }
}
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
	app.Get("/api/v1/execute", ExecuteDynamicCode)
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



