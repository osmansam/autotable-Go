package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"go.mongodb.org/mongo-driver/mongo"
)

// CreateMultipleContainers is a public, project-scoped development importer.
// Remove its route after use. It accepts an array of CreateContainer bodies.
func CreateMultipleContainers(c *fiber.Ctx) error {
	return newCreateMultipleContainersHandler(utils.GetTenantAndProjectFromSlugs, createContainerForProject)(c)
}

type containerBatchResult struct {
	SchemaName string      `json:"schemaName"`
	Status     int         `json:"status"`
	Created    bool        `json:"created"`
	ID         interface{} `json:"id,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type containerProjectResolver func(*fiber.Ctx) (string, string, error)
type containerProjectCreator func(context.Context, models.ContainerModel, string, string) (*mongo.InsertOneResult, error)

func newCreateMultipleContainersHandler(resolve containerProjectResolver, create containerProjectCreator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var containers []models.ContainerModel
		if err := json.Unmarshal(c.Body(), &containers); err != nil || len(containers) == 0 || len(containers) > 100 {
			return utils.SendResponse(c, http.StatusBadRequest, "Provide a JSON array containing 1 to 100 container definitions.", nil)
		}
		names := make(map[string]bool, len(containers))
		for i := range containers {
			name := strings.TrimSpace(containers[i].SchemaName)
			if name == "" || names[name] {
				return utils.SendResponse(c, http.StatusBadRequest, "Every container must have a non-empty, distinct schemaName.", nil)
			}
			containers[i].SchemaName = name
			names[name] = true
		}

		// Resolve only URL slugs, never caller-supplied tenant/project IDs.
		tenantID, projectID, err := resolve(c)
		if err != nil || tenantID == "" || projectID == "" {
			return utils.SendResponse(c, http.StatusNotFound, "The tenant/project URL must identify an existing active project.", nil)
		}
		results := make([]containerBatchResult, 0, len(containers))
		created, failed := 0, 0
		for _, container := range containers {
			ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
			inserted, err := create(ctx, container, tenantID, projectID)
			cancel()
			item := containerBatchResult{SchemaName: container.SchemaName, Status: http.StatusCreated}
			if inserted != nil {
				item.ID = inserted.InsertedID
				item.Created = true
				created++
			}
			if err != nil {
				item.Status = http.StatusInternalServerError
				item.Error = err.Error()
				var httpErr *fiber.Error
				if errors.As(err, &httpErr) {
					item.Status = httpErr.Code
					item.Error = httpErr.Message
				}
				failed++
			}
			results = append(results, item)
		}
		if created > 0 {
			userID, _ := c.Locals("userID").(string)
			ws.EmitContainerChanged(userID, tenantID, projectID)
		}
		status := http.StatusCreated
		if failed > 0 {
			status = http.StatusMultiStatus
		}
		return c.Status(status).JSON(fiber.Map{
			"created": created, "failed": failed, "results": results,
		})
	}
}
