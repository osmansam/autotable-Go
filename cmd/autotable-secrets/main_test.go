package main

import (
	"bytes"
	"github.com/osmansam/autotableGo/configs"
	"strings"
	"testing"
)

func TestGeneratedSecretsAreStrongDistinctAndNotReused(t *testing.T) {
	var first, second bytes.Buffer
	if err := writeSecrets(&first); err != nil {
		t.Fatal(err)
	}
	if err := writeSecrets(&second); err != nil {
		t.Fatal(err)
	}
	if first.String() == second.String() {
		t.Fatal("generated credentials reused")
	}
	lines := strings.Split(strings.TrimSpace(first.String()), "\n")
	if len(lines) != 2 {
		t.Fatal("wrong number of credentials")
	}
	values := map[string]string{}
	for _, line := range lines {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatal("invalid env output")
		}
		if _, err := configs.ValidateJWTSecret(value); err != nil {
			t.Fatal(err)
		}
		values[name] = value
	}
	if len(values["JWT_SECRET"]) != 64 || len(values["TENANT_JWT_SECRET"]) != 64 || values["JWT_SECRET"] == values["TENANT_JWT_SECRET"] {
		t.Fatal("invalid generated credentials")
	}
}
