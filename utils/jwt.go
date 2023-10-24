package utils

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))  

func CreateJWT(user string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["user"] = user
	claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

	t, err := token.SignedString(jwtSecret)
	return t, err
}

func ParseJWT(tokenStr string) (string, error) {
	// Remove the "Bearer " prefix
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["user"].(string)
		if !ok {
			return "", errors.New("user claim is missing from token")
		}
		return userID, nil
	}

	return "", errors.New("invalid token")
}

