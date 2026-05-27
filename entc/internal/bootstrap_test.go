// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (User) Hooks() []ent.Hook {")
	require.Contains(t, s, "return make([]ent.Hook, 1)", "1-element Hooks body must become make([]ent.Hook, 1)")
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
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (Tenant) Policy() ent.Policy {")
	require.Contains(t, s, "func (Tenant) Interceptors() []ent.Interceptor {")
	// Policy() returns a composite struct literal (not []ent.Hook) so falls
	// back to return nil. Interceptors() has 1 element so becomes make([]ent.Interceptor, 1).
	require.Contains(t, s, "return nil", "Policy body must be replaced")
	require.Contains(t, s, "return make([]ent.Interceptor, 1)", "Interceptors body must be replaced with correct count")
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
	out, err := StripHookBodies([]byte(src), nil)
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
	out, err := StripHookBodies([]byte(src), nil)
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
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "[]ent.Hook{nil}", "top-level Hooks function must be preserved")
}

func TestStripHookBodies_SyntaxErrorReturnsError(t *testing.T) {
	src := `package schema

func ((((`
	_, err := StripHookBodies([]byte(src), nil)
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
	require.Contains(t, string(userOut), "make([]ent.Hook, 1)", "1-element hook preserved as make")

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

// --- count-aware stripping tests ---

func TestStripHookBodies_CompositeLiteralCountPreserved(t *testing.T) {
	// Hooks() returning a composite literal with 2 elements should become
	// `return make([]ent.Hook, 2)`, not `return nil`, so ent allocates
	// the right number of slots in the generated runtime.go init().
	src := `package schema

import (
	"entgo.io/ent"
	"example.com/x/ent/gen/hook"
)

type User struct{ ent.Schema }

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		hook.UserFunc(nil),
		hook.UserFunc(nil),
	}
}
`
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "return make([]ent.Hook, 2)", "2-element literal must become make([]ent.Hook, 2)")
	require.NotContains(t, s, "hook.UserFunc", "original body must be gone")
}

func TestStripHookBodies_EmptyLiteralBecomesNil(t *testing.T) {
	// An empty literal `return []ent.Hook{}` has 0 elements and should
	// become `return nil` (same as the existing nil-body behaviour).
	src := `package schema

import "entgo.io/ent"

type Group struct{ ent.Schema }

func (Group) Hooks() []ent.Hook {
	return []ent.Hook{}
}
`
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	require.Contains(t, string(out), "return nil")
}

func TestStripHookBodies_InterceptorCountPreserved(t *testing.T) {
	// Same count-preservation applies to Interceptors().
	src := `package schema

import (
	"entgo.io/ent"
	"example.com/x/ent/gen/intercept"
)

type Order struct{ ent.Schema }

func (Order) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		intercept.OrderFunc(nil),
		intercept.OrderFunc(nil),
		intercept.OrderFunc(nil),
	}
}
`
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	require.Contains(t, string(out), "return make([]ent.Interceptor, 3)")
}

func TestStripHookBodies_NamedFuncCallUsesCountMap(t *testing.T) {
	// When Hooks() returns a named function call, the count map supplies
	// the element count so ent allocates the right number of slots.
	src := `package schema

import "entgo.io/ent"

type BoxFolder struct{ ent.Schema }

func (BoxFolder) Hooks() []ent.Hook {
	return boxFolderHooks()
}
`
	counts := map[string]int{"boxFolderHooks": 2}
	out, err := StripHookBodies([]byte(src), counts)
	require.NoError(t, err)
	require.Contains(t, string(out), "return make([]ent.Hook, 2)",
		"named-func call with count 2 must become make([]ent.Hook, 2)")
	require.NotContains(t, string(out), "boxFolderHooks")
}

func TestStripHookBodies_NamedFuncNotInCountMapErrors(t *testing.T) {
	// When the helper is not in the count map (e.g. its *_runtime.go body is not
	// a single composite-literal return, so CountHookFunctions could not count
	// it), the slot count is undeterminable. Stubbing it to nil would silently
	// wire zero hooks and drop the entity's hooks at runtime, so StripHookBodies
	// fails closed instead of guessing.
	src := `package schema

import "entgo.io/ent"

type Widget struct{ ent.Schema }

func (Widget) Hooks() []ent.Hook {
	return unknownHooks()
}
`
	_, err := StripHookBodies([]byte(src), nil)
	require.Error(t, err)
	var ue *UncountableHookReturnError
	require.ErrorAs(t, err, &ue)
	require.Equal(t, "Hooks", ue.Method)
	require.Contains(t, ue.Expr, "unknownHooks")
}

func TestStripHookBodies_UnrecognizedReturnErrors(t *testing.T) {
	// A return that is neither a composite literal, a bare nil, nor a known
	// helper call (here a bare identifier built from a local) cannot be counted.
	src := `package schema

import "entgo.io/ent"

type Widget struct{ ent.Schema }

func (Widget) Hooks() []ent.Hook {
	hooks := buildWidgetHooks()
	return hooks
}
`
	_, err := StripHookBodies([]byte(src), nil)
	require.Error(t, err, "a bare-identifier return is not a recognized countable form")
	var ue *UncountableHookReturnError
	require.ErrorAs(t, err, &ue)
}

func TestStripHookBodies_NilReturnStillValid(t *testing.T) {
	// A bare nil return is a deliberate zero (the guarded-mixin pattern) and must
	// remain valid — it is not an uncountable error.
	src := `package schema

import "entgo.io/ent"

type Widget struct{ ent.Schema }

func (Widget) Hooks() []ent.Hook {
	return nil
}
`
	out, err := StripHookBodies([]byte(src), nil)
	require.NoError(t, err)
	require.Contains(t, string(out), "return nil")
}

func TestStripHookBodies_GuardedHooksPreservesGuardAndCount(t *testing.T) {
	// A Hooks() method with a DisableSoftDelete guard before the real return
	// (the BaseMixin pattern). The guard must be PRESERVED so codegen evaluates
	// it per-instance: a mixin with DisableSoftDelete=true returns nil (count 0,
	// so no runtime wiring), while DisableSoftDelete=false returns the real
	// count. Only the undefined leaf-func-call return is rewritten to make().
	src := `package schema

import "entgo.io/ent"

type BaseMixin struct{ DisableSoftDelete bool }

func (b BaseMixin) Hooks() []ent.Hook {
	if b.DisableSoftDelete {
		return nil
	}
	return baseMixinHooks(b)
}
`
	counts := map[string]int{"baseMixinHooks": 1}
	out, err := StripHookBodies([]byte(src), counts)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "if b.DisableSoftDelete", "the guard clause must be preserved so the count is per-instance")
	require.Contains(t, s, "return make([]ent.Hook, 1)", "the real (leaf-func) return must become make([]ent.Hook, 1)")
	require.NotContains(t, s, "baseMixinHooks", "the undefined leaf-func call must be gone")
	_, perr := parser.ParseFile(token.NewFileSet(), "", out, 0)
	require.NoError(t, perr, "stripped output must be valid Go")
}

func TestStripHookBodies_GuardedInterceptorsPreservesGuardAndCount(t *testing.T) {
	src := `package schema

import "entgo.io/ent"

type BaseMixin struct{ DisableSoftDelete bool }

func (b BaseMixin) Interceptors() []ent.Interceptor {
	if b.DisableSoftDelete {
		return nil
	}
	return baseMixinInterceptors(b)
}
`
	counts := map[string]int{"baseMixinInterceptors": 1}
	out, err := StripHookBodies([]byte(src), counts)
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "if b.DisableSoftDelete", "the guard clause must be preserved")
	require.Contains(t, s, "return make([]ent.Interceptor, 1)", "the real return must become make([]ent.Interceptor, 1)")
	require.NotContains(t, s, "baseMixinInterceptors", "the undefined leaf-func call must be gone")
	_, perr := parser.ParseFile(token.NewFileSet(), "", out, 0)
	require.NoError(t, perr, "stripped output must be valid Go")
}

func TestCountHookFunctions_BasicCases(t *testing.T) {
	dir := t.TempDir()

	// A *_runtime.go with two hook functions of different counts.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "owner_runtime.go"), []byte(`package schema

import "entgo.io/ent"

func ownerMixinHooks() []ent.Hook {
	return []ent.Hook{nil, nil}
}

func ownerCMixinHooks() []ent.Hook {
	return []ent.Hook{nil, nil, nil}
}
`), 0o644))

	// A regular schema file — its methods must NOT be counted.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.go"), []byte(`package schema

import "entgo.io/ent"

type User struct{ ent.Schema }

func (User) Hooks() []ent.Hook {
	return ownerMixinHooks()
}
`), 0o644))

	counts, err := CountHookFunctions(dir)
	require.NoError(t, err)
	require.Equal(t, 2, counts["ownerMixinHooks"])
	require.Equal(t, 3, counts["ownerCMixinHooks"])
	require.NotContains(t, counts, "Hooks", "schema methods must not be in the count map")
}

func TestCountHookFunctions_InterceptorFunctions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base_runtime.go"), []byte(`package schema

import "entgo.io/ent"

func baseMixinInterceptors() []ent.Interceptor {
	return []ent.Interceptor{nil}
}
`), 0o644))

	counts, err := CountHookFunctions(dir)
	require.NoError(t, err)
	require.Equal(t, 1, counts["baseMixinInterceptors"])
}

func TestCountHookFunctions_EmptyAndNilReturn(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noop_runtime.go"), []byte(`package schema

import "entgo.io/ent"

func noopHooks() []ent.Hook {
	return nil
}

func emptyHooks() []ent.Hook {
	return []ent.Hook{}
}
`), 0o644))

	counts, err := CountHookFunctions(dir)
	require.NoError(t, err)
	require.Equal(t, 0, counts["noopHooks"])
	require.Equal(t, 0, counts["emptyHooks"])
}

func TestStageStrippedSchema_HookCountsPreservedAcrossFiles(t *testing.T) {
	// End-to-end: a schema dir where the schema file delegates to a runtime
	// file. After staging, the stripped schema method should contain the
	// right make() count so the generated runtime.go allocates the right
	// number of slots.
	src := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(src, "owner.go"), []byte(`package schema

import "entgo.io/ent"

type Owner struct{ ent.Schema }

func (Owner) Hooks() []ent.Hook {
	return ownerHooks()
}
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(src, "owner_runtime.go"), []byte(`//go:build !entcodegen

package schema

import "entgo.io/ent"

func ownerHooks() []ent.Hook {
	return []ent.Hook{nil, nil}
}
`), 0o644))

	dst, err := StageStrippedSchema(src)
	require.NoError(t, err)
	defer os.RemoveAll(dst)

	ownerOut, err := os.ReadFile(filepath.Join(dst, "owner.go"))
	require.NoError(t, err)
	require.Contains(t, string(ownerOut), "return make([]ent.Hook, 2)",
		"staged schema must reflect the hook count from the runtime file")
}

func TestStageStrippedSchema_HookMethodOnlyInExcludedFileErrors(t *testing.T) {
	// The Hooks() METHOD itself lives in a //go:build !entcodegen file, so the
	// loader (which builds with -tags entcodegen) never sees it and would wire
	// zero hooks for Escrow — silently dropping them at runtime. Fail closed.
	src := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(src, "escrow.go"), []byte(`package schema

import "entgo.io/ent"

type Escrow struct{ ent.Schema }
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(src, "escrow_hooks.go"), []byte(`//go:build !entcodegen

package schema

import "entgo.io/ent"

func (Escrow) Hooks() []ent.Hook {
	return []ent.Hook{escrowTrsHook}
}
`), 0o644))

	_, err := StageStrippedSchema(src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Escrow.Hooks")
	require.Contains(t, err.Error(), "!entcodegen")
}

func TestStageStrippedSchema_HookMethodInUntaggedFileOK(t *testing.T) {
	// The correct pattern: the Hooks() METHOD is in the untagged entity file
	// (visible to codegen) and only the implementation lives in the !entcodegen
	// file. This must NOT error.
	src := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(src, "escrow.go"), []byte(`package schema

import "entgo.io/ent"

type Escrow struct{ ent.Schema }

func (Escrow) Hooks() []ent.Hook {
	return escrowHooks()
}
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(src, "escrow_hooks.go"), []byte(`//go:build !entcodegen

package schema

import "entgo.io/ent"

func escrowHooks() []ent.Hook {
	return []ent.Hook{nil}
}
`), 0o644))

	dst, err := StageStrippedSchema(src)
	require.NoError(t, err)
	defer os.RemoveAll(dst)
}
