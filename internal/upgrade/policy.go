package upgrade

import (
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"regexp"
	"strings"
)

type Scope struct{ TenantID, ProjectID, Schema string }

func (s Scope) Validate() error {
	tenant, err := primitive.ObjectIDFromHex(s.TenantID)
	if err != nil || tenant.Hex() != s.TenantID {
		return errors.New("invalid tenant scope")
	}
	project, err := primitive.ObjectIDFromHex(s.ProjectID)
	if err != nil || project.Hex() != s.ProjectID {
		return errors.New("invalid project scope")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`).MatchString(s.Schema) {
		return errors.New("invalid schema scope")
	}
	return nil
}
func (s Scope) collection(resource string) string {
	return "tenant_" + s.TenantID + "_project_" + s.ProjectID + "_" + resource
}

func PlanPasswordField(container models.ContainerModel) (models.Field, bool, error) {
	if !container.IsAuthContainer {
		return models.Field{}, false, errors.New("schema is not an auth container")
	}
	identifier := false
	hasSecret := false
	order := 0
	for _, field := range container.Fields {
		if field.Order >= order {
			order = field.Order + 1
		}
		if field.IsLoginCredential && !field.IsHashed && (field.Type == "string" || field.Type == "email") {
			identifier = true
		}
		if field.IsLoginCredential && field.IsHashed && field.Type == "string" {
			hasSecret = true
		}
		if strings.EqualFold(field.Name, "password") && !(field.Name == "password" && field.Type == "string" && field.IsHashed && field.IsLoginCredential) {
			return models.Field{}, false, errors.New("password field conflicts with existing schema")
		}
	}
	if !identifier {
		return models.Field{}, false, errors.New("no scalar login identifier is configured")
	}
	if hasSecret {
		return models.Field{}, false, nil
	}
	return models.Field{Name: "password", Type: "string", Tag: "required,minlength=8", IsHashed: true, IsLoginCredential: true, Order: order}, true, nil
}
func ValidateEnrollmentPassword(password []byte) error {
	if len(password) < 8 || len(password) > 72 || strings.TrimSpace(string(password)) == "" {
		return errors.New("password must contain 8 to 72 bytes")
	}
	return nil
}
