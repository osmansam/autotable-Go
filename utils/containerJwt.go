package utils

import (
	"errors"
	"os"

	"github.com/dgrijalva/jwt-go"
)


type CustomClaims struct {
	UserId    string `json:"userId"`
	Email     string `json:"email"`
	Projects  []ProjectRole `json:"projects"` 
	jwt.StandardClaims
}


type ProjectRole struct {
	Project Project `json:"project"`
	Role    Role    `json:"role"`
}


type Project struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}


type Role struct {
	Name        string `json:"name"`
	Permissions []string `json:"permissions"`
}


func ParseContainerToken(tokenStr string) (*CustomClaims, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(os.Getenv("CONTAINER_JWT_SECRET")), nil
	}

	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, keyFunc)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	} else {
		return nil, errors.New("invalid token")
	}
}
