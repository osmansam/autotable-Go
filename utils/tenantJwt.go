package utils

import (
	"errors"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var tenantJwtSecret = []byte(os.Getenv("TENANT_JWT_SECRET"))

// TenantTokenClaims represents the claims stored in a tenant user JWT
type TenantTokenClaims struct {
	UserID    string   `json:"user_id"`
	Email     string   `json:"email"`
	TenantID  string   `json:"tenant_id"`
	ProjectID string   `json:"project_id,omitempty"` // Optional, for project-scoped operations
	Roles     []string `json:"roles"`                // Can be tenant or project roles
	RoleScope string   `json:"role_scope"`           // "tenant" or "project"
	jwt.StandardClaims
}

// TenantTokenDetails holds both access and refresh tokens for tenant users
type TenantTokenDetails struct {
	AccessToken  string
	RefreshToken string
	ATExpires    int64
	RTExpires    int64
}

// GenerateTenantTokens generates access and refresh tokens for a tenant user
// For tenant-level access, pass empty string for projectID
// For project-level access, pass the projectID
func GenerateTenantTokens(userID, email, tenantID, projectID string, roles []string, roleScope string) (*TenantTokenDetails, error) {
	// Validate tenant JWT secret
	if len(tenantJwtSecret) == 0 {
		return nil, errors.New("TENANT_JWT_SECRET not set in environment")
	}

	td := &TenantTokenDetails{}
	td.ATExpires = time.Now().Add(time.Hour * 24).Unix()       // 24 hours
	td.RTExpires = time.Now().Add(time.Hour * 24 * 7).Unix()   // 7 days

	// Access Token
	atClaims := TenantTokenClaims{
		UserID:    userID,
		Email:     email,
		TenantID:  tenantID,
		ProjectID: projectID,
		Roles:     roles,
		RoleScope: roleScope,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: td.ATExpires,
			IssuedAt:  time.Now().Unix(),
		},
	}

	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	accessToken, err := at.SignedString(tenantJwtSecret)
	if err != nil {
		return nil, err
	}
	td.AccessToken = accessToken

	// Refresh Token
	rtClaims := TenantTokenClaims{
		UserID:    userID,
		Email:     email,
		TenantID:  tenantID,
		ProjectID: projectID,
		Roles:     roles,
		RoleScope: roleScope,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: td.RTExpires,
			IssuedAt:  time.Now().Unix(),
		},
	}

	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rtClaims)
	refreshToken, err := rt.SignedString(tenantJwtSecret)
	if err != nil {
		return nil, err
	}
	td.RefreshToken = refreshToken

	return td, nil
}

// ParseTenantToken validates and parses a tenant user JWT token
func ParseTenantToken(tokenString string) (*TenantTokenClaims, error) {
	if len(tenantJwtSecret) == 0 {
		return nil, errors.New("TENANT_JWT_SECRET not set in environment")
	}

	token, err := jwt.ParseWithClaims(tokenString, &TenantTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return tenantJwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*TenantTokenClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// ValidateTenantMembership checks if user has active membership in the tenant
func ValidateTenantMembership(userID, tenantID primitive.ObjectID) ([]string, error) {
	// This will query the TenantMembership collection
	// Returns the user's roles in the tenant
	// Implementation will be in the controller
	return nil, errors.New("not implemented - use in controller")
}

// ValidateProjectMembership checks if user has active membership in the project
func ValidateProjectMembership(userID, tenantID, projectID primitive.ObjectID) ([]string, error) {
	// This will query the ProjectMembership collection
	// Returns the user's roles in the project
	// Implementation will be in the controller
	return nil, errors.New("not implemented - use in controller")
}
