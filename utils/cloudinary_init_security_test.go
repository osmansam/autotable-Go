package utils

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCloudinaryConfigurationErrorOmitsCredentials(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "CLOUD_NAME=invalid%xy", "CLOUD_API_KEY=SYNTHETIC_CLOUD_KEY_MARKER", "CLOUD_API_SECRET=SYNTHETIC_CLOUD_SECRET_MARKER")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("invalid cloud configuration did not fail closed")
	}
	if strings.Contains(string(output), "SYNTHETIC_CLOUD_KEY_MARKER") || strings.Contains(string(output), "SYNTHETIC_CLOUD_SECRET_MARKER") {
		t.Fatal("cloud configuration failure exposed credential markers")
	}
}
