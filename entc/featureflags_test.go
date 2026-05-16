// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFeatureNoUpdate_SuppressesUpdateBuilders verifies that turning on
// FeatureNoUpdate produces no *_update.go files in the generated tree
// (and no Update/UpdateOne/UpdateOneID methods on the typed client).
func TestFeatureNoUpdate_SuppressesUpdateBuilders(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	mod := writeFeatureFlagsModule(t, "ffnoupdate")
	schemaPath := filepath.Join(mod, "schema")

	require.NoError(t, os.MkdirAll(schemaPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaPath, "user.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}
`), 0o644))

	target := filepath.Join(mod, "ent")
	runCodegenWithFeatures(t, mod, target, schemaPath, []string{"no-update"})

	// Assert no *_update.go file emitted anywhere under target.
	matches, err := filepath.Glob(filepath.Join(target, "*_update.go"))
	require.NoError(t, err)
	require.Empty(t, matches, "FeatureNoUpdate must suppress *_update.go files; got: %v", matches)

	subMatches, _ := filepath.Glob(filepath.Join(target, "*", "update.go"))
	require.Empty(t, subMatches, "FeatureNoUpdate must suppress sub-package update.go files; got: %v", subMatches)
}

// TestFeatureNoDelete_SuppressesDeleteBuilders is the parallel test for delete.
func TestFeatureNoDelete_SuppressesDeleteBuilders(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	mod := writeFeatureFlagsModule(t, "ffnodelete")
	schemaPath := filepath.Join(mod, "schema")

	require.NoError(t, os.MkdirAll(schemaPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaPath, "user.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}
`), 0o644))

	target := filepath.Join(mod, "ent")
	runCodegenWithFeatures(t, mod, target, schemaPath, []string{"no-delete"})

	matches, err := filepath.Glob(filepath.Join(target, "*_delete.go"))
	require.NoError(t, err)
	require.Empty(t, matches, "FeatureNoDelete must suppress *_delete.go files; got: %v", matches)

	subMatches, _ := filepath.Glob(filepath.Join(target, "*", "delete.go"))
	require.Empty(t, subMatches, "FeatureNoDelete must suppress sub-package delete.go files; got: %v", subMatches)
}

// writeFeatureFlagsModule creates a temp Go module with go.mod pointing back at
// the local ent repo via a replace directive. Returns the module root.
func writeFeatureFlagsModule(t *testing.T, module string) string {
	t.Helper()
	mod := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Clean(filepath.Join(wd, ".."))
	goMod := fmt.Sprintf(`module %s

go 1.23

require entgo.io/ent v0.0.0

replace entgo.io/ent => %s
`, module, repoRoot)
	require.NoError(t, os.WriteFile(filepath.Join(mod, "go.mod"), []byte(goMod), 0o644))
	return mod
}

// runCodegenWithFeatures invokes ent codegen via go run in the temp module,
// passing the named features via the entc.FeatureNames option. The gen.go is
// generated inside <target>/ to match the standard ent layout (see PR 1's
// bootstrap_test.go for the pattern).
func runCodegenWithFeatures(t *testing.T, mod, target, schemaPath string, features []string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(target, 0o755))

	// Build the feature literal list.
	var featuresLit string
	for _, f := range features {
		featuresLit += fmt.Sprintf("%q, ", f)
	}

	genGo := fmt.Sprintf(`//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate(%q, &gen.Config{Target: %q}, entc.FeatureNames(%s)); err != nil {
		log.Fatalf("running ent codegen: %%v", err)
	}
}
`, schemaPath, target, featuresLit)
	require.NoError(t, os.WriteFile(filepath.Join(target, "gen.go"), []byte(genGo), 0o644))

	// go mod tidy so the replace directive's deps resolve.
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = mod
	tidyCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}

	// Run codegen.
	runCmd := exec.Command("go", "run", "-mod=mod", "./gen.go")
	runCmd.Dir = target
	runCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("go run gen.go: %v\n%s", err, out)
	}
}
