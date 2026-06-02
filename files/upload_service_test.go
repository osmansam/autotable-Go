package files

import (
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
)

func TestNewUploadService(t *testing.T) {
	service := NewUploadService()
	if service == nil || service.TempDir != "./temp" {
		t.Fatalf("NewUploadService() = %#v", service)
	}
}

func TestSaveAndUploadRejectsInvalidTempDirectory(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "file")
	if err := os.WriteFile(filePath, []byte("content"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	service := &UploadService{TempDir: filePath}
	if _, err := service.SaveAndUpload(nil, &multipart.FileHeader{Filename: "upload.txt"}); err == nil {
		t.Fatal("SaveAndUpload() error = nil")
	}
}
