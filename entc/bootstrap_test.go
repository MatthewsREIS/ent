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

// TestBootstrap_SkipHooksThatReferenceGeneratedMutation is the canonical
// regression test for codegen-epic PR 1: a schema declares a Hooks() method
// that references a not-yet-generated mutation method. Without bootstrap
// mode, the loader fails to type-check the schema package and codegen
// aborts. With SkipHookCompilation, codegen succeeds.
//
// The test runs codegen as a subprocess (go run ./gen.go) from inside a
// temp module that replaces entgo.io/ent with the local repo. This matches
// the pattern used by internal/bench/bench.go and avoids the "directory
// outside main module" error that occurs when entc.Generate is called
// directly from the test binary (which runs inside entgo.io/ent's module).
func TestBootstrap_SkipHooksThatReferenceGeneratedMutation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	mod := writeBootstrapModule(t, "bootstraphooks")

	schemaDir := filepath.Join(mod, "ent", "schema")
	require.NoError(t, os.MkdirAll(schemaDir, 0o755))

	// Write a schema with a Hooks() body that calls SetMagicValue, a method
	// that does not exist in the generated code. Without bootstrap mode the
	// loader fails to type-check the schema package (because the schema
	// imports bootstraphooks/ent which doesn't exist yet, and even after
	// generation SetMagicValue is not real). With SkipHookCompilation the
	// Hooks() body is AST-stripped before the loader runs, so the
	// bootstraphooks/ent import becomes unused and is removed by the
	// stripper, and codegen succeeds.
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user.go"), []byte(`package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	bootstrapent "bootstraphooks/ent"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if um, ok := m.(*bootstrapent.UserMutation); ok {
					// SetMagicValue is not a real generated method; this
					// reference will fail to type-check unless bootstrap
					// strips the Hooks body before the loader runs.
					um.SetMagicValue("x")
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}
`), 0o644))

	// gen.go drives entc.Generate with SkipHookCompilation from inside the
	// temp module so the go tool resolves the module graph correctly.
	entDir := filepath.Join(mod, "ent")
	require.NoError(t, os.WriteFile(filepath.Join(entDir, "gen.go"), []byte(`//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{}, entc.SkipHookCompilation()); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
`), 0o644))

	cmd := exec.Command("go", "run", "-mod=mod", "./gen.go")
	cmd.Dir = entDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"codegen with SkipHookCompilation should succeed even though the schema hook references a not-yet-generated method;\noutput:\n%s", out)

	// Verify the generated User entity exists (proves codegen actually ran,
	// not just that the loader silently noop'd).
	userGo := filepath.Join(entDir, "user.go")
	_, statErr := os.Stat(userGo)
	require.NoError(t, statErr, "generated user.go should exist after bootstrap codegen")
}

// TestBootstrap_SkipPolicyThatReferenceGeneratedMutation is the regression
// test for bootstrap mode handling Policy() methods. A schema declares a
// Policy() method whose MutationRule body references a not-yet-generated
// mutation method. Without bootstrap mode the loader fails to type-check the
// schema package and codegen aborts. With SkipHookCompilation, codegen
// succeeds because the AST stripper removes the Policy() body before the
// loader runs.
//
// The test follows the same subprocess + aliased-import pattern as
// TestBootstrap_SkipHooksThatReferenceGeneratedMutation.
func TestBootstrap_SkipPolicyThatReferenceGeneratedMutation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	// Use a distinct module name to avoid any test-cache aliasing with the
	// hooks test above.
	mod := writeBootstrapModule(t, "bootstrappolicy")

	schemaDir := filepath.Join(mod, "ent", "schema")
	require.NoError(t, os.MkdirAll(schemaDir, 0o755))

	// Write a schema with a Policy() body that directly references
	// (*bootstrapent.UserMutation).NotARealMethod via an unreachable if-false
	// block. The go/types type-checker still validates the reference even in
	// unreachable code, so without bootstrap mode the schema package fails to
	// compile. With SkipHookCompilation the entire Policy() body is
	// AST-stripped before the loader runs, so bootstrappolicy/ent becomes
	// unused, is deleted from the import block by the stripper, and codegen
	// succeeds.
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	bootstrapent "bootstrappolicy/ent"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (User) Policy() ent.Policy {
	// NotARealMethod does not exist on *UserMutation; the type checker
	// validates this even in an unreachable branch, so the schema fails
	// to compile without bootstrap stripping.
	if false {
		(*bootstrapent.UserMutation)(nil).NotARealMethod()
	}
	return nil
}
`), 0o644))

	entDir := filepath.Join(mod, "ent")
	require.NoError(t, os.WriteFile(filepath.Join(entDir, "gen.go"), []byte(`//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{}, entc.SkipHookCompilation()); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
`), 0o644))

	cmd := exec.Command("go", "run", "-mod=mod", "./gen.go")
	cmd.Dir = entDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"codegen with SkipHookCompilation should succeed even though the schema Policy references a not-yet-generated method;\noutput:\n%s", out)

	// Verify the generated User entity exists (proves codegen actually ran,
	// not just that the loader silently noop'd).
	userGo := filepath.Join(entDir, "user.go")
	_, statErr := os.Stat(userGo)
	require.NoError(t, statErr, "generated user.go should exist after bootstrap codegen")
}

// writeBootstrapModule creates a temp Go module with go.mod pointing back at
// the local ent repo via a replace directive, then runs go mod tidy to
// populate go.sum. Returns the module root.
func writeBootstrapModule(t *testing.T, module string) string {
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

	// go mod tidy populates go.sum so that subsequent go run commands
	// succeed without network access.
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, tidyErr := cmd.CombinedOutput()
	require.NoError(t, tidyErr, "go mod tidy failed: %s", out)

	return mod
}
