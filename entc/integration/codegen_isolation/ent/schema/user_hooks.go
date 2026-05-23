// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

//go:build !entcodegen

package schema

import (
	"context"
	"fmt"

	"entgo.io/ent"

	genent "entgo.io/ent/entc/integration/codegen_isolation/ent"
	"entgo.io/ent/entc/integration/codegen_isolation/ent/hook"
)

// UserCreateHook references generated hook.UserFunc and *genent.UserMutation,
// which only exist after codegen has run. This file MUST be excluded
// during codegen (via //go:build !entcodegen) — without the tag,
// codegen fails to typecheck the schema package because the generated
// symbols don't exist yet.
//
// The cycle that would otherwise form (schema → hook → ent → schema)
// is broken by the split-runtime form (see user.go for why).
//
// At runtime, register this hook explicitly via client.User.Use(...).
//
// Exported for use by out-of-package tests that verify the Class B file is
// reachable in normal builds.
func UserCreateHook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return hook.UserFunc(func(ctx context.Context, m *genent.UserMutation) (ent.Value, error) {
			if name, ok := m.Name(); ok && name == "" {
				return nil, fmt.Errorf("name is required")
			}
			return next.Mutate(ctx, m)
		})
	}
}
