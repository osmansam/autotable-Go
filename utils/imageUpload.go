package utils

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/osmansam/autotableGo/models"
	_ "golang.org/x/image/webp"
)

const (
	DefaultBrandingMaxBytes     int64 = 2 << 20
	DefaultBrandingMaxDimension       = 4096
)

type ValidatedImage struct {
	Content []byte
	MIME    string
	Format  string
	Width   int
	Height  int
	Bytes   int64
}

type BrandingUploadOptions struct {
	Folder   string
	PublicID string
}

type BrandingAssetStore interface {
	Upload(ctx context.Context, file io.Reader, options BrandingUploadOptions) (models.BrandingAsset, error)
	Delete(ctx context.Context, assetID string) error
}

type CloudinaryBrandingAssetStore struct{}

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

func ValidateBrandingImage(reader io.Reader, maxBytes int64) (ValidatedImage, error) {
	return validateBrandingImage(reader, maxBytes, DefaultBrandingMaxDimension)
}

func validateBrandingImage(reader io.Reader, maxBytes int64, maxDimension int) (ValidatedImage, error) {
	if maxBytes <= 0 || maxDimension <= 0 {
		return ValidatedImage{}, fmt.Errorf("invalid branding image limits")
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return ValidatedImage{}, fmt.Errorf("read branding image: %w", err)
	}
	if int64(len(content)) > maxBytes {
		return ValidatedImage{}, fmt.Errorf("branding image exceeds %d bytes", maxBytes)
	}
	if len(content) == 0 {
		return ValidatedImage{}, fmt.Errorf("branding image is empty")
	}

	mimeType := http.DetectContentType(content)
	allowedMIME := map[string]bool{
		"image/png": true, "image/jpeg": true, "image/webp": true,
	}
	if !allowedMIME[mimeType] {
		return ValidatedImage{}, fmt.Errorf("unsupported branding image type %q", mimeType)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return ValidatedImage{}, fmt.Errorf("decode branding image: %w", err)
	}
	if format == "jpg" {
		format = "jpeg"
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxDimension || config.Height > maxDimension {
		return ValidatedImage{}, fmt.Errorf("branding image dimensions must be between 1 and %d pixels", maxDimension)
	}
	return ValidatedImage{
		Content: content,
		MIME:    mimeType,
		Format:  format,
		Width:   config.Width,
		Height:  config.Height,
		Bytes:   int64(len(content)),
	}, nil
}

func (CloudinaryBrandingAssetStore) Upload(ctx context.Context, file io.Reader, options BrandingUploadOptions) (models.BrandingAsset, error) {
	validated, err := ValidateBrandingImage(file, DefaultBrandingMaxBytes)
	if err != nil {
		return models.BrandingAsset{}, err
	}
	result, err := cld.Upload.Upload(ctx, bytes.NewReader(validated.Content), uploader.UploadParams{
		Folder:         options.Folder,
		PublicID:       options.PublicID,
		ResourceType:   "image",
		AllowedFormats: api.CldAPIArray{"png", "jpg", "jpeg", "webp"},
	})
	if err != nil {
		return models.BrandingAsset{}, err
	}
	return models.BrandingAsset{
		URL:      result.SecureURL,
		Provider: "cloudinary",
		AssetID:  result.PublicID,
		Width:    result.Width,
		Height:   result.Height,
		Format:   result.Format,
		Bytes:    int64(result.Bytes),
	}, nil
}

func (CloudinaryBrandingAssetStore) Delete(ctx context.Context, assetID string) error {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil
	}
	invalidate := true
	_, err := cld.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID: assetID, ResourceType: "image", Invalidate: &invalidate,
	})
	return err
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

// ConvertFormFieldTypes converts string values from multipart forms to their appropriate types based on the container schema
func ConvertFormFieldTypes(itemMap map[string]interface{}, container interface{}) map[string]interface{} {
	// Type assertion to get the Fields from container
	type ContainerWithFields interface {
		GetFields() []interface{}
	}

	// We'll accept the container as models.ContainerModel, but to avoid import cycle,
	// we'll use reflection or pass fields directly
	// For now, let's return the same map - this will be called from the controller
	return itemMap
}
