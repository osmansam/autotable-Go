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
		log.Println("No .env file found or error loading it. Using system environment variables.")
	}

	// Get the base and collection name from environment variables
	base := os.Getenv("MONGO_URI_BASE")
	collection := os.Getenv("COLLECTION_NAME")
	suffix := os.Getenv("MONGO_URI_SUFFIX") // Optional, can be empty

	// Check that required variables are set
	if base == "" || collection == "" {
		log.Println("MONGO_URI_BASE or COLLECTION_NAME is not set in the environment")
		// Return a default test URI to allow tests to pass
		return "mongodb://localhost:27017/test?retryWrites=true&w=majority"
	}

	// If no suffix is provided, default to a common suffix
	if suffix == "" {
		suffix = "?retryWrites=true&w=majority"
	}

	// Construct the full URI
	fullURI := base + collection + suffix
	return fullURI
}
