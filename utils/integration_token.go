package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/osmansam/autotableGo/models"
)

const IntegrationTokenPrefix = "ati_"

var (
	ErrIntegrationTenantProjectMismatch = errors.New("integration token is not valid for this tenant/project")
	ErrIntegrationTokenExpired          = errors.New("integration token expired")
	ErrIntegrationTokenRevoked          = errors.New("integration token revoked")
	ErrIntegrationPermissionDenied      = errors.New("integration token permission denied")
)

func GenerateIntegrationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return IntegrationTokenPrefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func LooksLikeIntegrationToken(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), IntegrationTokenPrefix)
}

func HashIntegrationToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func IntegrationPermissionAllowed(permissions []models.IntegrationPermission, required models.IntegrationPermission) bool {
	required = normalizeIntegrationPermission(required)
	if required.Kind == "" || required.SchemaName == "" || required.Method == "" {
		return false
	}

	for _, permission := range permissions {
		permission = normalizeIntegrationPermission(permission)
		if permission.Kind != required.Kind || permission.SchemaName != required.SchemaName || permission.Method != required.Method {
			continue
		}
		if required.Kind == models.IntegrationPermissionKindDynamicRoute && permission.Route == required.Route {
			return true
		}
		if required.Kind != models.IntegrationPermissionKindDynamicRoute && permission.Name == required.Name {
			return true
		}
	}

	return false
}

func IntegrationPermissionForDynamicRoute(routeName, schemaName, method, workflowName, apiName, pipelineName string) models.IntegrationPermission {
	permission := models.IntegrationPermission{
		Kind:       models.IntegrationPermissionKindDynamicRoute,
		SchemaName: schemaName,
		Route:      routeName,
		Method:     method,
	}

	switch routeName {
	case "ExecuteWorkflow":
		permission.Kind = models.IntegrationPermissionKindWorkflow
		permission.Route = ""
		permission.Name = workflowName
	case "ExecuteDynamicAPI":
		permission.Kind = models.IntegrationPermissionKindAPI
		permission.Route = ""
		permission.Name = apiName
	case "GetPipeline", "TestPipeline":
		permission.Kind = models.IntegrationPermissionKindPipeline
		permission.Route = ""
		permission.Name = pipelineName
	}

	return normalizeIntegrationPermission(permission)
}

func ValidateIntegrationCredentialAccess(credential models.IntegrationCredential, tenantID, projectID string, required models.IntegrationPermission, now time.Time) error {
	if credential.TenantID.Hex() != strings.TrimSpace(tenantID) || credential.ProjectID.Hex() != strings.TrimSpace(projectID) {
		return ErrIntegrationTenantProjectMismatch
	}
	if credential.RevokedAt != nil {
		return ErrIntegrationTokenRevoked
	}
	if !credential.ExpiresAt.IsZero() && !credential.ExpiresAt.After(now) {
		return ErrIntegrationTokenExpired
	}
	if !IntegrationPermissionAllowed(credential.Permissions, required) {
		return ErrIntegrationPermissionDenied
	}
	return nil
}

func normalizeIntegrationPermission(permission models.IntegrationPermission) models.IntegrationPermission {
	permission.Kind = strings.TrimSpace(permission.Kind)
	permission.SchemaName = strings.TrimSpace(permission.SchemaName)
	permission.Route = strings.TrimSpace(permission.Route)
	permission.Name = strings.TrimSpace(permission.Name)
	permission.Method = strings.ToUpper(strings.TrimSpace(permission.Method))
	return permission
}
