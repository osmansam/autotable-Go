package upgrade

import (
	"github.com/osmansam/autotableGo/models"
	"testing"
)

func TestPasswordPlanRequiresAnAuthIdentifierAndPreservesExistingFields(t *testing.T) {
	container := models.ContainerModel{IsAuthContainer: true, Fields: []models.Field{{Name: "email", Type: "string", IsLoginCredential: true, Order: 3}}}
	field, needed, err := PlanPasswordField(container)
	if err != nil || !needed || field.Name != "password" || !field.IsLoginCredential || !field.IsHashed || field.Tag != "required,minlength=8" || field.Order != 4 {
		t.Fatalf("password plan=%#v needed=%v err=%v", field, needed, err)
	}
	if len(container.Fields) != 1 {
		t.Fatal("planning mutated source metadata")
	}
	container.Fields = append(container.Fields, field)
	if _, needed, err := PlanPasswordField(container); err != nil || needed {
		t.Fatalf("existing secure credential must be idempotent: %v %v", needed, err)
	}
}
func TestPasswordPlanRejectsConflictsInsteadOfOverwritingFields(t *testing.T) {
	for _, fields := range [][]models.Field{
		{{Name: "password", Type: "string"}, {Name: "email", Type: "string", IsLoginCredential: true}},
		{{Name: "Password", Type: "string"}, {Name: "email", Type: "string", IsLoginCredential: true}},
		{{Name: "email", Type: "string"}},
		{{Name: "identifier", Type: "int", IsLoginCredential: true}},
	} {
		if _, _, err := PlanPasswordField(models.ContainerModel{IsAuthContainer: true, Fields: fields}); err == nil {
			t.Fatalf("unsafe migration accepted: %#v", fields)
		}
	}
	if _, _, err := PlanPasswordField(models.ContainerModel{Fields: []models.Field{{Name: "email", Type: "string", IsLoginCredential: true}}}); err == nil {
		t.Fatal("non-auth schema accepted")
	}
}
func TestEnrollmentRejectsInvalidScopeAndWeakOrOversizedPassword(t *testing.T) {
	valid := Scope{TenantID: "111111111111111111111111", ProjectID: "222222222222222222222222", Schema: "auth"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []Scope{{}, {TenantID: valid.TenantID, ProjectID: "other", Schema: "auth"}, {TenantID: valid.TenantID, ProjectID: valid.ProjectID, Schema: "../auth"}} {
		if err := scope.Validate(); err == nil {
			t.Fatalf("invalid scope accepted: %#v", scope)
		}
	}
	for _, password := range []string{"", "short", string(make([]byte, 73))} {
		if err := ValidateEnrollmentPassword([]byte(password)); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if err := ValidateEnrollmentPassword([]byte("long-enough-password")); err != nil {
		t.Fatal(err)
	}
}
