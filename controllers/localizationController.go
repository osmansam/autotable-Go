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
	}})
}

type runtimeTranslationResource struct {
	SourceText     string `json:"sourceText"`
	TranslatedText string `json:"translatedText"`
}

func runtimeTranslationResources(entries []models.TranslationEntry) []runtimeTranslationResource {
	resources := make([]runtimeTranslationResource, 0, len(entries))
	for _, entry := range entries {
		if entry.IsActive && entry.Status == models.TranslationStatusCurrent && entry.SourceText != "" && entry.TranslatedText != "" {
			resources = append(resources, runtimeTranslationResource{
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
	entries, err := repositories.NewLocalizationRepository().ListTranslations(c.Context(), tenantID, projectID, c.Query("locale"))
	if err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Failed to load translations"})
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Translations retrieved", Data: entries})
}

func UpdateProjectTranslation(c *fiber.Ctx) error {
	tenantID, projectID, err := localizationTenantProjectIDs(c)
	if err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: err.Error()})
	}
	locale := c.Params("locale")
	key, err := decodeTranslationKeyParam(c.Params("key"))
	if err != nil {
		return c.Status(400).JSON(responses.GeneralResponse{Status: 400, Message: "Invalid translation key"})
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
	if userID, parseErr := primitive.ObjectIDFromHex(fmt.Sprint(c.Locals("userID"))); parseErr == nil {
		entry.UpdatedBy = &userID
	}
	if err := repository.UpsertManualTranslation(c.Context(), *entry); err != nil {
		return c.Status(500).JSON(responses.GeneralResponse{Status: 500, Message: "Failed to update translation"})
	}
	return c.JSON(responses.GeneralResponse{Status: 200, Message: "Translation updated", Data: entry})
}

func decodeTranslationKeyParam(key string) (string, error) {
	return url.PathUnescape(key)
}

func validateProjectLocaleInput(input projectLocaleInput) error {
	if len(input.EnabledLocales) == 0 {
		return fmt.Errorf("at least one enabled language is required")
	}
	enabled := make(map[string]struct{}, len(input.EnabledLocales))
	for _, locale := range input.EnabledLocales {
		tag, err := language.Parse(locale)
		if err != nil || tag == language.Und {
			return fmt.Errorf("invalid locale %q", locale)
		}
		canonical := tag.String()
		if _, duplicate := enabled[canonical]; duplicate {
			return fmt.Errorf("duplicate locale %q", canonical)
		}
		enabled[canonical] = struct{}{}
	}
	source, err := language.Parse(input.SourceLocale)
	if err != nil {
		return fmt.Errorf("invalid source locale")
	}
	defaultTag, err := language.Parse(input.DefaultLocale)
	if err != nil {
		return fmt.Errorf("invalid default locale")
	}
	if _, ok := enabled[source.String()]; !ok {
		return fmt.Errorf("source language must be enabled")
	}
	if _, ok := enabled[defaultTag.String()]; !ok {
		return fmt.Errorf("default language must be enabled")
	}
	return nil
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
	if err := validateProjectLocaleInput(input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: err.Error()})
	}
	result, err := projectsCollection().UpdateOne(c.Context(), bson.M{"_id": projectID, "tenantId": tenantOID}, bson.M{
		"$set": bson.M{
			"sourceLocale": input.SourceLocale, "defaultLocale": input.DefaultLocale,
			"enabledLocales": input.EnabledLocales, "updatedAt": time.Now().UTC(),
		},
		"$inc": bson.M{"localizationVersion": 1},
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to update language settings"})
	}
	if result.MatchedCount == 0 {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{Status: http.StatusNotFound, Message: "Project not found"})
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
			queuedID, queueErr := services.QueueProjectTranslationJob(tenantOID, projectID, requestedBy, input.SourceLocale, targets)
			if queueErr != nil {
				return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Language settings saved, but translation job could not be created", Data: &fiber.Map{"error": queueErr.Error()}})
			}
			jobID = queuedID.Hex()
		}
	}
	return c.JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Language settings updated", Data: &fiber.Map{
		"sourceLocale": input.SourceLocale, "defaultLocale": input.DefaultLocale, "enabledLocales": input.EnabledLocales,
		"generationRequested": input.GenerateWithAI, "translationJobId": jobID,
	}})
}
