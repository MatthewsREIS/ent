// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"reflect"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

func edgeTestDescriptor() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{},
		Edges: map[string]entbuilder.EdgeSpec{
			"teams": {Cardinality: entbuilder.M2M, Target: "Team", TargetIDType: reflect.TypeFor[int]()},
			"owner": {Cardinality: entbuilder.O2OUnique, Target: "User", TargetIDType: reflect.TypeFor[int](), Inverse: true},
		},
	}
}

func TestMutation_AddEdgeIDs_M2M(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, edgeTestDescriptor())
	require.NoError(t, m.AddEdgeIDs("teams", 1, 2, 3))
	ids := m.EdgeIDs("teams")
	require.ElementsMatch(t, []any{1, 2, 3}, ids)
	require.ElementsMatch(t, []string{"teams"}, m.AddedEdges())
	added := m.AddedIDs("teams")
	require.ElementsMatch(t, []ent.Value{1, 2, 3}, added)
}

func TestMutation_RemoveEdgeIDs_M2M(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, edgeTestDescriptor())
	require.NoError(t, m.AddEdgeIDs("teams", 1, 2, 3))
	require.NoError(t, m.RemoveEdgeIDs("teams", 2))
	require.ElementsMatch(t, []any{1, 3}, m.EdgeIDs("teams"))
	require.ElementsMatch(t, []any{2}, m.RemovedEdgeIDs("teams"))
	require.ElementsMatch(t, []string{"teams"}, m.RemovedEdges())
	require.ElementsMatch(t, []ent.Value{2}, m.RemovedIDs("teams"))
}

func TestMutation_RemoveEdgeIDs_UniqueErrors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, edgeTestDescriptor())
	require.NoError(t, m.SetEdgeID("owner", 99))
	require.Error(t, m.RemoveEdgeIDs("owner", 99)) // not allowed on unique edges
}

func TestMutation_SetEdgeID_Unique(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, edgeTestDescriptor())
	require.NoError(t, m.SetEdgeID("owner", 7))
	id, ok := m.EdgeID("owner")
	require.True(t, ok)
	require.Equal(t, 7, id)
	require.ElementsMatch(t, []string{"owner"}, m.AddedEdges())
	require.ElementsMatch(t, []ent.Value{7}, m.AddedIDs("owner"))
}

func TestMutation_ClearEdge(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, edgeTestDescriptor())
	require.NoError(t, m.ClearEdge("owner"))
	require.True(t, m.EdgeCleared("owner"))
	require.ElementsMatch(t, []string{"owner"}, m.ClearedEdges())
}

func TestMutation_ClearEdge_UnknownErrors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, edgeTestDescriptor())
	require.Error(t, m.ClearEdge("nope"))
}

func TestMutation_ResetEdge_M2M(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, edgeTestDescriptor())
	require.NoError(t, m.AddEdgeIDs("teams", 1, 2))
	require.NoError(t, m.RemoveEdgeIDs("teams", 1))
	require.NoError(t, m.ClearEdge("teams"))
	require.NoError(t, m.ResetEdge("teams"))
	require.Empty(t, m.EdgeIDs("teams"))
	require.Empty(t, m.RemovedEdgeIDs("teams"))
	require.False(t, m.EdgeCleared("teams"))
}

func TestMutation_AddEdgeIDs_TypeMismatch(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, edgeTestDescriptor())
	require.Error(t, m.AddEdgeIDs("teams", "not-an-int")) // string into int-keyed edge
}

func TestMutation_IDRoundTrip(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, testDescriptor())
	_, ok := m.ID()
	require.False(t, ok)
	m.SetID(42)
	id, ok := m.ID()
	require.True(t, ok)
	require.Equal(t, 42, id)
}

func TestMutation_IDs_RejectsNonUpdateDelete(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, testDescriptor())
	_, err := m.IDs(context.Background())
	require.Error(t, err)
}

func TestMutation_IDs_UsesIDsFunc(t *testing.T) {
	desc := testDescriptor()
	desc.IDsFn = func(ctx context.Context, c any, preds ...func(*sql.Selector)) ([]any, error) {
		return []any{1, 2, 3}, nil
	}
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, desc)
	m.SetIDsFunc(func(ctx context.Context, preds ...func(*sql.Selector)) ([]any, error) {
		return desc.IDsFn(ctx, nil, preds...)
	})
	ids, err := m.IDs(context.Background())
	require.NoError(t, err)
	require.Equal(t, []any{1, 2, 3}, ids)
}

func TestMutation_WhereP(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, testDescriptor())
	p1 := func(s *sql.Selector) { s.Where(sql.EQ("id", 1)) }
	p2 := func(s *sql.Selector) { s.Where(sql.False()) }
	m.WhereP(p1, p2)
	require.Len(t, m.MutationPredicates(), 2)
}

func TestMutation_AddPredicate(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, testDescriptor())
	m.AddPredicate(func(s *sql.Selector) {})
	require.Len(t, m.MutationPredicates(), 1)
}

func appendDescriptor() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"tags": {Type: reflect.TypeFor[[]string](), GoName: "Tags"},
		},
		Edges: map[string]entbuilder.EdgeSpec{},
	}
}

func TestMutation_AppendField_RoundTrip(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, appendDescriptor())
	want := []string{"a", "b"}
	require.NoError(t, m.AppendField("tags", want))
	got, ok := m.AppendedField("tags")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestMutation_AppendedField_Unset(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, appendDescriptor())
	v, ok := m.AppendedField("tags")
	require.False(t, ok)
	require.Nil(t, v)
}

func TestMutation_AppendField_UnknownField_Errors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, appendDescriptor())
	require.Error(t, m.AppendField("nonexistent", []string{"x"}))
}

func TestMutation_AppendField_TypeMismatch_Errors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, appendDescriptor())
	// []int is not []string
	err := m.AppendField("tags", []int{1, 2})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected type")
}

func TestMutation_ResetField_ClearsAppended(t *testing.T) {
	m := entbuilder.NewMutation[testEntity](nil, ent.OpUpdate, appendDescriptor())
	require.NoError(t, m.AppendField("tags", []string{"x"}))
	_, ok := m.AppendedField("tags")
	require.True(t, ok)
	require.NoError(t, m.ResetField("tags"))
	_, ok = m.AppendedField("tags")
	require.False(t, ok)
}

// Compile-time check; will fail to compile if interface drifts.
var _ ent.Mutation = (*entbuilder.Mutation[testEntity])(nil)
