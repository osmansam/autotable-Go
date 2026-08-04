package controllers

import "testing"

func TestValidateProjectLocaleInputRequiresSourceAndDefaultEnabled(t *testing.T) {
	if err := validateProjectLocaleInput(projectLocaleInput{SourceLocale: "en", DefaultLocale: "tr", EnabledLocales: []string{"en"}}); err == nil {
		t.Fatal("expected disabled default locale error")
	}
	if err := validateProjectLocaleInput(projectLocaleInput{SourceLocale: "en", DefaultLocale: "tr", EnabledLocales: []string{"en", "tr"}}); err != nil {
		t.Fatalf("valid locale input rejected: %v", err)
	}
}
