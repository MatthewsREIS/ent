// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Default("unknown"),
	}
}

// Hooks returns a no-op schema-attached hook. Its only purpose is to
// ensure codegen sees NumHooks > 0, which triggers the split-runtime
// form (ent/runtime/runtime.go in package runtime) instead of the
// monolithic form (ent/runtime.go in package ent that imports schema).
// The split form avoids the structural cycle that would otherwise occur
// when user_hooks.go references the generated hook subpackage.
//
// Real per-mutation logic lives in user_hooks.go (Class B,
// //go:build !entcodegen), registered explicitly via client.Use(...).
func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		// identity hook — intentionally empty; presence alone triggers runtime split.
		func(next ent.Mutator) ent.Mutator { return next },
	}
}
