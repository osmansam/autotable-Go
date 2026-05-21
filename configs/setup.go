package configs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
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
	Cache            CacheConfig  `json:"cache"`
	Limits           LimitsConfig `json:"limits"`
	CorsWhitelist    []string     `json:"corsWhitelist"`
	MigrationEnabled bool         `json:"migrationEnabled"`
}

type CacheConfig struct {
	DefaultCacheTTLMinutes            int `json:"defaultCacheTTLMinutes"`
	CacheFillLockTTLSeconds           int `json:"cacheFillLockTTLSeconds"`
	CacheFillWaitTimeoutMilliseconds  int `json:"cacheFillWaitTimeoutMilliseconds"`
	CacheFillPollIntervalMilliseconds int `json:"cacheFillPollIntervalMilliseconds"`
}

type LimitsConfig struct {
	DefaultPageLimit      int                  `json:"defaultPageLimit"`
	MaxPageLimit          int                  `json:"maxPageLimit"`
	MaxUnboundedReadLimit int                  `json:"maxUnboundedReadLimit"`
	MaxExportLimit        int                  `json:"maxExportLimit"`
	MaxBulkWriteLimit     int                  `json:"maxBulkWriteLimit"`
	MaxBulkUpdateLimit    int                  `json:"maxBulkUpdateLimit"`
	MaxBulkDeleteLimit    int                  `json:"maxBulkDeleteLimit"`
	BodySizeLimits        BodySizeLimitsConfig `json:"bodySizeLimits"`
}

type BodySizeLimitsConfig struct {
	DefaultBodySizeBytes    int `json:"defaultBodySizeBytes"`
	BulkWriteBodySizeBytes  int `json:"bulkWriteBodySizeBytes"`
	BulkUpdateBodySizeBytes int `json:"bulkUpdateBodySizeBytes"`
	BulkDeleteBodySizeBytes int `json:"bulkDeleteBodySizeBytes"`
	ExportBodySizeBytes     int `json:"exportBodySizeBytes"`
	UploadBodySizeBytes     int `json:"uploadBodySizeBytes"`
}

const (
	DefaultPageLimit                  = 20
	MaxPageLimit                      = 100
	MaxUnboundedReadLimit             = 5000
	MaxExportLimit                    = 50000
	MaxBulkWriteLimit                 = 1000
	MaxBulkUpdateLimit                = 1000
	MaxBulkDeleteLimit                = 1000
	DefaultCacheTTLMinutes            = 10
	CacheFillLockTTLSeconds           = 15
	CacheFillWaitTimeoutMilliseconds  = 800
	CacheFillPollIntervalMilliseconds = 50

	DefaultBodySizeBytes    = 1 * 1024 * 1024
	BulkWriteBodySizeBytes  = 10 * 1024 * 1024
	BulkUpdateBodySizeBytes = 10 * 1024 * 1024
	BulkDeleteBodySizeBytes = 2 * 1024 * 1024
	ExportBodySizeBytes     = 1 * 1024 * 1024
	UploadBodySizeBytes     = 25 * 1024 * 1024
)

var (
	appConfig     *Config
	appConfigOnce sync.Once
)

func configFileForEnv() string {
	env := os.Getenv("NODE_ENV")
	if env == "" {
		env = "development"
	}
	return fmt.Sprintf("configs/%s.json", env)
}

func GetAppConfig() *Config {
	appConfigOnce.Do(func() {
		cfg, err := LoadConfig(configFileForEnv())
		if err != nil {
			log.Printf("Error loading app config, using defaults: %v", err)
			cfg = &Config{}
		}
		appConfig = cfg
	})

	return appConfig
}

func GetDefaultPageLimit() int {
	limit := GetAppConfig().Limits.DefaultPageLimit
	if limit < 1 {
		return DefaultPageLimit
	}
	return limit
}

func GetMaxPageLimit() int {
	limit := GetAppConfig().Limits.MaxPageLimit
	if limit < 1 {
		return MaxPageLimit
	}
	return limit
}

func GetMaxUnboundedReadLimit() int {
	limit := GetAppConfig().Limits.MaxUnboundedReadLimit
	if limit < 1 {
		return MaxUnboundedReadLimit
	}
	return limit
}

func GetMaxExportLimit() int {
	limit := GetAppConfig().Limits.MaxExportLimit
	if limit < 1 {
		return MaxExportLimit
	}
	return limit
}

func GetMaxBulkWriteLimit() int {
	limit := GetAppConfig().Limits.MaxBulkWriteLimit
	if limit < 1 {
		return MaxBulkWriteLimit
	}
	return limit
}

func GetMaxBulkUpdateLimit() int {
	limit := GetAppConfig().Limits.MaxBulkUpdateLimit
	if limit < 1 {
		return MaxBulkUpdateLimit
	}
	return limit
}

func GetMaxBulkDeleteLimit() int {
	limit := GetAppConfig().Limits.MaxBulkDeleteLimit
	if limit < 1 {
		return MaxBulkDeleteLimit
	}
	return limit
}

func GetDefaultCacheTTL() time.Duration {
	ttlMinutes := GetAppConfig().Cache.DefaultCacheTTLMinutes
	if ttlMinutes < 1 {
		ttlMinutes = DefaultCacheTTLMinutes
	}
	return time.Duration(ttlMinutes) * time.Minute
}

func GetCacheFillLockTTL() time.Duration {
	seconds := GetAppConfig().Cache.CacheFillLockTTLSeconds
	if seconds < 1 {
		seconds = CacheFillLockTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

func GetCacheFillWaitTimeout() time.Duration {
	milliseconds := GetAppConfig().Cache.CacheFillWaitTimeoutMilliseconds
	if milliseconds < 1 {
		milliseconds = CacheFillWaitTimeoutMilliseconds
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func GetCacheFillPollInterval() time.Duration {
	milliseconds := GetAppConfig().Cache.CacheFillPollIntervalMilliseconds
	if milliseconds < 1 {
		milliseconds = CacheFillPollIntervalMilliseconds
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func GetDefaultBodySizeLimit() int {
	limit := GetAppConfig().Limits.BodySizeLimits.DefaultBodySizeBytes
	if limit < 1 {
		return DefaultBodySizeBytes
	}
	return limit
}

func GetBulkWriteBodySizeLimit() int {
	limit := GetAppConfig().Limits.BodySizeLimits.BulkWriteBodySizeBytes
	if limit < 1 {
		return BulkWriteBodySizeBytes
	}
	return limit
}

func GetBulkUpdateBodySizeLimit() int {
	limit := GetAppConfig().Limits.BodySizeLimits.BulkUpdateBodySizeBytes
	if limit < 1 {
		return BulkUpdateBodySizeBytes
	}
	return limit
}

func GetBulkDeleteBodySizeLimit() int {
	limit := GetAppConfig().Limits.BodySizeLimits.BulkDeleteBodySizeBytes
	if limit < 1 {
		return BulkDeleteBodySizeBytes
	}
	return limit
}

func GetExportBodySizeLimit() int {
	limit := GetAppConfig().Limits.BodySizeLimits.ExportBodySizeBytes
	if limit < 1 {
		return ExportBodySizeBytes
	}
	return limit
}

func GetUploadBodySizeLimit() int {
	limit := GetAppConfig().Limits.BodySizeLimits.UploadBodySizeBytes
	if limit < 1 {
		return UploadBodySizeBytes
	}
	return limit
}

func GetMaxRequestBodySizeLimit() int {
	limits := []int{
		GetDefaultBodySizeLimit(),
		GetBulkWriteBodySizeLimit(),
		GetBulkUpdateBodySizeLimit(),
		GetBulkDeleteBodySizeLimit(),
		GetExportBodySizeLimit(),
		GetUploadBodySizeLimit(),
	}

	maxLimit := 0
	for _, limit := range limits {
		if limit > maxLimit {
			maxLimit = limit
		}
	}
	if maxLimit < 1 {
		return UploadBodySizeBytes
	}
	return maxLimit
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
	configFile := configFileForEnv()
	log.Printf("Using config file: %s", configFile)

	// Load the configuration from the JSON file.
	cfg := GetAppConfig()

	// Construct the Redis address using the host and port from the config.
	redisAddress := fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port)

	// Create the Redis client.
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddress, // e.g., "autotable-redis-staging:6379"
		Password: "",           // Set password if needed.
		DB:       0,            // Use default DB.
	})

	// Test the connection.
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	fmt.Println("Connected to Redis")
	return rdb
}

// Global variables to hold our database connections.
var (
	DB            *mongo.Client
	RedisClient   *redis.Client
	database      *mongo.Database
	dbInitialized bool
)

// InitDB initializes the database connections if not already initialized.
func InitDB() {
	if !dbInitialized {
		DB = ConnectDB()
		RedisClient = ConnectRedis()
		database = DB.Database(os.Getenv("COLLECTION_NAME"))
		dbInitialized = true
	}
}

// GetCollection returns a collection from the MongoDB database.
func GetCollection(collectionName string) *mongo.Collection {
	InitDB()
	return database.Collection(collectionName)
}
