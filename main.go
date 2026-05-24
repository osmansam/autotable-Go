package main

import (
	"expvar"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/websocket/v2"
	"github.com/joho/godotenv"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/controllers"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/routes"
	"github.com/osmansam/autotableGo/ws"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	appConfig := configs.GetAppConfig()
	portNumber := ":" + os.Getenv("PORT_NUMBER")
	app := fiber.New(fiber.Config{
		BodyLimit: configs.GetMaxRequestBodySizeLimit(),
	})

	app.Use(requestid.New())
	configurePprof(app, portNumber)

	// Initialize custom metrics
	initMetrics()

	// Create a new directory if not exists to store images
	if _, err := os.Stat("./temp"); os.IsNotExist(err) {
		os.Mkdir("./temp", 0755)
	}
	// Global rate limiting middleware - COMMENTED OUT FOR LOAD TESTING
	// app.Use(limiter.New(limiter.Config{
	// 	Max:        100, // Set the maximum number of requests per client
	// 	Expiration: 1 * time.Minute, // Set the time duration for the rate limit
	// 	KeyGenerator: func(c *fiber.Ctx) string {
	// 		return c.IP() // Use client IP as the identifier
	// 	},
	// 	LimitReached: func(c *fiber.Ctx) error {
	// 		return c.Status(fiber.StatusTooManyRequests).SendString("Rate limit exceeded")
	// 	},
	// }))

	app.Use(corsFromConfig(appConfig))
	//run database
	configs.InitDB()

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
	go ws.RunRedisSubscriber()

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "Server is healthy",
		})
	})

	//routes
	routes.TenantAuthRoutes(app) // Tenant authentication routes (new multi-tenancy system)
	routes.ProjectRoutes(app)    // Project management routes

	// Global OAuth callback route (doesn't require tenant/project in URL)
	// This is the fixed URL registered in Google Cloud Console
	// Tenant/project context is retrieved from Redis state
	app.Get("/api/v1/auth/google/callback", middlewares.PublicRateLimit(), controllers.GoogleCallback)

	// Project-scoped routes with tenant and project slugs in URL
	routes.ContainerRoutes("api/v1/:tenantSlug/:projectSlug/container", app)
	routes.DynamicRoutes("api/v1/:tenantSlug/:projectSlug/dynamic", app)
	routes.AuthRoutes("api/v1/:tenantSlug/:projectSlug/auth", app) // Dynamic auth (project-scoped end-users)
	routes.PageRoutes("api/v1/:tenantSlug/:projectSlug/page", app)
	routes.AuditRoutes("api/v1/:tenantSlug/:projectSlug/audit-logs", app)
	routes.SchemaInfoRoutes("api/v1/:tenantSlug/:projectSlug", app) // Schema info routes with role-based auth
	routes.SetupExcelRoutes(app, "api/v1")                          // Excel upload routes
	routes.SwaggerRoutes(app)
	log.Println("Server is running on port: ", portNumber)
	log.Println("Metrics available at http://localhost" + portNumber + "/debug/vars")
	app.Listen(portNumber)
}

func corsFromConfig(cfg *configs.Config) fiber.Handler {
	allowedOrigins := make(map[string]struct{}, len(cfg.CorsWhitelist))
	for _, origin := range cfg.CorsWhitelist {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowedOrigins[origin] = struct{}{}
	}

	if len(allowedOrigins) == 0 {
		log.Println("CORS whitelist is empty; browser cross-origin requests will be denied")
	} else {
		log.Printf("CORS whitelist enabled for %d origin(s)", len(allowedOrigins))
	}

	return cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			_, ok := allowedOrigins[origin]
			return ok
		},
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, Idempotency-Key",
	})
}

func configurePprof(app *fiber.App, portNumber string) {
	if isProduction() {
		log.Println("pprof disabled in production")
		return
	}

	log.Println("pprof available at http://localhost" + portNumber + "/debug/pprof/")
	app.Use(pprof.New())
}

func isProduction() bool {
	return strings.EqualFold(os.Getenv("NODE_ENV"), "production")
}

// initMetrics initializes custom runtime metrics for monitoring
func initMetrics() {
	// Publish goroutine count
	expvar.Publish("goroutines", expvar.Func(func() interface{} {
		return runtime.NumGoroutine()
	}))

	// Publish memory statistics
	expvar.Publish("memory", expvar.Func(func() interface{} {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return map[string]interface{}{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
			"heap_objects":   m.HeapObjects,
		}
	}))

	// Publish CPU count
	expvar.Publish("num_cpu", expvar.Func(func() interface{} {
		return runtime.NumCPU()
	}))
}
