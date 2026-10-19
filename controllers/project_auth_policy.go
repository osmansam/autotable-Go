package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var errUnsafePasswordLogin = errors.New("password login requires a hashed secret credential; configure authentication before enabling login")

func isAccountPrivilegeField(name string) bool {
	switch strings.ToLower(name) {
	case "role", "roleid", "roles", "isadmin", "permissions", "tenantid", "projectid":
		return true
	}
	return false
}

func projectLoginCredentials(container models.ContainerModel, input map[string]interface{}) (bson.M, []models.Field, error) {
	filter := bson.M{}
	var secrets []models.Field
	for _, field := range container.Fields {
		if !field.IsLoginCredential {
			continue
		}
		// Credentials are scalar strings, never MongoDB selectors or arrays.
		value, ok := input[field.Name].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, nil, fiber.NewError(fiber.StatusBadRequest, "Invalid or missing login credential: "+field.Name)
		}
		if field.IsHashed {
			secrets = append(secrets, field)
		} else {
			filter[field.Name] = value
		}
	}
	if len(secrets) == 0 || len(filter) == 0 {
		return nil, nil, errUnsafePasswordLogin
	}
	return filter, secrets, nil
}

func publicRegistrationFields(container models.ContainerModel, input map[string]interface{}) (map[string]interface{}, error) {
	if !container.IsAuthContainer || !container.IsRegisterActive {
		return nil, fiber.NewError(fiber.StatusForbidden, "Self-registration is disabled")
	}
	if _, _, err := projectLoginCredentials(container, input); err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(container.Fields))
	for _, field := range container.Fields {
		allowed[field.Name] = true
	}
	result := make(map[string]interface{}, len(input))
	for key, value := range input {
		if !allowed[key] || isSystemLoginConfigField(key) || isAccountPrivilegeField(key) {
			return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("Field %s cannot be supplied during self-registration", key))
		}
		result[key] = value
	}
	return result, nil
}

func defaultProjectAuthContainer() models.ContainerModel {
	return models.ContainerModel{
		SchemaName: "auth", IsAuthContainer: true, IsRegisterActive: false,
		Fields: []models.Field{
			{Name: "email", Type: "string", Tag: "required", IsLoginCredential: true, Unique: true, Order: 1},
			{Name: "password", Type: "string", Tag: "required,minlength=8", IsLoginCredential: true, IsHashed: true, Order: 2},
			{Name: "role", Type: "objectId", Tag: "required", ObjectSchemaName: "role", Order: 3,
				PopulationSettings: &models.PopulationSettings{FieldName: "role", PopulatedFields: []string{"name"}, DisplayFields: []string{"name"}, InputSelectionField: "name", DisplayLabel: "Role"}},
		},
		// Account data is managed through the management boundary, independently
		// of legacy project end-user route flags.
		Redis:           defaultContainerRedis("auth", nil),
		PopulatedRoutes: []string{}, Pipelines: []models.PipelineStage{},
		DynamicFunctions: []models.DynamicFunction{}, Workflows: []models.DynamicWorkflow{},
		DynamicApis: []models.DynamicApiModel{}, Indexes: []models.Index{},
	}
}

func assignPublicRegistrationRole(ctx context.Context, tenantID, projectID string, container models.ContainerModel, user map[string]interface{}) error {
	for _, field := range container.Fields {
		if !isAccountPrivilegeField(field.Name) {
			continue
		}
		switch field.Name {
		case "role", "roleId", "roles":
		default:
			continue // Other privilege fields are never set by public registration.
		}
		var role interface{} = "user"
		switch strings.ToLower(field.Type) {
		case "objectid", "objectidarray":
			var stored struct {
				ID primitive.ObjectID `bson:"_id"`
			}
			if err := utils.GetDynamicCollectionForProject(tenantID, projectID, "role").FindOne(ctx, bson.M{"name": "user"}).Decode(&stored); err != nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "Default self-registration role is not configured")
			}
			role = stored.ID
		}
		if strings.HasSuffix(strings.ToLower(field.Type), "array") || field.Name == "roles" {
			if id, ok := role.(primitive.ObjectID); ok {
				role = id.Hex()
			}
			role = []interface{}{role}
		}
		user[field.Name] = role
	}
	return nil
}
