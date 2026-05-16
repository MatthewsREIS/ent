// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteMutation_Setter(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:   "Task",
			Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) SetTitle(s string) {}
func hook(m *TaskMutation) { m.SetTitle("hi") }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField("title", "hi")`)
	require.NotContains(t, out, "m.SetTitle(")
}

func TestRewriteMutation_Getter(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:   "Task",
			Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) Title() (string, bool) { return "", false }
func hook(m *TaskMutation) { v, ok := m.Title(); _ = v; _ = ok }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.GetField[string](m, "title")`)
}

func TestRewriteMutation_EdgeAdd(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) AddTeamIDs(ids ...int) {}
func hook(m *TaskMutation) { m.AddTeamIDs(1, 2, 3) }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, "AddEdgeIDs")
	require.Contains(t, out, `"teams"`)
}

func TestRewriteMutation_PreservesUnrelatedCalls(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{Name: "Task", Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}}},
	}
	src := `package x
import "fmt"
type TaskMutation struct{}
func (m *TaskMutation) SetTitle(s string) {}
func hook(m *TaskMutation) { fmt.Println("hi"); m.SetTitle("hi") }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `fmt.Println`)
	require.Contains(t, out, `m.SetField("title", "hi")`)
}

func TestRewriteMutation_AddsImportWhenNeeded(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{Name: "Task", Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}}},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) Title() (string, bool) { return "", false }
func hook(m *TaskMutation) { _, _ = m.Title() }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.True(t, strings.Contains(out, `"entgo.io/ent/runtime/entbuilder"`), "expected entbuilder import to be added; got:\n%s", out)
}
