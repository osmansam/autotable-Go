package controllers

import (
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestExternalAPICredentialResponseRedactsSecret(t *testing.T) {
	credential := models.ExternalAPICredential{
		ID:              primitive.NewObjectID(),
		Name:            "Stripe",
		AuthType:        models.ExternalAPIAuthTypeBearer,
		EncryptedSecret: "ciphertext",
		AllowedDomains:  []string{"api.stripe.com"},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	got := externalAPICredentialResponse(credential)
	if got.EncryptedSecret != "" {
		t.Fatalf("externalAPICredentialResponse() exposed encrypted secret: %#v", got)
	}
	if got.Name != credential.Name || len(got.AllowedDomains) != 1 {
		t.Fatalf("externalAPICredentialResponse() = %#v, want metadata", got)
	}
}

func TestValidateExternalAPICredentialInputNormalizesDomains(t *testing.T) {
	input := CreateExternalAPICredentialInput{
		Name:           "Stripe",
		AuthType:       models.ExternalAPIAuthTypeBearer,
		Secret:         "sk_test",
		AllowedDomains: []string{"https://API.Stripe.com/v1", "checkout.stripe.com"},
	}

	got, err := validateExternalAPICredentialInput(input, time.Now())
	if err != nil {
		t.Fatalf("validateExternalAPICredentialInput() error = %v", err)
	}
	want := []string{"api.stripe.com", "checkout.stripe.com"}
	if len(got.AllowedDomains) != len(want) || got.AllowedDomains[0] != want[0] || got.AllowedDomains[1] != want[1] {
		t.Fatalf("AllowedDomains = %#v, want %#v", got.AllowedDomains, want)
	}
}

func TestValidateExternalAPICredentialInputRejectsInvalidValues(t *testing.T) {
	valid := CreateExternalAPICredentialInput{
		Name:           "Provider",
		AuthType:       models.ExternalAPIAuthTypeBearer,
		Secret:         "secret",
		AllowedDomains: []string{"api.example.com"},
	}
	tests := []struct {
		name  string
		input CreateExternalAPICredentialInput
	}{
		{name: "missing name", input: CreateExternalAPICredentialInput{AuthType: valid.AuthType, Secret: valid.Secret, AllowedDomains: valid.AllowedDomains}},
		{name: "missing secret", input: CreateExternalAPICredentialInput{Name: valid.Name, AuthType: valid.AuthType, AllowedDomains: valid.AllowedDomains}},
		{name: "bad auth type", input: CreateExternalAPICredentialInput{Name: valid.Name, AuthType: "basic", Secret: valid.Secret, AllowedDomains: valid.AllowedDomains}},
		{name: "header auth missing header name", input: CreateExternalAPICredentialInput{Name: valid.Name, AuthType: models.ExternalAPIAuthTypeHeader, Secret: valid.Secret, AllowedDomains: valid.AllowedDomains}},
		{name: "authorization custom header", input: CreateExternalAPICredentialInput{Name: valid.Name, AuthType: models.ExternalAPIAuthTypeHeader, HeaderName: "Authorization", Secret: valid.Secret, AllowedDomains: valid.AllowedDomains}},
		{name: "missing domains", input: CreateExternalAPICredentialInput{Name: valid.Name, AuthType: valid.AuthType, Secret: valid.Secret}},
		{name: "expired", input: CreateExternalAPICredentialInput{Name: valid.Name, AuthType: valid.AuthType, Secret: valid.Secret, AllowedDomains: valid.AllowedDomains, ExpiresAt: time.Now().Add(-time.Hour)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := validateExternalAPICredentialInput(tt.input, time.Now()); err == nil {
				t.Fatal("validateExternalAPICredentialInput() error = nil")
			}
		})
	}
}
