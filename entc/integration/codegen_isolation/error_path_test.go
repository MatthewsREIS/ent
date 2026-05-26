// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package codegen_isolation_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/stretchr/testify/require"
)

// setupTempModule creates an isolated module directory outside the ent
// source tree (in os.TempDir()) so that packages.Load resolves it as an
// independent module rather than a subdirectory of entgo.io/ent.
//
// It writes a go.mod with a replace directive pointing to the local ent
// checkout, changes the working directory to the module root for the
// duration of the test (restored on cleanup), and returns the module root.
//
// Callers must write all Go source files into the module directory BEFORE
// calling goModTidy so that go mod tidy can resolve transitive deps.
//
// This mirrors the pattern used by gen/regression_test.go:writeTempModule.
func setupTempModule(t *testing.T, module string) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)
	// entc/integration/codegen_isolation → three levels up to repo root.
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))

	modDir := t.TempDir()

	goMod := fmt.Sprintf(`module %s

go 1.24

require entgo.io/ent v0.0.0

replace entgo.io/ent => %s
`, module, repoRoot)
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "go.mod"), []byte(goMod), 0o644))

	// Change cwd to the temp module so that packages.Load and `go` invocations
	// inside load.Config.load() / gocmd() operate within the right module root.
	require.NoError(t, os.Chdir(modDir))
	t.Cleanup(func() {
		os.Chdir(wd) //nolint:errcheck // best-effort restore
	})

	return modDir
}

// goModTidy runs `go mod tidy -e` in dir and fails the test if it errors.
// The -e flag tells tidy to continue despite unresolvable imports (e.g. not-
// yet-generated packages); those will surface as typecheck errors later, which
// is exactly what the error-path tests are verifying.
//
// Call this after writing all schema source files so tidy can add
// transitively-required real deps (like entgo.io/ent) to go.sum.
func goModTidy(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "mod", "tidy", "-e")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go mod tidy -e failed: %s", out)
}

// TestErrorPath_BrokenHookWithoutTag confirms that when a schema-package
// file references generated symbols WITHOUT the //go:build !entcodegen
// tag, codegen fails with an actionable error message — not a silent
// snapshot restore.
func TestErrorPath_BrokenHookWithoutTag(t *testing.T) {
	tmpDir := setupTempModule(t, "example.com/test")
	schemaDir := filepath.Join(tmpDir, "schema")
	require.NoError(t, os.MkdirAll(schemaDir, 0o755))

	// Class A — pure schema.
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}
`), 0o644))

	// Deliberately broken file WITHOUT the build tag — references
	// a non-existent import that the typechecker must reject.
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user_broken.go"), []byte(`package schema

import "this/package/does/not/exist"

var _ = nonexistent.Symbol
`), 0o644))

	goModTidy(t, tmpDir)

	err := entc.Generate(schemaDir, &gen.Config{
		Target:  filepath.Join(tmpDir, "ent"),
		Package: "example.com/test/ent",
	})

	require.Error(t, err, "codegen must not silently succeed when schema package fails to typecheck")
	require.Contains(t, err.Error(), "this/package/does/not/exist", "error must name the failing import")
	// No build-tag hint expected: the broken symbol does not match the
	// generated-code heuristic, so the hint must be absent.
	require.NotContains(t, err.Error(), "//go:build !entcodegen", "must not suggest the build-tag for non-generated-looking symbols")
}

// TestErrorPath_HookReferencesGeneratedCode confirms that the build-tag
// hint IS emitted when the failing symbol looks like generated code.
func TestErrorPath_HookReferencesGeneratedCode(t *testing.T) {
	tmpDir := setupTempModule(t, "example.com/test")
	schemaDir := filepath.Join(tmpDir, "schema")
	require.NoError(t, os.MkdirAll(schemaDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}
`), 0o644))

	// This file imports entgo.io/ent (which exists) but references a symbol
	// that does not exist: ent.UserMutationFunc. The error message will be
	// "undefined: ent.UserMutationFunc", which matches generatedSymbolRE
	// (\bent\.[A-Z]) and therefore should produce the build-tag hint.
	//
	// This simulates the real scenario: a developer writes a hook helper
	// that references generated types without the //go:build !entcodegen guard.
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user_hooks.go"), []byte(`package schema

import "entgo.io/ent"

// pretend this references a generated mutation type
var _ = ent.UserMutationFunc
`), 0o644))

	goModTidy(t, tmpDir)

	err := entc.Generate(schemaDir, &gen.Config{
		Target:  filepath.Join(tmpDir, "ent"),
		Package: "example.com/test/ent",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "//go:build !entcodegen", "must suggest the build-tag escape hatch when the symbol looks generated")
}
