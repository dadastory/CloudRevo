package constants

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDockerBuildDefaultsUseBackendMigrationVersion(t *testing.T) {
	contents, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	versions := regexp.MustCompile(`ARG VERSION=([^\r\n]+)`).FindAllStringSubmatch(string(contents), -1)
	if len(versions) != 2 {
		t.Fatalf("expected frontend and backend VERSION defaults, got %#v", versions)
	}
	for _, match := range versions {
		if match[1] != BackendVersion {
			t.Fatalf("Docker default version %q must match migration version %q", match[1], BackendVersion)
		}
	}
}

func TestFrontendPackageVersionUsesBackendMigrationVersion(t *testing.T) {
	contents, err := os.ReadFile("../../assets/package.json")
	if err != nil {
		t.Fatalf("read frontend package manifest: %v", err)
	}
	expected := `"version": "` + BackendVersion + `"`
	if !strings.Contains(string(contents), expected) {
		t.Fatalf("frontend package version must match migration version %q", BackendVersion)
	}
}
