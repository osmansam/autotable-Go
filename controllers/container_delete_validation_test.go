package controllers

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDeleteContainerRejectsMalformedIDBeforeDatabaseAccess(t *testing.T) {
	if os.Getenv("AUTOTABLE_TEST_INVALID_CONTAINER_DELETE") == "1" {
		app := fiber.New()
		app.Delete("/:id", func(c *fiber.Ctx) error {
			c.Locals("tenantID", "tenant")
			c.Locals("projectID", "project")
			return DeleteContainer(c)
		})
		response, err := app.Test(httptest.NewRequest("DELETE", "/malformed", nil))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatalf("malformed delete status=%d, want 400", response.StatusCode)
		}
		return
	}
	// The isolated subprocess catches the old database initializer's fatal exit.
	// Its explicit unreachable loopback URI cannot access application data.
	cmd := exec.Command(os.Args[0], "-test.run=^TestDeleteContainerRejectsMalformedIDBeforeDatabaseAccess$")
	cmd.Env = append(os.Environ(), "AUTOTABLE_TEST_INVALID_CONTAINER_DELETE=1", "MONGO_URI_BASE=mongodb://127.0.0.1:1/", "COLLECTION_NAME=invalid_delete_test", "MONGO_URI_SUFFIX=?serverSelectionTimeoutMS=50&connectTimeoutMS=50")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("malformed ID reached database initialization: %v\n%s", err, output)
	}
}
