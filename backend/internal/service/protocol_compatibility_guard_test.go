//go:build unit

package service

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolCompatibilityGuard_NoDetectorSpecificProductionBranches(t *testing.T) {
	t.Parallel()

	forbidden := []string{"ztest", "ztest-probe"}
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(contents))
		for _, marker := range forbidden {
			require.NotContainsf(
				t,
				lower,
				marker,
				"production protocol behavior must not identify or special-case a detector: %s",
				path,
			)
		}
		return nil
	})
	require.NoError(t, err)
}
