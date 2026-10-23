package controllers

import (
	"regexp"
	"testing"
)

func TestProjectCollectionNamePatternOnlyMatchesOwnedCollections(t *testing.T) {
	tenantID := "65a1b2c3d4e5f67890abcdef"
	projectID := "78f9e8d7c6b5a43210fedcba"
	re := regexp.MustCompile(projectCollectionNamePattern(tenantID, projectID))

	owned := []string{
		"tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_containers",
		"tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_pages",
		"tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_products",
	}
	for _, name := range owned {
		if !re.MatchString(name) {
			t.Errorf("expected %q to match", name)
		}
	}

	notOwned := []string{
		"projects",
		"containers",
		"tenant_65a1b2c3d4e5f67890abcdef_project_000000000000000000000000_pages",
		"tenant_000000000000000000000000_project_78f9e8d7c6b5a43210fedcba_pages",
		"x_tenant_65a1b2c3d4e5f67890abcdef_project_78f9e8d7c6b5a43210fedcba_pages",
	}
	for _, name := range notOwned {
		if re.MatchString(name) {
			t.Errorf("expected %q not to match", name)
		}
	}
}

