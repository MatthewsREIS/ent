// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteTypedEdgeAccessor_OneID(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) OwnerID() (int, bool) { return 0, false }
func use(m *TaskMutation) { id, ok := m.OwnerID(); _ = id; _ = ok }
`
	out, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.EdgeIDAs[int](m, "owner")`)
	require.NotContains(t, out, "m.OwnerID(")
}

func TestRewriteTypedEdgeAccessor_ManyIDs(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) TeamsIDs() []int { return nil }
func use(m *TaskMutation) { ids := m.TeamsIDs(); _ = ids }
`
	out, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.EdgeIDsAs[int](m, "teams")`)
	require.NotContains(t, out, "m.TeamsIDs(")
}

func TestRewriteTypedEdgeAccessor_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) OwnerID() (int, bool) { return 0, false }
func use(m *TaskMutation) { id, ok := m.OwnerID(); _ = id; _ = ok }
`
	pass1, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	pass2, err := RewriteTypedEdgeAccessorSource("x.go", pass1, descs)
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
	require.True(t, strings.Contains(pass2, "entbuilder.EdgeIDAs"), "transformation should persist")
}

func TestRewriteTypedEdgeAccessor_SkipsNonMutation(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type Helper struct{}
func (h *Helper) OwnerID() (int, bool) { return 0, false }
func use(h *Helper) { id, ok := h.OwnerID(); _ = id; _ = ok }
`
	out, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, "h.OwnerID(", "non-mutation receiver must be skipped")
	require.NotContains(t, out, "entbuilder.EdgeIDAs", "no rewrite expected")
}
