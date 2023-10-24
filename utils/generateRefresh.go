package utils

import (
	"crypto/rand"
	"encoding/base64"
)

func GenerateRefreshToken() string {
	tokenLength := 32
	randomBytes := make([]byte, tokenLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic("Failed to generate refresh token.")
	}
	return base64.URLEncoding.EncodeToString(randomBytes)
}
