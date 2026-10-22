package controllers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
)

var loadRuntimeSchemaContainers = func(ctx context.Context, tenantID, projectID string) ([]models.ContainerModel, error) {
	cursor, err := utils.GetContainerCollectionForProject(tenantID, projectID).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	containers := []models.ContainerModel{}
	if err := cursor.All(ctx, &containers); err != nil {
		return nil, err
	}
	for i := range containers {
		containers[i].SortFieldsByOrder()
	}
	return containers, nil
}

// GetRuntimeSchemas runs after optional, project-bound application authentication.
// Its response is filtered by current application access, never management roles.
func GetRuntimeSchemas(c *fiber.Ctx) error {
	return serveRuntimeSchemas(c, func(schemas []runtimeSchema) error { return c.JSON(schemas) })
}

func GetRuntimeSchema(c *fiber.Ctx) error {
	return serveRuntimeSchemas(c, func(schemas []runtimeSchema) error {
		for _, schema := range schemas {
			if strings.EqualFold(schema.ID, c.Params("id")) {
				return c.JSON(schema)
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Schema metadata not found"})
	})
}

func GetRuntimeSchemaTypes(c *fiber.Ctx) error {
	return serveRuntimeSchemas(c, func(schemas []runtimeSchema) error {
		result := make([]models.ContainerTypes, 0, len(schemas))
		for _, schema := range schemas {
			fields := make(map[string]string, len(schema.Fields))
			for _, field := range schema.Fields {
				fields[field.Name] = field.Type
			}
			result = append(result, models.ContainerTypes{ID: schema.ID, SchemaName: schema.SchemaName, FieldTypes: fields})
		}
		return c.JSON(result)
	})
}

func serveRuntimeSchemas(c *fiber.Ctx, respond func([]runtimeSchema) error) error {
	setAuthNoStoreHeaders(c)
	userID, roles := getPageRequestUserID(c), getPageRequestRoles(c)
	_, credentialErr := middlewares.ResolveProjectCredential(c)
	refreshCookie := c.Cookies(utils.ProjectAuthCookieName(c.Params("tenantSlug"), c.Params("projectSlug"), "refresh"))
	if userID == "" && (credentialErr == nil || !errors.Is(credentialErr, middlewares.ErrMissingCredential) || refreshCookie != "") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Session expired. Please log in again."})
	}
	tenantID, _ := c.Locals("expectedTenantID").(string)
	projectID, _ := c.Locals("expectedProjectID").(string)
	if tenantID == "" || projectID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Project context is required"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	containers, err := loadRuntimeSchemaContainers(ctx, tenantID, projectID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Schema metadata could not be loaded"})
	}
	return respond(buildRuntimeSchemas(containers, userID, roles))
}

// Runtime metadata is a deliberate allowlist, never the management container model.
type runtimeSchema struct {
	ID         string           `json:"id"`
	SchemaName string           `json:"schemaName"`
	Fields     []runtimeField   `json:"fields"`
	Frontend   *models.Frontend `json:"frontend,omitempty"`
}

type runtimeField struct {
	Name               string                     `json:"name"`
	Type               string                     `json:"type"`
	Tag                string                     `json:"tag,omitempty"`
	ObjectSchemaName   string                     `json:"objectSchemaName,omitempty"`
	EnumList           []interface{}              `json:"enumList,omitempty"`
	Unique             bool                       `json:"unique,omitempty"`
	IsComputed         bool                       `json:"isComputed,omitempty"`
	IsForceDelete      bool                       `json:"isForceDelete,omitempty"`
	IsAuditIdentity    bool                       `json:"isAuditIdentity,omitempty"`
	IsSearchable       bool                       `json:"isSearchable,omitempty"`
	Order              int                        `json:"order,omitempty"`
	Children           []runtimeField             `json:"children,omitempty"`
	Frontend           *models.Frontend           `json:"frontend,omitempty"`
	PopulationSettings *models.PopulationSettings `json:"populationSettings,omitempty"`
}

func runtimeRouteAccessible(route models.RouteSpec, userID string, roles []string) bool {
	if !route.IsActive || ((route.IsAuthenticated || route.IsAuthorized || route.Access != nil) && userID == "") {
		return false
	}
	return !route.IsAuthorized || pageRequestHasAllowedRole(roles, route.AuthorizeRole)
}

func runtimeSchemaAccessible(container models.ContainerModel, userID string, roles []string) bool {
	name := strings.ToLower(strings.TrimSpace(container.SchemaName))
	if name == "containers" {
		return false
	}
	r := container.Routes
	if container.IsAuthContainer || name == "auth" || name == "role" {
		// Account writes and executable sources remain management-only. Expose
		// safe metadata only when an application data read is actually allowed.
		for _, route := range []models.RouteSpec{
			r.GetAllDynamicModelItems, r.GetAllDynamicModelItemsWithPagination, r.GetDynamicModelItem,
			r.GetItemsForSelection, r.HandleSearchDynamicModelItem, r.HandleFilterDynamicModelItem,
			r.ExportDynamicModelItems,
		} {
			if runtimeRouteAccessible(route, userID, roles) {
				return true
			}
		}
		return false
	}
	for _, route := range []models.RouteSpec{
		r.GetAllDynamicModelItems, r.GetAllDynamicModelItemsWithPagination, r.GetDynamicModelItem,
		r.GetItemsForSelection, r.HandleSearchDynamicModelItem, r.HandleFilterDynamicModelItem,
		r.CreateDynamicModelItem, r.CreateMultipleDynamicModelItem, r.UpdateDynamicModelItem,
		r.UpdateMultipleDynamicModelItem, r.DeleteDynamicModelItem, r.DeleteMultipleDynamicModelItem,
		r.ExportDynamicModelItems,
	} {
		if runtimeRouteAccessible(route, userID, roles) {
			return true
		}
	}
	for _, pipeline := range container.Pipelines {
		if runtimeRouteAccessible(models.RouteSpec{IsActive: pipeline.IsActive, IsAuthenticated: pipeline.IsAuthenticated, IsAuthorized: pipeline.IsAuthorized, AuthorizeRole: pipeline.AuthorizeRole}, userID, roles) {
			return true
		}
	}
	for _, workflow := range container.Workflows {
		if runtimeRouteAccessible(models.RouteSpec{IsActive: workflow.IsActive, IsAuthenticated: workflow.IsAuthenticated, IsAuthorized: workflow.IsAuthorized, AuthorizeRole: workflow.AuthorizeRole}, userID, roles) {
			return true
		}
	}
	for _, api := range container.DynamicApis {
		if runtimeRouteAccessible(models.RouteSpec{IsActive: api.IsActive, IsAuthenticated: api.IsAuthenticated, IsAuthorized: api.IsAuthorized, AuthorizeRole: api.AuthorizeRole}, userID, roles) {
			return true
		}
	}
	return false
}

func runtimeFields(fields []models.Field, userID string, roles []string) []runtimeField {
	result := make([]runtimeField, 0, len(fields))
	for _, field := range fields {
		if field.IsHashed || (field.IsAuthorized && (userID == "" || !pageRequestHasAllowedRole(roles, field.AuthorizeRole))) {
			continue
		}
		result = append(result, runtimeField{Name: field.Name, Type: field.Type, Tag: field.Tag,
			ObjectSchemaName: field.ObjectSchemaName, EnumList: field.EnumList, Unique: field.Unique,
			IsComputed: field.Equation != "", IsForceDelete: field.IsForceDelete, IsAuditIdentity: field.IsAuditIdentity,
			IsSearchable: field.IsSearchable, Order: field.Order, Frontend: field.Frontend,
			PopulationSettings: field.PopulationSettings, Children: runtimeFields(field.Children, userID, roles)})
	}
	return result
}

func buildRuntimeSchemas(containers []models.ContainerModel, userID string, roles []string) []runtimeSchema {
	result := make([]runtimeSchema, 0, len(containers))
	for _, container := range containers {
		if !runtimeSchemaAccessible(container, userID, roles) {
			continue
		}
		result = append(result, runtimeSchema{ID: container.ID.Hex(), SchemaName: container.SchemaName,
			Fields: runtimeFields(container.Fields, userID, roles), Frontend: container.Frontend})
	}
	return result
}
