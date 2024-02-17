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
	BaseUrl	 	string `json:"baseUrl"`
}

type Role struct {
	Name        string `json:"name"`
	Permissions []string `json:"permissions"`
}

func ParseContainerToken(tokenStr string) ([]string, error) {
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

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	//TODO: Change to the correct environment variable 
	envBaseUrl := os.Getenv("PORT_NUMBER") 

	// Iterate through projects to find a match for the BaseUrl
	for _, projectRole := range claims.Projects {
		if projectRole.Project.BaseUrl == envBaseUrl {
			// Found a project with a matching BaseUrl, return its permissions
			return projectRole.Role.Permissions, nil
		}
	}

	// If no project matches the expected BaseUrl, return an error
	return nil, errors.New("no project matches the expected BaseUrl")
}