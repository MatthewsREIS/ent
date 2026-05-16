// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

type testPred = func(*sql.Selector)

func TestQueryState_AddPredicates(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	p1 := testPred(func(*sql.Selector) {})
	p2 := testPred(func(*sql.Selector) {})
	s.AddPredicates(p1, p2)
	require.Len(t, s.Predicates, 2)
}

func TestQueryState_SetLimit(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	s.SetLimit(42)
	require.NotNil(t, s.Ctx.Limit)
	require.Equal(t, 42, *s.Ctx.Limit)
}

func TestQueryState_SetOffset(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	s.SetOffset(7)
	require.NotNil(t, s.Ctx.Offset)
	require.Equal(t, 7, *s.Ctx.Offset)
}

func TestQueryState_SetUnique(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	s.SetUnique(true)
	require.NotNil(t, s.Ctx.Unique)
	require.True(t, *s.Ctx.Unique)
}

func TestQueryState_Clone_DeepCopiesPredicates(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{
		Ctx:        &ent.QueryContext{Type: "User"},
		Predicates: []testPred{func(*sql.Selector) {}, func(*sql.Selector) {}},
	}
	c := s.Clone()
	require.NotSame(t, s, c, "Clone must return a distinct pointer")
	require.NotSame(t, s.Ctx, c.Ctx, "Clone must deep-copy Ctx")
	require.Equal(t, s.Ctx.Type, c.Ctx.Type)
	require.Len(t, c.Predicates, 2)
	// Mutating clone must not affect original.
	c.AddPredicates(func(*sql.Selector) {})
	require.Len(t, s.Predicates, 2)
	require.Len(t, c.Predicates, 3)
}

func TestQueryState_Clone_PreservesInterceptors(t *testing.T) {
	inter := ent.InterceptFunc(func(next ent.Querier) ent.Querier { return next })
	s := &entbuilder.QueryState[testPred]{
		Ctx:    &ent.QueryContext{},
		Inters: []ent.Interceptor{inter},
	}
	c := s.Clone()
	require.Len(t, c.Inters, 1)
}

// Compile-time sanity check that QueryState[P] satisfies the embedded-into-<Entity>Query pattern.
func TestQueryState_EmbedsCleanly(t *testing.T) {
	type UserQuery struct {
		entbuilder.QueryState[testPred]
		extraField int
	}
	uq := &UserQuery{}
	uq.Ctx = &ent.QueryContext{}
	uq.SetLimit(5)
	uq.extraField = 10
	require.Equal(t, 5, *uq.Ctx.Limit)
	require.Equal(t, 10, uq.extraField)
	_ = context.Background() // unused-import suppressor; context is imported above by other tests
}
