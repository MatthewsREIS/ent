// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package codegen_isolation_test

import (
	"context"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/codegen_isolation/ent/enttest"
	"entgo.io/ent/entc/integration/codegen_isolation/ent/schema"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestCodegenIsolation_UserCreate confirms the generated ent package
// produced by the codegen-isolation scaffold works end-to-end.
// If this test compiles and passes, codegen succeeded against a schema
// package containing a `//go:build !entcodegen` file that references
// not-yet-generated symbols — proving the tag injection unblocks codegen.
func TestCodegenIsolation_UserCreate(t *testing.T) {
	// Note: this test exercises the generated package's CRUD path; it does NOT
	// register UserCreateHook via client.User.Use(...). End-to-end hook behavior
	// is intentionally out of scope here — see TestCodegenIsolation_ClassBSymbolReachable
	// for the build-time reachability assertion.
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	u, err := client.User.Create().SetName("alice").Save(ctx)
	require.NoError(t, err)
	require.Equal(t, "alice", u.Name)
}

// TestCodegenIsolation_ClassBSymbolReachable confirms that the Class B
// file (user_hooks.go, gated with //go:build !entcodegen) IS compiled
// into the test binary in normal builds and its exported symbol is
// callable. Together with TestCodegenIsolation_UserCreate this proves
// the tag's exclusion is codegen-only, not build-wide.
func TestCodegenIsolation_ClassBSymbolReachable(t *testing.T) {
	hook := schema.UserCreateHook()
	require.NotNil(t, hook, "Class B file must contribute UserCreateHook to normal builds")

	// Invoke the outer closure with a no-op mutator to confirm the closure
	// body executes — proving the inner references to generated hook.UserFunc
	// and *genent.UserMutation resolved correctly at link time.
	noop := ent.MutateFunc(func(_ context.Context, _ ent.Mutation) (ent.Value, error) {
		return nil, nil
	})
	mutator := hook(noop)
	require.NotNil(t, mutator, "hook must produce a wrapping mutator")
}
