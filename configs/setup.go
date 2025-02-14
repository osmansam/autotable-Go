package configs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// init loads environment variables from .env file when the package is initialized.
func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it. Using system environment variables.")
	}
}

// Config holds all the configuration data loaded from a JSON file.
type Config struct {
	Redis struct {
		Port string `json:"port"`
		Host string `json:"host"`
	} `json:"redis"`
	App struct {
		Port int `json:"port"`
	} `json:"app"`
	Panel struct {
		Host string `json:"host"`
	} `json:"panel"`
	CorsWhitelist    []string `json:"corsWhitelist"`
	MigrationEnabled bool     `json:"migrationEnabled"`
}

// LoadConfig reads a JSON configuration file and unmarshals it into a Config struct.
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open config file %q: %w", path, err)
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("could not decode config JSON: %w", err)
	}

	return &cfg, nil
}

// ctx is a package-level context used for Redis operations.
var ctx = context.Background()


// ConnectDB connects to MongoDB using the URI from the environment.
func ConnectDB() *mongo.Client {
	client, err := mongo.NewClient(options.Client().ApplyURI(EnvMongoURI()))
	if err != nil {
		log.Fatal(err)
	}

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err = client.Connect(ctxWithTimeout); err != nil {
		log.Fatal(err)
	}

	// Ping the database to ensure the connection is established.
	if err = client.Ping(ctxWithTimeout, nil); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to MongoDB")
	return client
}

// ConnectRedis loads the appropriate configuration file based on NODE_ENV
// and creates a Redis client.
func ConnectRedis() *redis.Client {
	// Determine the environment; default to "staging" if not set.
	env := os.Getenv("NODE_ENV")
	if env == "" {
		env = "development"
	}
	// Construct the configuration file name (e.g., "staging.json", "production.json").
	configFile := fmt.Sprintf("configs/%s.json", env)
	log.Printf("Using config file: %s", configFile)

	// Load the configuration from the JSON file.
	cfg, err := LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Construct the Redis address using the host and port from the config.
	redisAddress := fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port)

	// Create the Redis client.
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddress, // e.g., "autotable-redis-staging:6379"
		Password: "",           // Set password if needed.
		DB:       0,            // Use default DB.
	})

	// Test the connection.
	if _, err = rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	fmt.Println("Connected to Redis")
	return rdb
}

// Global variables to hold our database connections.
var (
	DB          *mongo.Client   = ConnectDB()
	RedisClient *redis.Client   = ConnectRedis()
	database    *mongo.Database = DB.Database(os.Getenv("MONGO_DB_NAME"))
)

// GetCollection returns a collection from the MongoDB database.
func GetCollection(collectionName string) *mongo.Collection {
	return database.Collection(collectionName)
}
