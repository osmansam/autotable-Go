package utils

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

type TokenDetails struct {
	AccessToken  string
	RefreshToken string
	ATExpires    int64
	RTExpires    int64
}

const (
	TokenTypeAccess      = "access"
	TokenTypeRefresh     = "refresh"
	ProjectTokenAudience = "autoapi-project"
)

type ProjectTokenClaims struct {
	UserID      string
	Role        string
	TenantID    string
	ProjectID   string
	TenantSlug  string
	ProjectSlug string
	DisplayName string
	TokenType   string
	Scope       string
	Audience    string
	ID          string
	FamilyID    string
	ExpiresAt   int64
}

func GenerateTokens(userID string, role string, tenantID string, projectID string, tenantSlug string, projectSlug string) (*TokenDetails, error) {
	return GenerateTokensWithDisplayName(userID, role, tenantID, projectID, tenantSlug, projectSlug, "")
}

func GenerateTokensWithDisplayName(userID string, role string, tenantID string, projectID string, tenantSlug string, projectSlug string, displayName string) (*TokenDetails, error) {
	return GenerateTokensForFamily(userID, role, tenantID, projectID, tenantSlug, projectSlug, displayName, uuid.NewString())
}

func GenerateTokensForFamily(userID string, role string, tenantID string, projectID string, tenantSlug string, projectSlug string, displayName string, familyID string) (*TokenDetails, error) {
	td := &TokenDetails{}
	td.ATExpires = time.Now().Add(time.Hour * 24).Unix()     // 24 hours validity
	td.RTExpires = time.Now().Add(time.Hour * 24 * 7).Unix() // 7 days validity

	var err error
	// Access Token
	atClaims := jwt.MapClaims{}
	atClaims["authorized"] = true
	atClaims["user_id"] = userID
	atClaims["role"] = role
	atClaims["tenant_id"] = tenantID
	atClaims["project_id"] = projectID
	atClaims["tenant_slug"] = tenantSlug
	atClaims["project_slug"] = projectSlug
	atClaims["token_type"] = TokenTypeAccess
	atClaims["scope"] = TokenScopeProject
	atClaims["aud"] = ProjectTokenAudience
	atClaims["jti"] = uuid.NewString()
	atClaims["family_id"] = familyID
	if displayName != "" {
		atClaims["display_name"] = displayName
	}
	atClaims["exp"] = td.ATExpires
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	td.AccessToken, err = at.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}

	// Refresh Token
	rtClaims := jwt.MapClaims{}
	rtClaims["user_id"] = userID
	rtClaims["role"] = role
	rtClaims["tenant_id"] = tenantID
	rtClaims["project_id"] = projectID
	rtClaims["tenant_slug"] = tenantSlug
	rtClaims["project_slug"] = projectSlug
	rtClaims["token_type"] = TokenTypeRefresh
	rtClaims["scope"] = TokenScopeProject
	rtClaims["aud"] = ProjectTokenAudience
	rtClaims["jti"] = uuid.NewString()
	rtClaims["family_id"] = familyID
	if displayName != "" {
		rtClaims["display_name"] = displayName
	}
	rtClaims["exp"] = td.RTExpires
	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rtClaims)
	td.RefreshToken, err = rt.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}

	return td, nil
}

func ParseToken(tokenStr string) (userID, role, tenantID, projectID, tenantSlug, projectSlug string, err error) {
	claims, err := parseProjectTokenClaims(tokenStr, TokenTypeAccess)
	if err != nil {
		return "", "", "", "", "", "", err
	}
	return claims.UserID, claims.Role, claims.TenantID, claims.ProjectID, claims.TenantSlug, claims.ProjectSlug, nil
}

func ParseProjectRefreshToken(tokenStr string) (*ProjectTokenClaims, error) {
	return parseProjectTokenClaims(tokenStr, TokenTypeRefresh)
}

func parseProjectTokenClaims(tokenStr, expectedType string) (*ProjectTokenClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claimString := func(name string, required bool) (string, error) {
		value, ok := claims[name].(string)
		if !ok && required {
			return "", fmt.Errorf("%s claim is missing from token", name)
		}
		return value, nil
	}
	userID, err := claimString("user_id", true)
	if err != nil {
		return nil, err
	}
	role, err := claimString("role", true)
	if err != nil {
		return nil, err
	}
	tenantID, err := claimString("tenant_id", true)
	if err != nil {
		return nil, err
	}
	projectID, err := claimString("project_id", true)
	if err != nil {
		return nil, err
	}
	tokenType, err := claimString("token_type", true)
	if err != nil || tokenType != expectedType {
		return nil, errors.New("invalid token type")
	}
	scope, err := claimString("scope", true)
	if err != nil || scope != TokenScopeProject {
		return nil, errors.New("invalid token scope")
	}
	audience, err := claimString("aud", true)
	if err != nil || audience != ProjectTokenAudience {
		return nil, errors.New("invalid token audience")
	}
	tokenID, err := claimString("jti", true)
	if err != nil {
		return nil, err
	}
	familyID, err := claimString("family_id", true)
	if err != nil {
		return nil, err
	}
	expiresAt, _ := claims["exp"].(float64)
	tenantSlug, _ := claimString("tenant_slug", false)
	projectSlug, _ := claimString("project_slug", false)
	displayName, _ := claimString("display_name", false)
	return &ProjectTokenClaims{
		UserID: userID, Role: role, TenantID: tenantID, ProjectID: projectID,
		TenantSlug: tenantSlug, ProjectSlug: projectSlug, DisplayName: displayName,
		TokenType: tokenType, Scope: scope, Audience: audience, ID: tokenID,
		FamilyID: familyID, ExpiresAt: int64(expiresAt),
	}, nil
}

func ParseTokenDisplayName(tokenStr string) string {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	displayName, _ := claims["display_name"].(string)
	return displayName
}
