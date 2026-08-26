package controllers

import (
	"context"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var brandingAssetStore utils.BrandingAssetStore = utils.CloudinaryBrandingAssetStore{}

type brandingManagementResponse struct {
	Overrides *models.Branding       `json:"overrides,omitempty"`
	Effective models.RuntimeBranding `json:"effective"`
}

func brandingUpdateDocuments(patch models.BrandingPatch, currentVersion int64, now time.Time) (bson.M, bson.M) {
	set := bson.M{
		"branding.version": currentVersion + 1,
		"updatedAt":        now,
	}
	if patch.DisplayName != nil {
		set["branding.displayName"] = *patch.DisplayName
	}
	if patch.LogoAlt != nil {
		set["branding.logoAlt"] = *patch.LogoAlt
	}
	if patch.PrimaryColor != nil {
		set["branding.primaryColor"] = *patch.PrimaryColor
	}
	if patch.LoginBrandingEnabled != nil {
		set["branding.loginBrandingEnabled"] = *patch.LoginBrandingEnabled
	}
	unset := bson.M{}
	for _, field := range patch.Reset {
		unset["branding."+field] = ""
	}
	return set, unset
}

func brandingAssetField(slot string) (string, error) {
	switch slot {
	case "logo", "compactLogo", "favicon":
		return "branding." + slot, nil
	default:
		return "", fmt.Errorf("unsupported branding asset slot")
	}
}

func validateBrandingProjectScope(roleScope, currentProjectID, requestedProjectID string) error {
	if roleScope == string(models.RoleScopeTenant) {
		return nil
	}
	if roleScope == string(models.RoleScopeProject) && currentProjectID == requestedProjectID {
		return nil
	}
	return fmt.Errorf("project access denied")
}

func brandingTenantID(c *fiber.Ctx) (primitive.ObjectID, error) {
	value, ok := c.Locals("tenantID").(string)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("tenant context required")
	}
	return primitive.ObjectIDFromHex(value)
}

func brandingProjectID(c *fiber.Ctx) (primitive.ObjectID, error) {
	requested := c.Params("id")
	projectOID, err := primitive.ObjectIDFromHex(requested)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid project ID")
	}
	roleScope, _ := c.Locals("roleScope").(string)
	currentProjectID, _ := c.Locals("projectID").(string)
	if err := validateBrandingProjectScope(roleScope, currentProjectID, requested); err != nil {
		return primitive.NilObjectID, err
	}
	return projectOID, nil
}

func brandingUpdate(set, unset bson.M) bson.M {
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	return update
}

func loadTenant(ctx context.Context, tenantOID primitive.ObjectID) (models.Tenant, error) {
	var tenant models.Tenant
	err := tenantsCollection().FindOne(ctx, bson.M{"_id": tenantOID, "isActive": true}).Decode(&tenant)
	return tenant, err
}

func loadProject(ctx context.Context, tenantOID, projectOID primitive.ObjectID) (models.Project, error) {
	var project models.Project
	err := projectsCollection().FindOne(ctx, bson.M{"_id": projectOID, "tenantId": tenantOID, "isActive": true}).Decode(&project)
	return project, err
}

func sendBrandingError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(responses.GeneralResponse{Status: status, Message: message})
}

func GetTenantBranding(c *fiber.Ctx) error {
	tenantOID, err := brandingTenantID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	tenant, err := loadTenant(ctx, tenantOID)
	if err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Tenant not found")
	}
	return c.JSON(responses.GeneralResponse{Status: http.StatusOK, Data: brandingManagementResponse{
		Overrides: tenant.Branding, Effective: models.ResolveBranding("", tenant.Branding, nil),
	}})
}

func PatchTenantBranding(c *fiber.Ctx) error {
	tenantOID, err := brandingTenantID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	var patch models.BrandingPatch
	if err := c.BodyParser(&patch); err != nil {
		return sendBrandingError(c, http.StatusBadRequest, "Invalid request body")
	}
	patch, err = models.ValidateBrandingPatch(patch)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	tenant, err := loadTenant(ctx, tenantOID)
	if err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Tenant not found")
	}
	version := int64(0)
	if tenant.Branding != nil {
		version = tenant.Branding.Version
	}
	set, unset := brandingUpdateDocuments(patch, version, time.Now().UTC())
	if _, err := tenantsCollection().UpdateOne(ctx, bson.M{"_id": tenantOID}, brandingUpdate(set, unset)); err != nil {
		return sendBrandingError(c, http.StatusInternalServerError, "Failed to update tenant branding")
	}
	return GetTenantBranding(c)
}

func GetProjectBranding(c *fiber.Ctx) error {
	tenantOID, err := brandingTenantID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	projectOID, err := brandingProjectID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusForbidden, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	tenant, err := loadTenant(ctx, tenantOID)
	if err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Tenant not found")
	}
	project, err := loadProject(ctx, tenantOID, projectOID)
	if err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Project not found")
	}
	return c.JSON(responses.GeneralResponse{Status: http.StatusOK, Data: brandingManagementResponse{
		Overrides: project.Branding, Effective: models.ResolveBranding(project.Name, tenant.Branding, project.Branding),
	}})
}

func PatchProjectBranding(c *fiber.Ctx) error {
	tenantOID, err := brandingTenantID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	projectOID, err := brandingProjectID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusForbidden, err.Error())
	}
	var patch models.BrandingPatch
	if err := c.BodyParser(&patch); err != nil {
		return sendBrandingError(c, http.StatusBadRequest, "Invalid request body")
	}
	patch, err = models.ValidateBrandingPatch(patch)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	project, err := loadProject(ctx, tenantOID, projectOID)
	if err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Project not found")
	}
	version := int64(0)
	if project.Branding != nil {
		version = project.Branding.Version
	}
	set, unset := brandingUpdateDocuments(patch, version, time.Now().UTC())
	result, err := projectsCollection().UpdateOne(ctx, bson.M{"_id": projectOID, "tenantId": tenantOID}, brandingUpdate(set, unset))
	if err != nil || result.MatchedCount == 0 {
		return sendBrandingError(c, http.StatusInternalServerError, "Failed to update project branding")
	}
	return GetProjectBranding(c)
}

func openBrandingUpload(c *fiber.Ctx) (*multipart.FileHeader, multipart.File, error) {
	header, err := c.FormFile("file")
	if err != nil {
		return nil, nil, fmt.Errorf("branding image file is required")
	}
	file, err := header.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("open branding image: %w", err)
	}
	return header, file, nil
}

func brandingAsset(branding *models.Branding, slot string) *models.BrandingAsset {
	if branding == nil {
		return nil
	}
	switch slot {
	case "logo":
		return branding.Logo
	case "compactLogo":
		return branding.CompactLogo
	case "favicon":
		return branding.Favicon
	default:
		return nil
	}
}

func uploadBrandingAsset(c *fiber.Ctx, tenant bool) error {
	field, err := brandingAssetField(c.Params("slot"))
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	tenantOID, err := brandingTenantID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	var projectOID primitive.ObjectID
	if !tenant {
		projectOID, err = brandingProjectID(c)
		if err != nil {
			return sendBrandingError(c, http.StatusForbidden, err.Error())
		}
	}
	_, file, err := openBrandingUpload(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()
	folder := path.Join("branding", "tenants", tenantOID.Hex())
	if !tenant {
		folder = path.Join(folder, "projects", projectOID.Hex())
	}
	asset, err := brandingAssetStore.Upload(ctx, file, utils.BrandingUploadOptions{
		Folder: folder, PublicID: c.Params("slot") + "-" + uuid.NewString(),
	})
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}

	filter := bson.M{"_id": tenantOID}
	collection := tenantsCollection()
	var old *models.BrandingAsset
	version := int64(0)
	if tenant {
		stored, loadErr := loadTenant(ctx, tenantOID)
		if loadErr != nil {
			_ = brandingAssetStore.Delete(ctx, asset.AssetID)
			return sendBrandingError(c, http.StatusNotFound, "Tenant not found")
		}
		old, version = brandingAsset(stored.Branding, c.Params("slot")), brandingVersion(stored.Branding)
	} else {
		filter = bson.M{"_id": projectOID, "tenantId": tenantOID}
		collection = projectsCollection()
		stored, loadErr := loadProject(ctx, tenantOID, projectOID)
		if loadErr != nil {
			_ = brandingAssetStore.Delete(ctx, asset.AssetID)
			return sendBrandingError(c, http.StatusNotFound, "Project not found")
		}
		old, version = brandingAsset(stored.Branding, c.Params("slot")), brandingVersion(stored.Branding)
	}
	result, err := collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		field: asset, "branding.version": version + 1, "updatedAt": time.Now().UTC(),
	}})
	if err != nil || result.MatchedCount == 0 {
		_ = brandingAssetStore.Delete(ctx, asset.AssetID)
		return sendBrandingError(c, http.StatusInternalServerError, "Failed to save branding asset")
	}
	if old != nil && old.AssetID != "" {
		if err := brandingAssetStore.Delete(ctx, old.AssetID); err != nil {
			log.Printf("failed to delete replaced branding asset %s: %v", old.AssetID, err)
		}
	}
	if tenant {
		return GetTenantBranding(c)
	}
	return GetProjectBranding(c)
}

func brandingVersion(branding *models.Branding) int64 {
	if branding == nil {
		return 0
	}
	return branding.Version
}

func UploadTenantBrandingAsset(c *fiber.Ctx) error  { return uploadBrandingAsset(c, true) }
func UploadProjectBrandingAsset(c *fiber.Ctx) error { return uploadBrandingAsset(c, false) }

func deleteBrandingAsset(c *fiber.Ctx, tenant bool) error {
	field, err := brandingAssetField(c.Params("slot"))
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	tenantOID, err := brandingTenantID(c)
	if err != nil {
		return sendBrandingError(c, http.StatusBadRequest, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	filter := bson.M{"_id": tenantOID}
	collection := tenantsCollection()
	var old *models.BrandingAsset
	version := int64(0)
	if tenant {
		stored, loadErr := loadTenant(ctx, tenantOID)
		if loadErr != nil {
			return sendBrandingError(c, http.StatusNotFound, "Tenant not found")
		}
		old, version = brandingAsset(stored.Branding, c.Params("slot")), brandingVersion(stored.Branding)
	} else {
		projectOID, projectErr := brandingProjectID(c)
		if projectErr != nil {
			return sendBrandingError(c, http.StatusForbidden, projectErr.Error())
		}
		filter = bson.M{"_id": projectOID, "tenantId": tenantOID}
		collection = projectsCollection()
		stored, loadErr := loadProject(ctx, tenantOID, projectOID)
		if loadErr != nil {
			return sendBrandingError(c, http.StatusNotFound, "Project not found")
		}
		old, version = brandingAsset(stored.Branding, c.Params("slot")), brandingVersion(stored.Branding)
	}
	result, err := collection.UpdateOne(ctx, filter, bson.M{
		"$unset": bson.M{field: ""},
		"$set":   bson.M{"branding.version": version + 1, "updatedAt": time.Now().UTC()},
	})
	if err != nil || result.MatchedCount == 0 {
		return sendBrandingError(c, http.StatusInternalServerError, "Failed to remove branding asset")
	}
	if old != nil && old.AssetID != "" {
		if err := brandingAssetStore.Delete(ctx, old.AssetID); err != nil {
			log.Printf("failed to delete removed branding asset %s: %v", old.AssetID, err)
		}
	}
	if tenant {
		return GetTenantBranding(c)
	}
	return GetProjectBranding(c)
}

func DeleteTenantBrandingAsset(c *fiber.Ctx) error  { return deleteBrandingAsset(c, true) }
func DeleteProjectBrandingAsset(c *fiber.Ctx) error { return deleteBrandingAsset(c, false) }

func GetRuntimeBranding(c *fiber.Ctx) error {
	tenantSlug := strings.TrimSpace(c.Params("tenantSlug"))
	projectSlug := strings.TrimSpace(c.Params("projectSlug"))
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	var tenant models.Tenant
	if err := tenantsCollection().FindOne(ctx, bson.M{"slug": tenantSlug, "isActive": true}).Decode(&tenant); err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Project not found")
	}
	var project models.Project
	if err := projectsCollection().FindOne(ctx, bson.M{
		"slug": projectSlug, "tenantId": tenant.ID, "isActive": true,
	}).Decode(&project); err != nil {
		return sendBrandingError(c, http.StatusNotFound, "Project not found")
	}
	etag := fmt.Sprintf(`W/"branding-%d-%d"`, brandingVersion(tenant.Branding), brandingVersion(project.Branding))
	c.Set(fiber.HeaderETag, etag)
	if c.Get(fiber.HeaderIfNoneMatch) == etag {
		return c.SendStatus(http.StatusNotModified)
	}
	return c.JSON(fiber.Map{"data": models.ResolveBranding(project.Name, tenant.Branding, project.Branding)})
}
