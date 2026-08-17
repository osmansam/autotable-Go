package requests

import (
	"strings"
	"testing"
)

func TestParseArrayRowMutationAcceptsAddAndUpdate(t *testing.T) {
	tests := []struct {
		name      string
		operation ArrayMutationOperation
		body      string
	}{
		{name: "add", operation: ArrayMutationAdd, body: `{"rowIdentityField":"duty","item":{"duty":"Clean","order":0}}`},
		{name: "update", operation: ArrayMutationUpdate, body: `{"rowIdentityField":"duty","updates":{"description":"Closing task"}}`},
		{name: "delete", operation: ArrayMutationDelete, body: `{"rowIdentityField":"duty"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := ParseArrayRowMutation(strings.NewReader(tt.body), tt.operation)
			if err != nil {
				t.Fatalf("ParseArrayRowMutation() error = %v", err)
			}
			if request.RowIdentityField != "duty" {
				t.Fatalf("RowIdentityField = %q, want duty", request.RowIdentityField)
			}
		})
	}
}

func TestParseArrayRowMutationRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name      string
		operation ArrayMutationOperation
		body      string
	}{
		{name: "blank identity", operation: ArrayMutationAdd, body: `{"rowIdentityField":" ","item":{"duty":"Clean"}}`},
		{name: "add missing item", operation: ArrayMutationAdd, body: `{"rowIdentityField":"duty"}`},
		{name: "update missing updates", operation: ArrayMutationUpdate, body: `{"rowIdentityField":"duty"}`},
		{name: "empty update", operation: ArrayMutationUpdate, body: `{"rowIdentityField":"duty","updates":{}}`},
		{name: "unknown property", operation: ArrayMutationAdd, body: `{"rowIdentityField":"duty","item":{"duty":"Clean"},"operator":{"$set":{}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseArrayRowMutation(strings.NewReader(tt.body), tt.operation); err == nil {
				t.Fatal("ParseArrayRowMutation() error = nil")
			}
		})
	}
}

func TestParseArrayReorderAcceptsUniqueScalarIdentities(t *testing.T) {
	request, err := ParseArrayReorder(strings.NewReader(`{"rowIdentityField":"duty","orderField":"order","rowIdentities":["Open","Clean"]}`))
	if err != nil {
		t.Fatalf("ParseArrayReorder() error = %v", err)
	}
	if len(request.RowIdentities) != 2 || request.RowIdentities[1] != "Clean" {
		t.Fatalf("RowIdentities = %#v", request.RowIdentities)
	}
}

func TestParseArrayReorderRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "blank identity field", body: `{"rowIdentityField":" ","orderField":"order","rowIdentities":["Open"]}`},
		{name: "blank order field", body: `{"rowIdentityField":"duty","orderField":" ","rowIdentities":["Open"]}`},
		{name: "empty identities", body: `{"rowIdentityField":"duty","orderField":"order","rowIdentities":[]}`},
		{name: "duplicate identities", body: `{"rowIdentityField":"duty","orderField":"order","rowIdentities":["Open","Open"]}`},
		{name: "structured identity", body: `{"rowIdentityField":"duty","orderField":"order","rowIdentities":[{"value":"Open"}]}`},
		{name: "unknown property", body: `{"rowIdentityField":"duty","orderField":"order","rowIdentities":["Open"],"extra":true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseArrayReorder(strings.NewReader(tt.body)); err == nil {
				t.Fatal("ParseArrayReorder() error = nil")
			}
		})
	}
}
