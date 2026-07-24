package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
)

// GetAuditLogs retrieves audit logs with query filters, sorting, and pagination.
// Supports query parameters:
// - action: filter by action type (create, update, delete, login, logout, etc.)
// - schemaName: filter by schema name
// - userEmail: filter by user email
// - userId: filter by user ID (ObjectID hex string)
// - documentId: filter by document ID (ObjectID hex string)
// - startDate: filter by start date (RFC3339 format)
// - endDate: filter by end date (RFC3339 format)
// - ip: filter by IP address
// - role: filter by user role
// - sort: sort field (default: "timestamp")
// - asc: sort ascending (true/false, default: false)
// - page: page number (for pagination)
// - limit: items per page (for pagination)
func GetAuditLogs(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("Fetching audit logs with filters")

	// Call the utility function to get audit logs
	results, pager, err := utils.GetAuditLogs(ctx, c)
	if err != nil {
		log.Printf("Error fetching audit logs: %v", err)
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Error fetching audit logs: " + err.Error(),
			Data:    nil,
		})
	}

	// If no pagination, return simple response
	if pager == nil {
		log.Printf("Fetched %d audit logs (no pagination)", len(results))
		return c.JSON(results)
	}

	// Return paginated response
	log.Printf("Fetched %d audit logs (page %d of %d)", len(results), pager.Page, pager.TotalPages)
	return c.JSON(fiber.Map{
		"items":      results,
		"totalItems": pager.TotalItems,
		"totalPages": pager.TotalPages,
		"page":       pager.Page,
		"limit":      pager.Limit,
	})
}

func GetAuditLogsConfig(c *fiber.Ctx) error {
	config, err := utils.GetAuditLogsConfig()
	if err != nil {
		return utils.SendErrorResponse(c, err, "Error fetching audit logs config.")
	}
	tenantID, _ := c.Locals("tenantID").(string)
	projectID, _ := c.Locals("projectID").(string)
	authorizeRoleNames := []string{}
	if tenantID != "" && projectID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		expandedRoles := utils.ExpandRoleIdentifiers(ctx, tenantID, projectID, config.AuthorizeRole)
		for _, role := range expandedRoles {
			if !containsString(config.AuthorizeRole, role) {
				authorizeRoleNames = append(authorizeRoleNames, role)
			}
		}
	}
	return utils.SendResponse(c, http.StatusOK, "Audit logs config fetched successfully.", fiber.Map{
		"isAuthorized":       config.IsAuthorized,
		"authorizeRole":      config.AuthorizeRole,
		"authorizeRoleNames": authorizeRoleNames,
	})
}

func UpdateAuditLogsConfig(c *fiber.Ctx) error {
	var input models.AuditLogsConfig
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Invalid audit logs config payload.",
			Data:    nil,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := utils.UpdateAuditLogsConfig(ctx, input)
	if err != nil {
		return utils.SendErrorResponse(c, err, "Error updating audit logs config.")
	}
	return utils.SendResponse(c, http.StatusOK, "Audit logs config updated successfully.", config)
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
