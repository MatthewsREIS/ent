// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteEdgeFKSetField_SetField(t *testing.T) {
	descs := Descriptors{
		"Account": &EntityDesc{
			Name: "Account",
			Edges: map[string]EdgeDesc{
				"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID", Field: "owner_id"},
			},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation, id any) error {
	return m.SetField("owner_id", id)
}
`
	out, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.SetEdgeID(m, "owner", id)`)
	require.NotContains(t, out, `m.SetField("owner_id"`)
	require.Contains(t, out, `"entgo.io/ent/runtime/entbuilder"`)
}

func TestRewriteEdgeFKSetField_Field(t *testing.T) {
	descs := Descriptors{
		"Account": &EntityDesc{
			Name: "Account",
			Edges: map[string]EdgeDesc{
				"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID", Field: "owner_id"},
			},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation) (ent.Value, bool) {
	return m.Field("owner_id")
}
`
	out, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.EdgeID(m, "owner")`)
	require.NotContains(t, out, `m.Field("owner_id"`)
}

func TestRewriteEdgeFKSetField_AggregatesAcrossEntities(t *testing.T) {
	// Two entities, both with "owner_id" backing edge "owner" — consistent,
	// so the rewrite fires. Schema authors name FK columns after the edge.
	descs := Descriptors{
		"Account": &EntityDesc{
			Name:  "Account",
			Edges: map[string]EdgeDesc{"owner": {Field: "owner_id", Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID"}},
		},
		"Contact": &EntityDesc{
			Name:  "Contact",
			Edges: map[string]EdgeDesc{"owner": {Field: "owner_id", Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID"}},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation, id any) error { return m.SetField("owner_id", id) }
`
	out, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.SetEdgeID(m, "owner", id)`)
}

func TestRewriteEdgeFKSetField_AmbiguousFKIsSkipped(t *testing.T) {
	// "fk" maps to "edgeA" in Account and "edgeB" in Contact — ambiguous.
	// Skipping is the safe default: a missed rewrite is fixable; a wrong
	// rewrite (calling SetEdgeID with the wrong edge name) is silent.
	descs := Descriptors{
		"Account": &EntityDesc{
			Name:  "Account",
			Edges: map[string]EdgeDesc{"edgeA": {Field: "fk", Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
		"Contact": &EntityDesc{
			Name:  "Contact",
			Edges: map[string]EdgeDesc{"edgeB": {Field: "fk", Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation, id any) error { return m.SetField("fk", id) }
`
	out, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField("fk", id)`, "ambiguous FK column must be left as-is")
	require.NotContains(t, out, "entbuilder.SetEdgeID")
}

func TestRewriteEdgeFKSetField_NonEdgeFKLeftAlone(t *testing.T) {
	// "title" is a scalar field, not an edge FK column. The pass must not
	// touch SetField on it — that's rewrite_mutation's domain (and a
	// generic SetField on a scalar already works at runtime).
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:   "Task",
			Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}},
			Edges:  map[string]EdgeDesc{},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation) error { return m.SetField("title", "hi") }
`
	out, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField("title", "hi")`)
	require.NotContains(t, out, "entbuilder.SetEdgeID")
}

func TestRewriteEdgeFKSetField_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Account": &EntityDesc{
			Name:  "Account",
			Edges: map[string]EdgeDesc{"owner": {Field: "owner_id", Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID"}},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation, id any) error { return m.SetField("owner_id", id) }
`
	pass1, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	pass2, err := RewriteEdgeFKSetFieldSource("hook.go", pass1, descs, "")
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
	require.True(t, strings.Contains(pass2, "entbuilder.SetEdgeID"))
}

func TestRewriteEdgeFKSetField_SkipsSchemaDSLFalsePositive(t *testing.T) {
	// Regression: edge.To(...).Field("owner_id") in a schema Edges() method
	// must not be rewritten. The receiver is *assocBuilder, not a mutation.
	// Before the receiver-type gate landed, the pass rewrote this DSL chain
	// into entbuilder.SetEdgeID(edge.To(...), "owner", ...) and corrupted
	// every schema file with a FK-backed edge.
	descs := Descriptors{
		"Account": &EntityDesc{
			Name:  "Account",
			Edges: map[string]EdgeDesc{"owner": {Field: "owner_id", Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID"}},
		},
	}
	src := `package x
type assocBuilder struct{}
func (b *assocBuilder) Field(s string) *assocBuilder { return b }
func edgeTo() *assocBuilder { return &assocBuilder{} }
func Edges() { edgeTo().Field("owner_id") }
`
	out, err := RewriteEdgeFKSetFieldSource("schema.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `edgeTo().Field("owner_id")`)
	require.NotContains(t, out, "entbuilder.EdgeID")
}

func TestRewriteEdgeFKSetField_PreservesNonStringLiteralCallsite(t *testing.T) {
	// SetField with a non-literal field name (e.g. dynamic from a variable)
	// can't be statically classified — leave it alone.
	descs := Descriptors{
		"Account": &EntityDesc{
			Name:  "Account",
			Edges: map[string]EdgeDesc{"owner": {Field: "owner_id", Cardinality: "entbuilder.M2O", TargetIDType: "uuid.UUID"}},
		},
	}
	src := `package x
import "entgo.io/ent"
func hook(m ent.Mutation, name string, id any) error { return m.SetField(name, id) }
`
	out, err := RewriteEdgeFKSetFieldSource("hook.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `m.SetField(name, id)`)
	require.NotContains(t, out, "entbuilder.SetEdgeID")
}
