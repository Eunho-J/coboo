package store

import "testing"

func TestScopesConflict(t *testing.T) {
	testCases := []struct {
		name              string
		newScopeType      string
		newScopePath      string
		existingScopeType string
		existingScopePath string
		expectedConflict  bool
	}{
		{
			name:              "file vs same file",
			newScopeType:      "file",
			newScopePath:      "src/api/users.go",
			existingScopeType: "file",
			existingScopePath: "src/api/users.go",
			expectedConflict:  true,
		},
		{
			name:              "file vs sibling file",
			newScopeType:      "file",
			newScopePath:      "src/api/users.go",
			existingScopeType: "file",
			existingScopePath: "src/api/posts.go",
			expectedConflict:  false,
		},
		{
			name:              "prefix vs file under prefix",
			newScopeType:      "prefix",
			newScopePath:      "src/api",
			existingScopeType: "file",
			existingScopePath: "src/api/users.go",
			expectedConflict:  true,
		},
		{
			name:              "prefix vs non-overlapping prefix",
			newScopeType:      "prefix",
			newScopePath:      "src/ui",
			existingScopeType: "prefix",
			existingScopePath: "src/api",
			expectedConflict:  false,
		},
		{
			name:              "prefix overlap",
			newScopeType:      "prefix",
			newScopePath:      "src/api",
			existingScopeType: "prefix",
			existingScopePath: "src/api/v1",
			expectedConflict:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			conflict := scopesConflict(
				testCase.newScopeType,
				testCase.newScopePath,
				testCase.existingScopeType,
				testCase.existingScopePath,
			)
			if conflict != testCase.expectedConflict {
				t.Fatalf("expected conflict=%v, got %v", testCase.expectedConflict, conflict)
			}
		})
	}
}
