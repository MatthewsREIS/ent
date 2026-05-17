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

func TestRewriteMutation_SkipsSchemaDSLFalsePositive(t *testing.T) {
	// Simulates the schema-DSL false-positive bug: a non-mutation receiver
	// (e.g. schema field-builder) with a method name that overlaps a
	// descriptor's known field (e.g. SetTitle) must not be rewritten.
	// Before the fix: the matcher dispatched purely on method name and
	// would rewrite f.SetTitle("hello") → f.SetField("title", "hello")
	// on any receiver, corrupting consumer schema code.
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:   "Task",
			Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}},
		},
	}
	src := `package x
type SchemaField struct{}
func (f *SchemaField) SetTitle(s string) *SchemaField { return f }
func use(f *SchemaField) { f.SetTitle("hello") }
`
	out, err := RewriteMutationSource("schema.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `f.SetTitle("hello")`, "non-mutation receiver must be skipped")
	require.NotContains(t, out, "SetField", "must not rewrite to mutation API on non-mutation receiver")
	require.NotContains(t, out, "entbuilder", "must not add entbuilder import for skipped rewrite")
}

func TestRewriteMutation_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:   "Task",
			Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}},
			Edges:  map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) SetTitle(s string) {}
func (m *TaskMutation) Title() (string, bool) { return "", false }
func (m *TaskMutation) AddTeamIDs(ids ...int) {}
func hook(m *TaskMutation) {
	m.SetTitle("hi")
	v, _ := m.Title()
	_ = v
	m.AddTeamIDs(1, 2)
}
`
	pass1, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.NotEqual(t, src, pass1, "first pass must transform the source")

	pass2, err := RewriteMutationSource("hook.go", pass1, descs)
	require.NoError(t, err)
	require.Equal(t, pass1, pass2, "second pass must be a no-op (idempotent)")
}

func TestRewriteMutation_SetTypedField_SnakeCase(t *testing.T) {
	// Multi-word PascalCase method name must lookup against the snake_case
	// descriptor key. Previously lcFirst("WrikeTitle") = "wrikeTitle" missed
	// the descriptor entry "wrike_title" and the call was left un-rewritten.
	descs := Descriptors{
		"WrikeProject": &EntityDesc{
			Name:   "WrikeProject",
			Fields: map[string]FieldDesc{"wrike_title": {GoName: "WrikeTitle", Type: "string"}},
		},
	}
	src := `package x
type WrikeProjectMutation struct{}
func (m *WrikeProjectMutation) SetWrikeTitle(s string) {}
func hook(m *WrikeProjectMutation) { m.SetWrikeTitle("foo") }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField("wrike_title", "foo")`)
	require.NotContains(t, out, "m.SetWrikeTitle(")
}

func TestRewriteMutation_SetXID_FieldFallback(t *testing.T) {
	// SetFolderID where the schema declares a typed field "folder_id"
	// (not an edge "folder") must fall back to the field setter form,
	// not be left un-rewritten as a setEdge that misses.
	descs := Descriptors{
		"BoxFolder": &EntityDesc{
			Name:   "BoxFolder",
			Fields: map[string]FieldDesc{"folder_id": {GoName: "FolderID", Type: "string"}},
		},
	}
	src := `package x
type BoxFolderMutation struct{}
func (m *BoxFolderMutation) SetFolderID(s string) {}
func hook(m *BoxFolderMutation) { m.SetFolderID("xyz") }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField("folder_id", "xyz")`)
	require.NotContains(t, out, "m.SetFolderID(")
	require.NotContains(t, out, "SetEdgeID", "must not emit SetEdgeID when target is a field, not an edge")
}

func TestRewriteMutation_SetTypedField_DigitSeparator(t *testing.T) {
	// Consumer schemas often declare fields like "installment_1_pay_status"
	// (digit as its own snake token). pascalToSnake must insert underscores
	// around digit runs so the method "SetInstallment1PayStatus" resolves
	// to the descriptor's "installment_1_pay_status" entry.
	descs := Descriptors{
		"Escrow": &EntityDesc{
			Name:   "Escrow",
			Fields: map[string]FieldDesc{"installment_1_pay_status": {GoName: "Installment1PayStatus", Type: "string"}},
		},
	}
	src := `package x
type EscrowMutation struct{}
func (m *EscrowMutation) SetInstallment1PayStatus(s string) {}
func hook(m *EscrowMutation) { m.SetInstallment1PayStatus("PAID") }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField("installment_1_pay_status", "PAID")`)
	require.NotContains(t, out, "m.SetInstallment1PayStatus(")
}

func TestRewriteMutation_SetXID_EdgePreferred(t *testing.T) {
	// When both an edge "owner" exists AND no field "owner_id" exists,
	// SetOwnerID must resolve to the edge form, not be reclassified as
	// a field set. The field fallback is strictly for the no-edge case.
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) SetOwnerID(id int) {}
func hook(m *TaskMutation) { m.SetOwnerID(42) }
`
	out, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `m.SetEdgeID("owner", 42)`)
	require.NotContains(t, out, "m.SetOwnerID(")
	require.NotContains(t, out, "SetField", "must not reclassify when an edge match exists")
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
