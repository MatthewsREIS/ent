// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"reflect"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

func TestGetField_Set(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, testDescriptor())
	require.NoError(t, m.SetField("title", "hello"))
	v, ok := entbuilder.GetField[string](m, "title")
	require.True(t, ok)
	require.Equal(t, "hello", v)
}

func TestGetField_Unset(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, testDescriptor())
	v, ok := entbuilder.GetField[string](m, "title")
	require.False(t, ok)
	require.Equal(t, "", v)
}

func TestOldFieldAs(t *testing.T) {
	desc := testDescriptor()
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdateOne, desc)
	m.SetID(7)
	m.SetOldValueLoader(func(ctx context.Context) (any, error) {
		return &testEntity{ID: 7, Title: "old"}, nil
	})
	got, err := entbuilder.OldFieldAs[string](context.Background(), m, "title")
	require.NoError(t, err)
	require.Equal(t, "old", got)
}

func TestEdgeIDsAs(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Edges: map[string]entbuilder.EdgeSpec{
			"teams": {Cardinality: entbuilder.M2M, TargetIDType: reflect.TypeFor[int]()},
		},
	}
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, desc)
	require.NoError(t, m.AddEdgeIDs("teams", 1, 2, 3))
	ids := entbuilder.EdgeIDsAs[int](m, "teams")
	require.ElementsMatch(t, []int{1, 2, 3}, ids)
}

func TestEdgeIDAs(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Edges: map[string]entbuilder.EdgeSpec{
			"owner": {Cardinality: entbuilder.O2OUnique, TargetIDType: reflect.TypeFor[int]()},
		},
	}
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, desc)
	require.NoError(t, m.SetEdgeID("owner", 99))
	id, ok := entbuilder.EdgeIDAs[int](m, "owner")
	require.True(t, ok)
	require.Equal(t, 99, id)
}
