// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteTypedSetter_SetDeletedAtCall(t *testing.T) {
	// Mirrors the consumer's mixin_base.go soft-delete hook: assertion on
	// ent.Mutation expecting SetOp/SetDeletedAt/WhereP, then a call to
	// mx.SetDeletedAt(now) inside a closure that returns (ent.Value, error).
	src := `package x

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
)

func hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		mx, ok := m.(interface {
			SetOp(ent.Op)
			SetDeletedAt(time.Time)
			WhereP(...func(*sql.Selector))
		})
		if !ok {
			return nil, fmt.Errorf("unexpected mutation type %T", m)
		}
		mx.SetOp(ent.OpUpdate)
		now := time.Now()
		mx.SetDeletedAt(now)
		return next.Mutate(ctx, m)
	})
}
`
	out, err := RewriteTypedSetterSource("hook.go", src, nil, "")
	require.NoError(t, err)
	require.NotContains(t, out, "SetDeletedAt(time.Time)", "interface method must be dropped")
	require.NotContains(t, out, "mx.SetDeletedAt(now)", "call site must be rewritten")
	require.Contains(t, out, `SetField(string, ent.Value) error`, "SetField must be added to the asserted interface")
	require.Contains(t, out, `mx.SetField("deleted_at", now)`)
	require.Contains(t, out, `fmt.Errorf("soft-delete: set deleted_at: %w", err)`)
}

func TestRewriteTypedSetter_NoMatchLeavesSourceUnchanged(t *testing.T) {
	// No SetDeletedAt in the asserted interface — nothing to do.
	src := `package x

import "entgo.io/ent"

func hook(m ent.Mutation) {
	if x, ok := m.(interface{ Foo() }); ok {
		_ = x
	}
}
`
	out, err := RewriteTypedSetterSource("hook.go", src, nil, "")
	require.NoError(t, err)
	require.Equal(t, src, out)
}

func TestRewriteTypedSetter_Idempotent(t *testing.T) {
	src := `package x

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
)

func hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		mx, ok := m.(interface {
			SetOp(ent.Op)
			SetDeletedAt(time.Time)
			WhereP(...func(*sql.Selector))
		})
		if !ok {
			return nil, fmt.Errorf("unexpected mutation type %T", m)
		}
		mx.SetDeletedAt(time.Now())
		return next.Mutate(ctx, m)
	})
}
`
	pass1, err := RewriteTypedSetterSource("hook.go", src, nil, "")
	require.NoError(t, err)
	pass2, err := RewriteTypedSetterSource("hook.go", pass1, nil, "")
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
	require.True(t, strings.Contains(pass2, `mx.SetField("deleted_at"`))
}

func TestRewriteTypedSetter_NestedInIfBlock(t *testing.T) {
	// The call to SetDeletedAt sits inside an if-statement nested under the
	// MutateFunc closure. The walker has to recurse into the if-body.
	src := `package x

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
)

func hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		mx, ok := m.(interface {
			SetOp(ent.Op)
			SetDeletedAt(time.Time)
			WhereP(...func(*sql.Selector))
		})
		if !ok {
			return nil, fmt.Errorf("unexpected mutation type %T", m)
		}
		if true {
			mx.SetDeletedAt(time.Now())
		}
		return next.Mutate(ctx, m)
	})
}
`
	out, err := RewriteTypedSetterSource("hook.go", src, nil, "")
	require.NoError(t, err)
	require.Contains(t, out, `mx.SetField("deleted_at"`, "rewrite must reach nested if-body")
	require.NotContains(t, out, "mx.SetDeletedAt(")
}
