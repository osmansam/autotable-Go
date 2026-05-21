package files

import (
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

type UploadService struct {
	TempDir string
}

func NewUploadService() *UploadService {
	return &UploadService{TempDir: "./temp"}
}

func (s *UploadService) SaveAndUpload(c *fiber.Ctx, file *multipart.FileHeader) (string, error) {
	if err := os.MkdirAll(s.TempDir, 0755); err != nil {
		return "", err
	}

	tempFilePath := filepath.Join(s.TempDir, filepath.Base(file.Filename))
	if err := c.SaveFile(file, tempFilePath); err != nil {
		return "", err
	}
	defer os.Remove(tempFilePath)

	return utils.UploadToCloudinary(tempFilePath)
}
