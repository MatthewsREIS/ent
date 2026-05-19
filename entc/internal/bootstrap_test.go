// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripHookBodies_BasicHooks(t *testing.T) {
	src := `package schema

import (
	"context"

	"entgo.io/ent"
	"example.com/x/ent/gen"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return nil
}

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if um, ok := m.(*gen.UserMutation); ok {
					um.SetName("processed")
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (User) Hooks() []ent.Hook {")
	require.Contains(t, s, "return nil\n}", "Hooks body must be replaced with `return nil`")
	require.NotContains(t, s, "SetName", "the original hook body must be gone")
	require.NotContains(t, s, "MutateFunc", "the original hook body must be gone")
	require.NotContains(t, s, `"context"`, "context import must be dropped (now unused)")
	require.NotContains(t, s, `"example.com/x/ent/gen"`, "generated-pkg import must be dropped (now unused)")
	require.Contains(t, s, `"entgo.io/ent"`, "ent import must stay (still used in method signature)")
	require.Contains(t, s, "func (User) Fields() []ent.Field {", "Fields method must be preserved")
}

func TestStripHookBodies_PolicyAndInterceptors(t *testing.T) {
	src := `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/privacy"
	"example.com/x/ent/gen/rule"
)

type Tenant struct {
	ent.Schema
}

func (Tenant) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			rule.DenyIfNoTenant(),
		},
	}
}

func (Tenant) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		rule.FilterTenant(),
	}
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (Tenant) Policy() ent.Policy {")
	require.Contains(t, s, "func (Tenant) Interceptors() []ent.Interceptor {")
	occurrences := strings.Count(s, "return nil\n}")
	require.GreaterOrEqual(t, occurrences, 2, "both Policy and Interceptors bodies must be replaced")
	require.NotContains(t, s, "DenyIfNoTenant", "Policy body must be gone")
	require.NotContains(t, s, "FilterTenant", "Interceptors body must be gone")
	require.NotContains(t, s, `"entgo.io/ent/privacy"`, "privacy import must be dropped (now unused)")
	require.NotContains(t, s, `"example.com/x/ent/gen/rule"`, "rule import must be dropped (now unused)")
	require.Contains(t, s, `"entgo.io/ent"`, "ent import must stay")
}

func TestStripHookBodies_PreservesUnrelatedFunctions(t *testing.T) {
	src := `package schema

import "entgo.io/ent"

type Comment struct {
	ent.Schema
}

func (Comment) Fields() []ent.Field {
	return nil
}

func (Comment) Edges() []ent.Edge {
	return nil
}

func someHelper() int {
	return 42
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func someHelper() int {", "top-level helpers must be preserved (only method receivers are stripped)")
	require.Contains(t, s, "return 42", "helper body must be preserved")
	require.Contains(t, s, "func (Comment) Fields() []ent.Field {", "Fields preserved")
	require.Contains(t, s, "func (Comment) Edges() []ent.Edge {", "Edges preserved")
}

func TestStripHookBodies_NoHooksMethod(t *testing.T) {
	src := `package schema

import "entgo.io/ent"

type Group struct {
	ent.Schema
}

func (Group) Fields() []ent.Field {
	return nil
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (Group) Fields() []ent.Field {")
	require.NotContains(t, s, "Hooks", "no Hooks method in source -> no Hooks method in output")
}

func TestStripHookBodies_TopLevelFuncCalledHooksNotStripped(t *testing.T) {
	// A top-level (non-method) function named Hooks must NOT be stripped --
	// the stripper only touches methods (FuncDecl.Recv != nil).
	src := `package schema

import "entgo.io/ent"

func Hooks() []ent.Hook {
	return []ent.Hook{nil}
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "[]ent.Hook{nil}", "top-level Hooks function must be preserved")
}

func TestStripHookBodies_SyntaxErrorReturnsError(t *testing.T) {
	src := `package schema

func ((((`
	_, err := StripHookBodies([]byte(src))
	require.Error(t, err)
}

func TestStageStrippedSchema_CopiesAndStrips(t *testing.T) {
	src := t.TempDir()

	// Write a schema file with a Hooks method that references a "generated" symbol.
	userSrc := `package schema

import (
	"context"

	"entgo.io/ent"
	"badpackage" // intentionally bad path -- proves the stripped output doesn't need it
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field { return nil }

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				_ = badpackage.GoSomething(m)
				return next.Mutate(ctx, m)
			})
		},
	}
}
`
	require.NoError(t, os.WriteFile(filepath.Join(src, "user.go"), []byte(userSrc), 0o644))

	// Also write a non-.go file that should be copied verbatim.
	require.NoError(t, os.WriteFile(filepath.Join(src, "README"), []byte("hello"), 0o644))

	// And a subdirectory with another .go file.
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	subSrc := `package schema

import "entgo.io/ent"

type Comment struct{ ent.Schema }

func (Comment) Fields() []ent.Field { return nil }
`
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "comment.go"), []byte(subSrc), 0o644))

	dst, err := StageStrippedSchema(src)
	require.NoError(t, err)
	defer os.RemoveAll(dst)

	// user.go should exist with Hooks body stripped and bad imports removed.
	userOut, err := os.ReadFile(filepath.Join(dst, "user.go"))
	require.NoError(t, err)
	require.NotContains(t, string(userOut), "badpackage", "bad import must be gone from stripped file")
	require.NotContains(t, string(userOut), "GoSomething", "stripped body content must be gone")
	require.Contains(t, string(userOut), "func (User) Hooks() []ent.Hook {")

	// README should be copied verbatim.
	readme, err := os.ReadFile(filepath.Join(dst, "README"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(readme))

	// sub/comment.go should exist (no hooks to strip; should be unchanged in semantics).
	commentOut, err := os.ReadFile(filepath.Join(dst, "sub", "comment.go"))
	require.NoError(t, err)
	require.Contains(t, string(commentOut), "type Comment struct")

	// Source directory must be untouched.
	srcUser, err := os.ReadFile(filepath.Join(src, "user.go"))
	require.NoError(t, err)
	require.Contains(t, string(srcUser), "GoSomething", "original source must NOT be modified")
}

func TestStageStrippedSchema_NonExistentSrc(t *testing.T) {
	_, err := StageStrippedSchema("/this/path/does/not/exist")
	require.Error(t, err)
}
