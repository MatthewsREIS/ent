// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

// fakeMutation satisfies the minimal ent.Mutation surface for these tests.
type fakeMutation struct{ op ent.Op }

func (m *fakeMutation) Op() ent.Op                          { return m.op }
func (m *fakeMutation) Type() string                        { return "User" }
func (m *fakeMutation) Fields() []string                    { return nil }
func (m *fakeMutation) Field(string) (ent.Value, bool)      { return nil, false }
func (m *fakeMutation) SetField(string, ent.Value) error    { return nil }
func (m *fakeMutation) AddedFields() []string               { return nil }
func (m *fakeMutation) AddedField(string) (ent.Value, bool) { return nil, false }
func (m *fakeMutation) AddField(string, ent.Value) error    { return nil }
func (m *fakeMutation) ClearedFields() []string             { return nil }
func (m *fakeMutation) FieldCleared(string) bool            { return false }
func (m *fakeMutation) ClearField(string) error             { return nil }
func (m *fakeMutation) ResetField(string) error             { return nil }
func (m *fakeMutation) AddedEdges() []string                { return nil }
func (m *fakeMutation) AddedIDs(string) []ent.Value         { return nil }
func (m *fakeMutation) RemovedEdges() []string              { return nil }
func (m *fakeMutation) RemovedIDs(string) []ent.Value       { return nil }
func (m *fakeMutation) ClearedEdges() []string              { return nil }
func (m *fakeMutation) EdgeCleared(string) bool             { return false }
func (m *fakeMutation) ClearEdge(string) error              { return nil }
func (m *fakeMutation) ResetEdge(string) error              { return nil }
func (m *fakeMutation) OldField(context.Context, string) (ent.Value, error) {
	return nil, nil
}

func TestRunUpdate_NoHooks_CallsSqlSave(t *testing.T) {
	state := &entbuilder.UpdateState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpUpdate},
	}
	called := false
	sqlSave := func(context.Context) (int, error) {
		called = true
		return 3, nil
	}
	n, err := entbuilder.RunUpdate(context.Background(), state, sqlSave)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.True(t, called)
}

func TestRunUpdate_SqlSaveError_Propagates(t *testing.T) {
	state := &entbuilder.UpdateState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpUpdate},
	}
	want := errors.New("db locked")
	sqlSave := func(context.Context) (int, error) { return 0, want }
	n, err := entbuilder.RunUpdate(context.Background(), state, sqlSave)
	require.ErrorIs(t, err, want)
	require.Equal(t, 0, n)
}

func TestRunUpdateOne_NoHooks_CallsSqlSave(t *testing.T) {
	state := &entbuilder.UpdateState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpUpdateOne},
	}
	want := &fakeUser{Name: "updated"}
	sqlSave := func(context.Context) (*fakeUser, error) { return want, nil }
	got, err := entbuilder.RunUpdateOne[fakeUser](context.Background(), state, sqlSave)
	require.NoError(t, err)
	require.Same(t, want, got)
}
