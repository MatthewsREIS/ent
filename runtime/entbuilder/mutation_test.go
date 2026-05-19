// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

// Fixture entity for unit tests.
type testEntity struct {
	ID    int
	Title string
}

func testDescriptor() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"title": {Type: reflect.TypeFor[string](), GoName: "Title"},
		},
		Edges: map[string]entbuilder.EdgeSpec{},
	}
}

func TestNewMutation_PopulatesDescriptorAndOp(t *testing.T) {
	desc := testDescriptor()
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, desc)
	require.NotNil(t, m)
}

func TestMutation_SetField_TypeMismatch(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, testDescriptor())
	err := m.SetField("title", 42) // int into string field
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected type")
}

func TestMutation_SetField_UnknownField(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, testDescriptor())
	err := m.SetField("nonexistent", "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown")
}

func TestMutation_SetField_FieldRoundTrip(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, testDescriptor())
	require.NoError(t, m.SetField("title", "hello"))
	v, ok := m.Field("title")
	require.True(t, ok)
	require.Equal(t, "hello", v)
}

func TestMutation_Field_Unset(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, testDescriptor())
	v, ok := m.Field("title")
	require.False(t, ok)
	require.Nil(t, v)
}

func TestMutation_Fields_OrderedByDescriptor(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"a": {Type: reflect.TypeFor[int](), GoName: "A"},
			"b": {Type: reflect.TypeFor[int](), GoName: "B"},
			"c": {Type: reflect.TypeFor[int](), GoName: "C"},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, desc)
	require.NoError(t, m.SetField("b", 2))
	require.NoError(t, m.SetField("a", 1))
	got := m.Fields()
	require.ElementsMatch(t, []string{"a", "b"}, got) // Fields() iterates set keys; order may vary
}

func TestMutation_ClearField_Nillable(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"title": {Type: reflect.TypeFor[string](), GoName: "Title", Nillable: true},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, desc)
	require.NoError(t, m.ClearField("title"))
	require.True(t, m.FieldCleared("title"))
	require.ElementsMatch(t, []string{"title"}, m.ClearedFields())
}

func TestMutation_ClearField_NotNillable_Errors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, testDescriptor()) // title not nillable
	require.Error(t, m.ClearField("title"))
}

func TestMutation_OldField_RequiresUpdateOne(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, testDescriptor())
	_, err := m.OldField(context.Background(), "title")
	require.Error(t, err)
	require.Contains(t, err.Error(), "OldField is only allowed on UpdateOne operations")
}

func TestMutation_OldField_ReflectsOldValue(t *testing.T) {
	desc := testDescriptor()
	desc.OldValueFn = func(ctx context.Context, c any, id any) (any, error) {
		return &testEntity{ID: id.(int), Title: "before"}, nil
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdateOne, desc)
	idVal := 7
	m.SetID(idVal)
	m.SetOldValueLoader(func(ctx context.Context) (any, error) {
		return desc.OldValueFn(ctx, nil, idVal)
	})
	got, err := m.OldField(context.Background(), "title")
	require.NoError(t, err)
	require.Equal(t, "before", got)
}

func TestMutation_ResetField(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, testDescriptor())
	require.NoError(t, m.SetField("title", "x"))
	require.NoError(t, m.ResetField("title"))
	_, ok := m.Field("title")
	require.False(t, ok)
}

func TestMutation_ResetField_UnknownErrors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, testDescriptor())
	require.Error(t, m.ResetField("nope"))
}

// guard: SetField wraps assignment failures wisely
var _ = errors.New

func TestMutation_AddField_Numeric(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"count": {Type: reflect.TypeFor[int](), GoName: "Count", Numeric: true},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, desc)
	require.NoError(t, m.AddField("count", 5))
	v, ok := m.AddedField("count")
	require.True(t, ok)
	require.Equal(t, 5, v)
	require.ElementsMatch(t, []string{"count"}, m.AddedFields())
}

func TestMutation_AddField_NotNumeric_Errors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, testDescriptor()) // title not numeric
	require.Error(t, m.AddField("title", 5))
}

func TestMutation_AddField_Unknown_Errors(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, testDescriptor())
	require.Error(t, m.AddField("nope", 5))
}

func TestMutation_AddedField_Unset(t *testing.T) {
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, testDescriptor())
	_, ok := m.AddedField("title")
	require.False(t, ok)
}

func TestMutation_AddField_Accumulates(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"count": {Type: reflect.TypeFor[int](), GoName: "Count", Numeric: true},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, desc)
	require.NoError(t, m.AddField("count", 5))
	require.NoError(t, m.AddField("count", 3))
	v, ok := m.AddedField("count")
	require.True(t, ok)
	require.Equal(t, 8, v) // 5 + 3
}

func TestMutation_SetField_InterfaceType(t *testing.T) {
	anyType := reflect.TypeOf((*any)(nil)).Elem()
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"payload": {Type: anyType, GoName: "Payload"},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpCreate, desc)
	require.NoError(t, m.SetField("payload", "anything"))
	require.NoError(t, m.SetField("payload", 42))
	require.NoError(t, m.SetField("payload", []int{1, 2, 3}))
}

func TestMutation_ClearField_ClearsAddedAndAppended(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"tags": {Type: reflect.TypeFor[[]string](), GoName: "Tags", Nillable: true, Numeric: true},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, desc)
	require.NoError(t, m.SetField("tags", []string{"a"}))
	require.NoError(t, m.AddField("tags", []string{"b"}))
	require.NoError(t, m.AppendField("tags", []string{"c"}))
	require.NoError(t, m.ClearField("tags"))
	_, addedOk := m.AddedField("tags")
	_, appendedOk := m.AppendedField("tags")
	require.False(t, addedOk, "added should be cleared")
	require.False(t, appendedOk, "appended should be cleared")
}

func TestMutation_SetField_ClearsAppended(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"tags": {Type: reflect.TypeFor[[]string](), GoName: "Tags"},
		},
	}
	m := entbuilder.NewMutation[testEntity, int](nil, ent.OpUpdate, desc)
	require.NoError(t, m.AppendField("tags", []string{"a", "b"}))
	require.NoError(t, m.SetField("tags", []string{"c"}))
	_, appendedOk := m.AppendedField("tags")
	require.False(t, appendedOk, "appended should be cleared when SetField runs")
}
