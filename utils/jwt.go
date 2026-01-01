package utils

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

type TokenDetails struct {
	AccessToken  string
	RefreshToken string
	ATExpires    int64 
	RTExpires    int64 
}

func GenerateTokens(userID string, role string, tenantID string, projectID string) (*TokenDetails, error) {
	td := &TokenDetails{}
	td.ATExpires = time.Now().Add(time.Hour * 24).Unix() // 24 hours validity 
	td.RTExpires = time.Now().Add(time.Hour * 24 * 7).Unix() // 7 days validity 

	var err error
	// Access Token
	atClaims := jwt.MapClaims{}
	atClaims["authorized"] = true
	atClaims["user_id"] = userID
	atClaims["role"] = role
	atClaims["tenant_id"] = tenantID
	atClaims["project_id"] = projectID
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
	rtClaims["exp"] = td.RTExpires
	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rtClaims)
	td.RefreshToken, err = rt.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}

	return td, nil
}

func ParseToken(tokenStr string) (userID, role, tenantID, projectID string, err error) {
    token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return jwtSecret, nil
    })

    if err != nil {
        return "", "", "", "", err
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok || !token.Valid {
        return "", "", "", "", errors.New("invalid token")
    }

    userID, ok = claims["user_id"].(string)
    if !ok {
        return "", "", "", "", errors.New("user_id claim is missing from token")
    }
    role, ok = claims["role"].(string)
    if !ok {
        return "", "", "", "", errors.New("role claim is missing from token")
    }
    tenantID, ok = claims["tenant_id"].(string)
    if !ok {
        return "", "", "", "", errors.New("tenant_id claim is missing from token")
    }
    projectID, ok = claims["project_id"].(string)
    if !ok {
        return "", "", "", "", errors.New("project_id claim is missing from token")
    }

    return userID, role, tenantID, projectID, nil
}

