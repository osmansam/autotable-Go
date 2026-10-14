package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/text/language"
)

func localeDirection(locale string) string {
	base := locale
	if index := strings.IndexByte(base, '-'); index >= 0 {
		base = base[:index]
	}
	switch strings.ToLower(base) {
	case "ar", "fa", "he", "ur":
		return "rtl"
	default:
		return "ltr"
	}
}

func GetRuntimeLocaleSettings(c *fiber.Ctx) error {
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant context"})
	}
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var project models.Project
	if err := projectsCollection().FindOne(ctx, bson.M{"_id": projectOID, "tenantId": tenantOID}).Decode(&project); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}
	project.ApplyLocaleDefaults()
	directions := map[string]string{}
	for _, locale := range project.EnabledLocales {
		directions[locale] = localeDirection(locale)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"sourceLocale": project.SourceLocale, "defaultLocale": project.DefaultLocale,
		"enabledLocales": project.EnabledLocales, "directionByLocale": directions,
		"localizationVersion": project.LocalizationVersion,
	}})
}

type runtimeTranslationResource struct {
	TranslationKey string `json:"translationKey"`
	ResourceType   string `json:"resourceType"`
	ResourceID     string `json:"resourceId"`
	PropertyPath   string `json:"propertyPath"`
	SourceText     string `json:"sourceText"`
	TranslatedText string `json:"translatedText"`
}

func runtimeTranslationResources(entries []models.TranslationEntry) []runtimeTranslationResource {
	resources := make([]runtimeTranslationResource, 0, len(entries))
	for _, entry := range entries {
		if entry.IsActive && entry.Status == models.TranslationStatusCurrent && entry.SourceText != "" && entry.TranslatedText != "" {
			resources = append(resources, runtimeTranslationResource{
				TranslationKey: entry.TranslationKey, ResourceType: entry.ResourceType,
				ResourceID: entry.ResourceID, PropertyPath: entry.PropertyPath,
				SourceText: entry.SourceText, TranslatedText: entry.TranslatedText,
			})
		}
	}
	return resources
}

func GetRuntimeTranslations(c *fiber.Ctx) error {
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant context"})
	}
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	locale := c.Query("locale")
	if locale == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Locale is required"})
	}
	project, err := repositories.NewProjectLocalizationRepository().GetProject(c.Context(), tenantOID, projectOID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}
	if locale == project.SourceLocale || !containsLocale(project.EnabledLocales, locale) {
		return c.JSON(fiber.Map{"data": []runtimeTranslationResource{}})
	}
	entries, err := repositories.NewLocalizationRepository().ListTranslations(c.Context(), tenantOID, projectOID, locale)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load translations"})
	}
	return c.JSON(fiber.Map{"data": runtimeTranslationResources(entries)})
}

func SaveRuntimeLocalePreference(c *fiber.Ctx) error {
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	userID := fmt.Sprint(c.Locals("userID"))
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "User context required"})
	}
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant context"})
	}
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	var input struct {
		Locale string `json:"locale"`
	}
	if err := c.BodyParser(&input); err != nil || input.Locale == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Locale is required"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var project models.Project
	if err := projectsCollection().FindOne(ctx, bson.M{"_id": projectOID, "tenantId": tenantOID}).Decode(&project); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}
	project.ApplyLocaleDefaults()
	enabled := false
	for _, locale := range project.EnabledLocales {
		if locale == input.Locale {
			enabled = true
			break
		}
	}
	if !enabled {
		return c.Status(400).JSON(fiber.Map{"error": "Locale is not enabled for this project"})
	}
	preference := models.ProjectLocalePreference{UserID: userOID, TenantID: tenantOID, ProjectID: projectOID, Locale: input.Locale, UpdatedAt: time.Now().UTC()}
	if err := repositories.NewLocalizationRepository().UpsertPreference(ctx, preference); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save locale preference"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"locale": input.Locale}})
}

type projectLocaleInput struct {
	SourceLocale   string   `json:"sourceLocale"`
	DefaultLocale  string   `json:"defaultLocale"`
	EnabledLocales []string `json:"enabledLocales"`
	GenerateWithAI bool     `json:"generateWithAI"`
}

func localizationTenantProjectIDs(c *fiber.Ctx) (primitive.ObjectID, primitive.ObjectID, error) {
	projectID, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return primitive.NilObjectID, primitive.NilObjectID, fmt.Errorf("invalid project ID")
	}
	tenantID, err := primitive.ObjectIDFromHex(fmt.Sprint(c.Locals("tenantID")))
	if err != nil {
		return primitive.NilObjectID, primitive.NilObjectID, fmt.Errorf("invalid tenant ID")
	}
	return tenantID, projectID, nil
}

func ListProjectTranslations(c *fiber.Ctx) error {
	tenantID, projectID, err := localizationTenantProjectIDs(c)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: 400, Message: err.Error()})
	}
	entries, err := listEditorTranslations(c.Context(), tenantID, projectID, c.Query("locale"))
	if err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Failed to load translations"})
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Translations retrieved", Data: entries})
}

func listEditorTranslations(ctx context.Context, tenantID, projectID primitive.ObjectID, locale string) ([]models.TranslationEntry, error) {
	entries, err := repositories.NewLocalizationRepository().ListProjectTranslations(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if locale == "" {
		return entries, nil
	}
	filtered := make([]models.TranslationEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Locale == locale {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

func runtimeLocalizationProjectIDs(c *fiber.Ctx) (primitive.ObjectID, primitive.ObjectID, error) {
	tenantID, projectID, err := utils.GetTenantAndProjectContext(c)
	if err != nil {
		return primitive.NilObjectID, primitive.NilObjectID, err
	}
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return primitive.NilObjectID, primitive.NilObjectID, fmt.Errorf("invalid tenant context")
	}
	projectOID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return primitive.NilObjectID, primitive.NilObjectID, fmt.Errorf("invalid project context")
	}
	return tenantOID, projectOID, nil
}

func GetEditorTranslations(c *fiber.Ctx) error {
	tenantID, projectID, err := runtimeLocalizationProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	entries, err := listEditorTranslations(c.Context(), tenantID, projectID, c.Query("locale"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load translations"})
	}
	return c.JSON(fiber.Map{"data": entries})
}

func UpdateRuntimeProjectTranslation(c *fiber.Ctx) error {
	tenantID, projectID, err := runtimeLocalizationProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	return updateLocalizationTranslation(c, tenantID, projectID)
}

type translationGenerationInput struct {
	TargetLocales   []string `json:"targetLocales"`
	TranslationKeys []string `json:"translationKeys,omitempty"`
}

func queueLocalizationGeneration(c *fiber.Ctx, tenantID, projectID primitive.ObjectID) error {
	var input translationGenerationInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Invalid request body"})
	}
	project, err := repositories.NewProjectLocalizationRepository().GetProject(c.Context(), tenantID, projectID)
	if err != nil {
		return c.Status(404).JSON(responses.GeneralResponse{Status: 404, Message: "Project not found"})
	}
	if len(input.TargetLocales) == 0 {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "At least one target locale is required"})
	}
	for _, locale := range input.TargetLocales {
		if locale == project.SourceLocale || !containsLocale(project.EnabledLocales, locale) {
			return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Every target locale must be enabled and different from the source locale"})
		}
	}
	requestedBy, _ := primitive.ObjectIDFromHex(fmt.Sprint(c.Locals("userID")))
	if requestedBy.IsZero() {
		requestedBy, _ = primitive.ObjectIDFromHex(fmt.Sprint(c.Locals("tenantUserID")))
	}
	jobID, err := services.QueueProjectTranslationJob(tenantID, projectID, requestedBy, project.SourceLocale, input.TargetLocales, input.TranslationKeys)
	if err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Translation generation could not be queued"})
	}
	return c.Status(202).JSON(responses.GeneralResponse{Status: 202, Message: "Translation generation queued", Data: fiber.Map{"translationJobId": jobID.Hex()}})
}

func GenerateProjectTranslations(c *fiber.Ctx) error {
	tenantID, projectID, err := localizationTenantProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: err.Error()})
	}
	return queueLocalizationGeneration(c, tenantID, projectID)
}

func GenerateRuntimeProjectTranslations(c *fiber.Ctx) error {
	tenantID, projectID, err := runtimeLocalizationProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	return queueLocalizationGeneration(c, tenantID, projectID)
}

func getLocalizationJob(c *fiber.Ctx, tenantID, projectID primitive.ObjectID) error {
	jobID, err := primitive.ObjectIDFromHex(c.Params("jobId"))
	if err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Invalid translation job ID"})
	}
	job, err := repositories.NewLocalizationRepository().GetJob(c.Context(), tenantID, projectID, jobID)
	if err != nil {
		return c.Status(404).JSON(responses.GeneralResponse{Status: 404, Message: "Translation job not found"})
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Translation job retrieved", Data: job})
}

func GetProjectTranslationJob(c *fiber.Ctx) error {
	tenantID, projectID, err := localizationTenantProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: err.Error()})
	}
	return getLocalizationJob(c, tenantID, projectID)
}

func GetRuntimeTranslationJob(c *fiber.Ctx) error {
	tenantID, projectID, err := runtimeLocalizationProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	return getLocalizationJob(c, tenantID, projectID)
}

func ReconcileRuntimeProjectTranslations(c *fiber.Ctx) error {
	tenantID, projectID, err := runtimeLocalizationProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project context"})
	}
	project, err := repositories.NewProjectLocalizationRepository().GetProject(c.Context(), tenantID, projectID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}
	summary, err := services.ReconcileProjectTranslations(c.Context(), tenantID, projectID, project.SourceLocale, project.EnabledLocales)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Localization reconciliation failed", "data": summary})
	}
	return c.JSON(fiber.Map{"data": summary})
}

func containsLocale(locales []string, wanted string) bool {
	for _, locale := range locales {
		if locale == wanted {
			return true
		}
	}
	return false
}

func UpdateProjectTranslation(c *fiber.Ctx) error {
	tenantID, projectID, err := localizationTenantProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: err.Error()})
	}
	return updateLocalizationTranslation(c, tenantID, projectID)
}

func updateLocalizationTranslation(c *fiber.Ctx, tenantID, projectID primitive.ObjectID) error {
	locale := c.Params("locale")
	key, err := decodeTranslationKeyParam(c.Params("key"))
	if err != nil || key == "" {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Invalid translation key"})
	}
	project, err := repositories.NewProjectLocalizationRepository().GetProject(c.Context(), tenantID, projectID)
	if err != nil {
		return c.Status(404).JSON(responses.GeneralResponse{Status: 404, Message: "Project not found"})
	}
	if locale == project.SourceLocale || !containsLocale(project.EnabledLocales, locale) {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Target locale is not enabled for this project"})
	}
	repository := repositories.NewLocalizationRepository()
	entry, err := repository.GetTranslation(c.Context(), tenantID, projectID, locale, key)
	if err != nil {
		return c.Status(404).JSON(responses.GeneralResponse{Status: 404, Message: "Translation not found"})
	}
	var input struct {
		TranslatedText string `json:"translatedText"`
	}
	if err := c.BodyParser(&input); err != nil || input.TranslatedText == "" {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Translated text is required"})
	}
	entry.TranslatedText = input.TranslatedText
	entry.Origin = models.TranslationOriginManual
	entry.Status = models.TranslationStatusCurrent
	entry.IsActive = true
	entry.OrphanedAt = nil
	if userID, parseErr := primitive.ObjectIDFromHex(fmt.Sprint(c.Locals("userID"))); parseErr == nil {
		entry.UpdatedBy = &userID
	}
	if err := repositories.NewProjectLocalizationRepository().IncrementVersion(c.Context(), tenantID, projectID); err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Failed to reserve localization version"})
	}
	if err := repository.UpsertManualTranslation(c.Context(), *entry); err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Failed to update translation"})
	}
	if err := repositories.NewProjectLocalizationRepository().IncrementVersion(c.Context(), tenantID, projectID); err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Translation saved, but localization version could not be published"})
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Translation updated", Data: entry})
}

func decodeTranslationKeyParam(key string) (string, error) {
	// Canonical Ozmishow keys contain intentional percent escapes. Fiber may
	// already have decoded the outer URL layer, so never decode those twice.
	if strings.HasPrefix(key, "ozmishow:") {
		return key, nil
	}
	return url.PathUnescape(key)
}

func validateProjectLocaleInput(input projectLocaleInput) error {
	_, err := normalizeProjectLocaleInput(input)
	return err
}

func normalizeProjectLocaleInput(input projectLocaleInput) (projectLocaleInput, error) {
	if len(input.EnabledLocales) == 0 {
		return input, fmt.Errorf("at least one enabled language is required")
	}
	enabled := make(map[string]struct{}, len(input.EnabledLocales))
	normalizedEnabled := make([]string, 0, len(input.EnabledLocales))
	for _, locale := range input.EnabledLocales {
		tag, err := language.Parse(locale)
		if err != nil || tag == language.Und {
			return input, fmt.Errorf("invalid locale %q", locale)
		}
		canonical := tag.String()
		if _, duplicate := enabled[canonical]; duplicate {
			return input, fmt.Errorf("duplicate locale %q", canonical)
		}
		enabled[canonical] = struct{}{}
		normalizedEnabled = append(normalizedEnabled, canonical)
	}
	source, err := language.Parse(input.SourceLocale)
	if err != nil || source == language.Und {
		return input, fmt.Errorf("invalid source locale")
	}
	defaultTag, err := language.Parse(input.DefaultLocale)
	if err != nil || defaultTag == language.Und {
		return input, fmt.Errorf("invalid default locale")
	}
	if _, ok := enabled[source.String()]; !ok {
		return input, fmt.Errorf("source language must be enabled")
	}
	if _, ok := enabled[defaultTag.String()]; !ok {
		return input, fmt.Errorf("default language must be enabled")
	}
	input.SourceLocale = source.String()
	input.DefaultLocale = defaultTag.String()
	input.EnabledLocales = normalizedEnabled
	return input, nil
}

func UpdateProjectLocales(c *fiber.Ctx) error {
	projectID, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid project ID"})
	}
	tenantID, ok := c.Locals("tenantID").(string)
	if !ok {
		return c.Status(http.StatusForbidden).JSON(responses.GeneralResponse{Status: http.StatusForbidden, Message: "Tenant context required"})
	}
	tenantOID, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid tenant ID"})
	}
	var input projectLocaleInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Invalid request body"})
	}
	input, err = normalizeProjectLocaleInput(input)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: err.Error()})
	}
	projectRepository := repositories.NewProjectLocalizationRepository()
	current, err := projectRepository.GetProject(c.Context(), tenantOID, projectID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{Status: http.StatusNotFound, Message: "Project not found"})
	}
	if current.SourceLocale != input.SourceLocale {
		hasContent, contentErr := services.HasOzmishowLocalizedContent(c.Context(), tenantOID, projectID)
		if contentErr != nil {
			return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Failed to validate source language change"})
		}
		if hasContent {
			return c.Status(409).JSON(responses.GeneralResponse{Status: 409, Message: "Source language cannot be changed after Ozmishow localized content has been created"})
		}
	}
	updated, err := projectRepository.UpdateLocaleSettings(c.Context(), tenantOID, projectID, input.SourceLocale, input.DefaultLocale, input.EnabledLocales)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to update language settings"})
	}
	summary, err := services.ReconcileProjectTranslations(c.Context(), tenantOID, projectID, input.SourceLocale, input.EnabledLocales)
	if err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Language settings saved, but localization reconciliation failed", Data: summary})
	}
	jobID := ""
	if input.GenerateWithAI {
		targets := make([]string, 0, len(input.EnabledLocales))
		for _, locale := range input.EnabledLocales {
			if locale != input.SourceLocale {
				targets = append(targets, locale)
			}
		}
		if len(targets) > 0 {
			requestedBy, _ := primitive.ObjectIDFromHex(fmt.Sprint(c.Locals("userID")))
			queuedID, queueErr := services.QueueProjectTranslationJob(tenantOID, projectID, requestedBy, input.SourceLocale, targets, nil)
			if queueErr != nil {
				return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Language settings saved, but translation job could not be created"})
			}
			jobID = queuedID.Hex()
		}
	}
	return c.JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Language settings updated", Data: &fiber.Map{
		"sourceLocale": input.SourceLocale, "defaultLocale": input.DefaultLocale, "enabledLocales": input.EnabledLocales,
		"localizationVersion": updated.LocalizationVersion, "reconciliation": summary,
		"generationRequested": input.GenerateWithAI, "translationJobId": jobID,
	}})
}
