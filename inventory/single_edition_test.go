package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleEditionRuntimeDoesNotRetainProCompatibility(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path      string
		forbidden []string
	}{
		{
			path: "../application/constants/constants.go",
			forbidden: []string{
				"IsPro",
			},
		},
		{
			path: "../cmd/server.go",
			forbidden: []string{
				"license-key",
				"WithProFlag",
			},
		},
		{
			path: "../application/statics/statics.go",
			forbidden: []string{
				"isPro",
				"cloudrevo-frontend-pro",
			},
		},
		{
			path: "migration.go",
			forbidden: []string{
				"TrimSuffix",
			},
		},
		{
			path: "setting.go",
			forbidden: []string{
				"\"license\"",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			t.Parallel()

			contents, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}

			for _, forbidden := range tc.forbidden {
				if strings.Contains(string(contents), forbidden) {
					t.Errorf("%s still contains obsolete single-edition compatibility token %q", tc.path, forbidden)
				}
			}
		})
	}
}
