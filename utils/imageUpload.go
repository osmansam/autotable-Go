package utils

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

var cld *cloudinary.Cloudinary

func init() {
	cloudName := os.Getenv("CLOUD_NAME")
	apiKey := os.Getenv("CLOUD_API_KEY")
	apiSecret := os.Getenv("CLOUD_API_SECRET")

	cloudinaryURL := fmt.Sprintf("cloudinary://%s:%s@%s", apiKey, apiSecret, cloudName)

	var err error
	cld, err = cloudinary.NewFromURL(cloudinaryURL)
	if err != nil {
		// Handle the error or panic, depending on your preference
		panic(err)
	}
}

func UploadToCloudinary(filePath string) (string, error) {
	ctx := context.Background()

	// Open the file located at filePath
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Upload the file to Cloudinary
	uploadResult, err := cld.Upload.Upload(
		ctx,
		file,
		uploader.UploadParams{})
	if err != nil {
		return "", err
	}

	return uploadResult.SecureURL, nil
}

func SetNestedField(m map[string]interface{}, keys []string, value interface{}) {
    if len(keys) > 1 {
        key := keys[0]
        if _, exists := m[key]; !exists {
            m[key] = make(map[string]interface{})
        }
        SetNestedField(m[key].(map[string]interface{}), keys[1:], value)
    } else {
        m[keys[0]] = value
    }
}

func ProcessFormFields(fields map[string][]string) map[string]interface{} {
    result := make(map[string]interface{})
    for k, v := range fields {
        keys := strings.Split(k, "[")
        for i, key := range keys {
            keys[i] = strings.TrimSuffix(key, "]")
        }
        SetNestedField(result, keys, v[0]) 
    }
    return result
}
