package controllers

import (
	"context"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/url"
	"sort"
	"strings"
	"time"
)

type templateExternalAPIRequirement struct {
	SourceCredentialID string   `json:"sourceCredentialId"`
	Name               string   `json:"name"`
	Hosts              []string `json:"hosts"`
	Usages             []string `json:"usages"`
}

func visitTemplateCredentialSteps(steps []models.DynamicWorkflowStep, path string, visit func(*models.DynamicWorkflowStep, string)) {
	for i := range steps {
		step := &steps[i]
		usage := fmt.Sprintf("%s / %s[%d]", path, step.Name, i)
		if step.Type == "call_api" {
			visit(step, usage)
		}
		visitTemplateCredentialSteps(step.Steps, usage+" / steps", visit)
		visitTemplateCredentialSteps(step.ElseSteps, usage+" / elseSteps", visit)
		for j := range step.Branches {
			visitTemplateCredentialSteps(step.Branches[j].Steps, usage+" / branch "+step.Branches[j].Name, visit)
		}
	}
}
func templateExternalAPIRequirements(containers []models.ContainerModel) []templateExternalAPIRequirement {
	requirements := []templateExternalAPIRequirement{}
	indexes := map[string]int{}
	for _, container := range containers {
		for _, workflow := range container.Workflows {
			visitTemplateCredentialSteps(workflow.Steps, container.SchemaName+" / "+workflow.Name, func(step *models.DynamicWorkflowStep, usage string) {
				id, _ := step.Config["credentialId"].(string)
				if id == "" {
					return
				}
				idx, ok := indexes[id]
				if !ok {
					idx = len(requirements)
					indexes[id] = idx
					requirements = append(requirements, templateExternalAPIRequirement{SourceCredentialID: id, Name: fmt.Sprintf("External API connection %d", idx+1), Hosts: []string{}, Usages: []string{}})
				}
				req := &requirements[idx]
				req.Usages = append(req.Usages, usage)
				raw, _ := step.Config["url"].(string)
				parsed, err := url.Parse(raw)
				if err != nil {
					return
				}
				host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
				if host == "" || strings.ContainsAny(host, "{}$") {
					return
				}
				for _, existing := range req.Hosts {
					if existing == host {
						return
					}
				}
				req.Hosts = append(req.Hosts, host)
				sort.Strings(req.Hosts)
			})
		}
	}
	return requirements
}
func validateTemplateBindings(requirements []templateExternalAPIRequirement, bindings map[string]string) error {
	known := map[string]bool{}
	for _, req := range requirements {
		known[req.SourceCredentialID] = true
		if bindings[req.SourceCredentialID] == "" {
			return fmt.Errorf("missing external API credential binding for %s", req.SourceCredentialID)
		}
	}
	for source := range bindings {
		if !known[source] {
			return fmt.Errorf("unknown external API credential binding: %s", source)
		}
	}
	return nil
}
func copyBoundCredentialSteps(steps []models.DynamicWorkflowStep, bindings map[string]string) []models.DynamicWorkflowStep {
	if steps == nil {
		return nil
	}
	out := append([]models.DynamicWorkflowStep{}, steps...)
	for i := range out {
		s := &out[i]
		if s.Type == "call_api" {
			if id, ok := s.Config["credentialId"].(string); ok && id != "" {
				config := make(map[string]interface{}, len(s.Config))
				for key, value := range s.Config {
					config[key] = value
				}
				config["credentialId"] = bindings[id]
				s.Config = config
			}
		}
		s.Steps = copyBoundCredentialSteps(s.Steps, bindings)
		s.ElseSteps = copyBoundCredentialSteps(s.ElseSteps, bindings)
		if s.Branches != nil {
			s.Branches = append([]models.WorkflowBranch{}, s.Branches...)
			for j := range s.Branches {
				s.Branches[j].Steps = copyBoundCredentialSteps(s.Branches[j].Steps, bindings)
			}
		}
	}
	return out
}
func bindTemplateExternalAPICredentials(containers []models.ContainerModel, bindings map[string]string) ([]models.ContainerModel, error) {
	if err := validateTemplateBindings(templateExternalAPIRequirements(containers), bindings); err != nil {
		return nil, err
	}
	result := append([]models.ContainerModel{}, containers...)
	for i := range result {
		result[i].Workflows = append([]models.DynamicWorkflow{}, result[i].Workflows...)
		for j := range result[i].Workflows {
			w := &result[i].Workflows[j]
			w.Steps = copyBoundCredentialSteps(w.Steps, bindings)
		}
	}
	return result, nil
}

type tenantExternalCredentialLookup interface {
	GetTenantExternalAPICredential(context.Context, string, string) (*models.ExternalAPICredential, error)
}

func validateProjectExternalAPICredentials(ctx context.Context, repo tenantExternalCredentialLookup, tenant string, input CreateProjectInput, requirements []templateExternalAPIRequirement) ([]string, error) {
	if err := validateTemplateBindings(requirements, input.ExternalAPICredentialBindings); err != nil {
		return nil, err
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range input.ExternalAPICredentialIDs {
		if seen[id] {
			return nil, fmt.Errorf("duplicate external API credential ID: %s", id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, req := range requirements {
		id := input.ExternalAPICredentialBindings[req.SourceCredentialID]
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	credentials := map[string]*models.ExternalAPICredential{}
	for _, id := range ids {
		if _, err := primitive.ObjectIDFromHex(id); err != nil {
			return nil, fmt.Errorf("invalid external API credential ID: %s", id)
		}
		c, err := repo.GetTenantExternalAPICredential(ctx, tenant, id)
		if err != nil {
			return nil, fmt.Errorf("external API credential %s is not available in this tenant", id)
		}
		if c.TenantID.Hex() != tenant {
			return nil, fmt.Errorf("external API credential is not owned by this tenant")
		}
		if err := repositories.ValidateSharedExternalAPICredential(c, time.Now()); err != nil {
			return nil, fmt.Errorf("external API credential %s: %w", id, err)
		}
		credentials[id] = c
	}
	for _, req := range requirements {
		c := credentials[input.ExternalAPICredentialBindings[req.SourceCredentialID]]
		for _, host := range req.Hosts {
			allowed := false
			for _, domain := range c.AllowedDomains {
				domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
				if domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, fmt.Errorf("external API credential %s does not allow template host %s", c.ID.Hex(), host)
			}
		}
	}
	return ids, nil
}
func GetTemplateExternalAPIRequirements(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	tenant, _ := c.Locals("tenantID").(string)
	t, err := primitive.ObjectIDFromHex(tenant)
	if err != nil {
		return tenantCredentialError(c, err)
	}
	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return tenantCredentialError(c, err)
	}
	var project models.Project
	if err = projectsCollection().FindOne(ctx, bson.M{"_id": id}).Decode(&project); err != nil {
		return tenantCredentialError(c, err)
	}
	if !projectTemplateVisibleToTenant(project, t) {
		return c.Status(403).JSON(responses.GeneralResponse{Status: 403, Message: "Template project is not available to this tenant"})
	}
	containers, err := readProjectContainers(ctx, project.TenantID.Hex(), project.ID.Hex())
	if err != nil {
		return tenantCredentialError(c, err)
	}
	requirements := templateExternalAPIRequirements(containers)
	repo := repositories.NewDynamicRepository()
	for i := range requirements {
		if credential, e := repo.GetTenantExternalAPICredential(ctx, tenant, requirements[i].SourceCredentialID); e == nil {
			requirements[i].Name = credential.Name
		}
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Template external API requirements retrieved", Data: &fiber.Map{"requirements": requirements}})
}
