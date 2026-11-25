package main

import (
	"bytes"
	"github.com/osmansam/autotableGo/configs"
	"os"
	"os/exec"
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

func TestCheckModeValidatesWithoutGeneratingOrPrintingKeys(t *testing.T) {
	const project = "b2615c0e92f7483ad6810c9e7f024568a5e330c19bf607d40e8a672d195b3fc4"
	const tenant = "c5ae13f4d06897b22c8e106fa471d3506e9c08ab32745f16b09d8e6a1257f3c0"
	t.Setenv("JWT_SECRET", project)
	t.Setenv("TENANT_JWT_SECRET", tenant)
	t.Setenv("CONTAINER_JWT_SECRET", "")
	t.Setenv("OBSERVABILITY_TOKEN", "")
	t.Setenv("EXTERNAL_API_CREDENTIAL_KEY", "")
	t.Setenv("NODE_ENV", "development")
	var out bytes.Buffer
	if err := runSecretCommand([]string{"--check"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), project) || strings.Contains(out.String(), tenant) || strings.Contains(out.String(), "JWT_SECRET=") {
		t.Fatal("check mode exposed or generated credentials")
	}
	t.Setenv("JWT_SECRET", "weak")
	out.Reset()
	if err := runSecretCommand([]string{"--check"}, &out); err == nil || out.Len() != 0 {
		t.Fatal("weak configuration passed or printed credentials")
	}
	t.Setenv("JWT_SECRET", project)
	t.Setenv("OBSERVABILITY_TOKEN", project)
	if err := runSecretCommand([]string{"--check"}, &out); err == nil {
		t.Fatal("reused diagnostics credential passed")
	}
	t.Setenv("OBSERVABILITY_TOKEN", "")
	t.Setenv("EXTERNAL_API_CREDENTIAL_KEY", "not-base64")
	if err := runSecretCommand([]string{"--check"}, &out); err == nil {
		t.Fatal("invalid encryption key passed")
	}
	if err := runSecretCommand([]string{"--unknown"}, &out); err == nil {
		t.Fatal("unknown option passed")
	}
}

func TestSecretModesDoNotInitializeCloudinaryOrExposeCloudCredentials(t *testing.T) {
	if mode := os.Getenv("AUTOTABLE_TEST_SECRET_MODE"); mode != "" {
		os.Args = []string{"autotable-secrets"}
		if mode == "check" {
			os.Args = append(os.Args, "--check")
		}
		main()
		return
	}
	for _, mode := range []string{"check", "generate"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSecretModesDoNotInitializeCloudinaryOrExposeCloudCredentials$")
		cmd.Env = append(os.Environ(), "AUTOTABLE_TEST_SECRET_MODE="+mode, "CLOUD_NAME=invalid%xy", "CLOUD_API_KEY=SYNTHETIC_CLOUD_KEY_MARKER", "CLOUD_API_SECRET=SYNTHETIC_CLOUD_SECRET_MARKER", "JWT_SECRET=b2615c0e92f7483ad6810c9e7f024568a5e330c19bf607d40e8a672d195b3fc4", "TENANT_JWT_SECRET=c5ae13f4d06897b22c8e106fa471d3506e9c08ab32745f16b09d8e6a1257f3c0", "CONTAINER_JWT_SECRET=", "OBSERVABILITY_TOKEN=", "EXTERNAL_API_CREDENTIAL_KEY=")
		output, err := cmd.CombinedOutput()
		if strings.Contains(string(output), "SYNTHETIC_CLOUD_KEY_MARKER") || strings.Contains(string(output), "SYNTHETIC_CLOUD_SECRET_MARKER") {
			t.Fatalf("%s mode exposed unrelated cloud credentials", mode)
		}
		if err != nil {
			t.Fatalf("%s mode initialized an unrelated cloud integration", mode)
		}
	}
}
