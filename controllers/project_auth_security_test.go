package controllers

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDefaultAuthRejectsShortPassword(t *testing.T) {
	container := defaultProjectAuthContainer()
	input := map[string]interface{}{"email": "member@example.com", "password": "short", "role": "012345678901234567890123"}
	if err := utils.ValidateContainerModel(input, container); err == nil {
		t.Fatal("default password minimum is not enforced")
	}
	input["password"] = "long-enough-password"
	if err := utils.ValidateContainerModel(input, container); err != nil {
		t.Fatal(err)
	}
}

func secureLoginFixture() models.ContainerModel {
	return models.ContainerModel{SchemaName: "auth", IsAuthContainer: true, IsRegisterActive: true, Fields: []models.Field{
		{Name: "email", Type: "string", IsLoginCredential: true, Tag: "required"},
		{Name: "password", Type: "string", IsLoginCredential: true, IsHashed: true, Tag: "required"},
		{Name: "role", Type: "string"},
	}}
}

func TestProjectLoginRejectsUnsafeCredentials(t *testing.T) {
	for _, value := range []interface{}{nil, "", bson.M{"$ne": nil}, []interface{}{"email"}, true, float64(1)} {
		t.Run(reflect.TypeOf(valueStringForTest(value)).String()+"/"+valueStringForTest(value), func(t *testing.T) {
			_, _, err := projectLoginCredentials(secureLoginFixture(), map[string]interface{}{"email": value, "password": "secret"})
			if err == nil {
				t.Fatalf("accepted invalid identifier of type %T", value)
			}
		})
	}
	container := secureLoginFixture()
	container.Fields = container.Fields[:1]
	if _, _, err := projectLoginCredentials(container, map[string]interface{}{"email": "person@example.com"}); !errors.Is(err, errUnsafePasswordLogin) {
		t.Fatalf("email-only login error = %v", err)
	}
	filter, hashed, err := projectLoginCredentials(secureLoginFixture(), map[string]interface{}{"email": "person@example.com", "password": "secret"})
	if err != nil || !reflect.DeepEqual(filter, bson.M{"email": "person@example.com"}) || len(hashed) != 1 || hashed[0].Name != "password" {
		t.Fatalf("credentials = %#v, %#v, %v", filter, hashed, err)
	}
}

func valueStringForTest(value interface{}) string { return fmt.Sprint(value) }

func TestPublicRegistrationFieldsAreServerControlled(t *testing.T) {
	container := secureLoginFixture()
	for _, key := range []string{"role", "roleId", "roles", "_id", "isAdmin", "unknown"} {
		_, err := publicRegistrationFields(container, map[string]interface{}{"email": "a@b.com", "password": "secret", key: "admin"})
		if err == nil {
			t.Errorf("accepted server-controlled/unknown field %q", key)
		}
	}
	input := map[string]interface{}{"email": "a@b.com", "password": "secret"}
	got, err := publicRegistrationFields(container, input)
	if err != nil || len(got) != 2 {
		t.Fatalf("fields = %#v, %v", got, err)
	}
	got["email"] = "changed"
	if input["email"] != "a@b.com" {
		t.Fatal("mutated request input")
	}
	container.IsRegisterActive = false
	if _, err := publicRegistrationFields(container, input); err == nil {
		t.Fatal("registration enabled despite disabled flag")
	}
}

func TestDefaultProjectAuthRequiresSecretAndDisablesPublicRoutes(t *testing.T) {
	container := defaultProjectAuthContainer()
	if !container.IsAuthContainer || container.IsRegisterActive {
		t.Fatalf("unsafe flags: %#v", container)
	}
	found := false
	for _, field := range container.Fields {
		if field.IsLoginCredential && field.IsHashed {
			found = true
		}
	}
	if !found {
		t.Fatal("default auth has no hashed secret credential")
	}
	routes := reflect.ValueOf(container.Routes)
	for i := 0; i < routes.NumField(); i++ {
		if route, ok := routes.Field(i).Interface().(models.RouteSpec); ok && route.IsActive && !route.IsAuthenticated {
			t.Errorf("public auth route %s", routes.Type().Field(i).Name)
		}
	}
	config := buildProjectLoginConfig(secureLoginFixture())
	for _, field := range config.RegisterFields {
		if isAccountPrivilegeField(field.Name) {
			t.Fatalf("role selector exposed: %#v", field)
		}
	}
}
