package configs

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// EnvMongoURI loads the .env file and constructs the full MongoDB URI
// by combining MONGO_URI_BASE, COLLECTION_NAME, and MONGO_URI_SUFFIX.
func EnvMongoURI() string {
	// Load the .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Get the base and collection name from environment variables
	base := os.Getenv("MONGO_URI_BASE")
	collection := os.Getenv("COLLECTION_NAME")
	suffix := os.Getenv("MONGO_URI_SUFFIX") // Optional, can be empty

	// Check that required variables are set
	if base == "" || collection == "" {
		log.Fatal("MONGO_URI_BASE or COLLECTION_NAME is not set in the environment")
	}

	// If no suffix is provided, default to a common suffix
	if suffix == "" {
		suffix = "?retryWrites=true&w=majority"
	}

	// Construct the full URI
	fullURI := base + collection + suffix
	return fullURI
}
