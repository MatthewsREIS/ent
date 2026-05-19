// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunFixture_SmokePrivacy is a smoke test: it runs the bench against the
// in-repo privacy fixture and asserts the Run struct is populated. It does
// NOT pin specific numbers (they're machine-dependent); it only verifies the
// pipeline produces non-zero metrics end to end.
func TestRunFixture_SmokePrivacy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bench requires POSIX rusage")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	repoRoot := repoRootForTest(t)
	run, err := RunFixture(repoRoot, Fixture{
		Name:      "privacy",
		SchemaDir: "entc/integration/privacy/ent/schema",
	})
	require.NoError(t, err, "bench should complete")
	require.Equal(t, "privacy", run.Fixture)
	require.Greater(t, run.GenWallNS, int64(0), "gen wall should be > 0")
	require.Greater(t, run.BuildWallNS, int64(0), "build wall should be > 0")
	require.Greater(t, run.TotalLOC, 0, "generated LOC should be > 0")
	require.Greater(t, run.FileCount, 0, "generated file count should be > 0")
}

// TestRunFixture_ExternalSchemaPath verifies that an absolute SchemaDir works.
// We simulate "external" by passing an absolute path to one of the in-repo
// fixture schema dirs — the bench should treat the absolute path as-is rather
// than joining it onto repoRoot.
func TestRunFixture_ExternalSchemaPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bench requires POSIX rusage")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	repoRoot := repoRootForTest(t)
	abs := filepath.Join(repoRoot, "entc/integration/privacy/ent/schema")
	require.True(t, filepath.IsAbs(abs))
	run, err := RunFixture(repoRoot, Fixture{
		Name:      "privacy-external",
		SchemaDir: abs,
	})
	require.NoError(t, err)
	require.Equal(t, "privacy-external", run.Fixture)
	require.Greater(t, run.TotalLOC, 0)
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err, "git rev-parse must work in the test environment")
	return filepath.Clean(string(wd[:len(wd)-1])) // strip trailing newline
}
