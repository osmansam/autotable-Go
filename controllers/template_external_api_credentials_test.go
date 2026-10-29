package controllers

import (
	"context"
	"encoding/json"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"reflect"
	"strings"
	"testing"
	"time"
)

func nestedCredentialTemplate(id string) []models.ContainerModel {
	call := func(name, url string) models.DynamicWorkflowStep {
		return models.DynamicWorkflowStep{Name: name, Type: "call_api", Config: map[string]interface{}{"credentialId": id, "url": url, "headers": map[string]interface{}{"X-Reference": id}, "body": map[string]interface{}{"credentialId": id}}}
	}
	return []models.ContainerModel{{SchemaName: "orders", Workflows: []models.DynamicWorkflow{{Name: "inactive", Steps: []models.DynamicWorkflowStep{
		{Name: "parent", Type: "condition", Steps: []models.DynamicWorkflowStep{call("then", "https://api.example.com/a")}, ElseSteps: []models.DynamicWorkflowStep{call("else", "https://other.example.com/b")}, Branches: []models.WorkflowBranch{{Name: "branch", Steps: []models.DynamicWorkflowStep{call("branch", "https://{{host}}/c")}}}},
		{Name: "ordinary", Type: "set", Config: map[string]interface{}{"credentialId": id}},
	}}}}}
}
func TestTemplateExternalCredentialBindingsIncludeInactiveNestedBranches(t *testing.T) {
	source, target := primitive.NewObjectID().Hex(), primitive.NewObjectID().Hex()
	containers := nestedCredentialTemplate(source)
	requirements := templateExternalAPIRequirements(containers)
	if len(requirements) != 1 || requirements[0].SourceCredentialID != source || len(requirements[0].Usages) != 3 || !reflect.DeepEqual(requirements[0].Hosts, []string{"api.example.com", "other.example.com"}) {
		t.Fatalf("requirements=%#v", requirements)
	}
	before, _ := json.Marshal(containers)
	cloned, err := bindTemplateExternalAPICredentials(containers, map[string]string{source: target})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(containers)
	if string(before) != string(after) {
		t.Fatal("binding mutated source")
	}
	got := cloned[0].Workflows[0].Steps[0]
	for _, step := range []models.DynamicWorkflowStep{got.Steps[0], got.ElseSteps[0], got.Branches[0].Steps[0]} {
		if step.Config["credentialId"] != target {
			t.Fatal("nested reference not mapped")
		}
		if step.Config["body"].(map[string]interface{})["credentialId"] != source || step.Config["headers"].(map[string]interface{})["X-Reference"] != source {
			t.Fatal("unrelated authored config changed")
		}
	}
	if cloned[0].Workflows[0].Steps[1].Config["credentialId"] != source {
		t.Fatal("non-call_api ID changed")
	}
	for _, bindings := range []map[string]string{nil, {source: target, "unknown": target}} {
		if _, err := bindTemplateExternalAPICredentials(containers, bindings); err == nil {
			t.Fatalf("invalid bindings accepted: %v", bindings)
		}
	}
}

type templateCredentialLookup map[string]*models.ExternalAPICredential

func (lookup templateCredentialLookup) GetTenantExternalAPICredential(_ context.Context, tenant, id string) (*models.ExternalAPICredential, error) {
	c, ok := lookup[id]
	if !ok || c.TenantID.Hex() != tenant {
		return nil, mongo.ErrNoDocuments
	}
	return c, nil
}
func TestTemplateCredentialPreflightRejectsUnusableOrUnownedKeys(t *testing.T) {
	tenant, id, source := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID().Hex()
	base := models.ExternalAPICredential{ID: id, TenantID: tenant, Scope: "tenant", AllowedDomains: []string{"example.com"}}
	requirements := templateExternalAPIRequirements(nestedCredentialTemplate(source))
	now := time.Now()
	for _, scenario := range []string{"valid", "foreign", "project", "expired", "revoked", "wrong host", "duplicate", "unknown binding", "missing binding"} {
		t.Run(scenario, func(t *testing.T) {
			c := base
			input := CreateProjectInput{ExternalAPICredentialIDs: []string{id.Hex()}, ExternalAPICredentialBindings: map[string]string{source: id.Hex()}}
			switch scenario {
			case "foreign":
				c.TenantID = primitive.NewObjectID()
			case "project":
				c.Scope = "project"
			case "expired":
				c.ExpiresAt = now.Add(-time.Hour)
			case "revoked":
				c.RevokedAt = &now
			case "wrong host":
				c.AllowedDomains = []string{"different.example"}
			case "duplicate":
				input.ExternalAPICredentialIDs = append(input.ExternalAPICredentialIDs, id.Hex())
			case "unknown binding":
				input.ExternalAPICredentialBindings["unknown"] = id.Hex()
			case "missing binding":
				input.ExternalAPICredentialBindings = nil
			}
			ids, err := validateProjectExternalAPICredentials(context.Background(), templateCredentialLookup{id.Hex(): &c}, tenant.Hex(), input, requirements)
			if scenario == "valid" {
				if err != nil || len(ids) != 1 {
					t.Fatalf("valid mapping: %v %v", ids, err)
				}
			} else if err == nil {
				t.Fatal("invalid credentials accepted")
			}
			if err != nil && strings.Contains(err.Error(), "ciphertext") {
				t.Fatal("secret in error")
			}
		})
	}
}
