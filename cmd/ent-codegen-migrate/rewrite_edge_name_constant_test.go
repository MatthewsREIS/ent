// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteEdgeNameConstant_AddEdgeIDs(t *testing.T) {
	descs := Descriptors{
		"MarketAgentRegion": &EntityDesc{
			Name: "MarketAgentRegion",
			Edges: map[string]EdgeDesc{
				"properties": {Cardinality: "M2M", TargetIDType: "uuid.UUID", Target: "Property"},
			},
		},
	}
	src := `package x

import (
	"example.com/ent/gen"
)

func hook(m *gen.MarketAgentRegionMutation, anySlice []any) error {
	return m.AddEdgeIDs("properties", anySlice...)
}
`
	out, err := RewriteEdgeNameConstantSource("hook.go", src, descs, "example.com/ent/gen")
	require.NoError(t, err)
	require.Contains(t, out, `m.AddEdgeIDs(marketagentregion.EdgeProperties, anySlice...)`)
	require.Contains(t, out, `"example.com/ent/gen/marketagentregion"`)
	require.NotContains(t, out, `m.AddEdgeIDs("properties"`)
}

func TestRewriteEdgeNameConstant_AllMethods(t *testing.T) {
	descs := Descriptors{
		"Contact": &EntityDesc{
			Name:  "Contact",
			Edges: map[string]EdgeDesc{"primary_address": {Cardinality: "M2O", TargetIDType: "uuid.UUID", Target: "Address"}},
		},
	}
	cases := []struct {
		method string
		call   string
		want   string
	}{
		{"AddEdgeIDs", `m.AddEdgeIDs("primary_address", ids...)`, `m.AddEdgeIDs(contact.EdgePrimaryAddress, ids...)`},
		{"RemoveEdgeIDs", `m.RemoveEdgeIDs("primary_address", ids...)`, `m.RemoveEdgeIDs(contact.EdgePrimaryAddress, ids...)`},
		{"SetEdgeID", `m.SetEdgeID("primary_address", id)`, `m.SetEdgeID(contact.EdgePrimaryAddress, id)`},
		{"ClearEdge", `m.ClearEdge("primary_address")`, `m.ClearEdge(contact.EdgePrimaryAddress)`},
		{"EdgeID", `_, _ = m.EdgeID("primary_address")`, `_, _ = m.EdgeID(contact.EdgePrimaryAddress)`},
		{"EdgeIDs", `_ = m.EdgeIDs("primary_address")`, `_ = m.EdgeIDs(contact.EdgePrimaryAddress)`},
		{"RemovedEdgeIDs", `_ = m.RemovedEdgeIDs("primary_address")`, `_ = m.RemovedEdgeIDs(contact.EdgePrimaryAddress)`},
		{"EdgeCleared", `_ = m.EdgeCleared("primary_address")`, `_ = m.EdgeCleared(contact.EdgePrimaryAddress)`},
		{"ResetEdge", `_ = m.ResetEdge("primary_address")`, `_ = m.ResetEdge(contact.EdgePrimaryAddress)`},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			src := "package x\nimport \"example.com/ent/gen\"\nfunc _f(m *gen.ContactMutation, id any, ids []any) {\n\t" + tc.call + "\n}\n"
			out, err := RewriteEdgeNameConstantSource("x.go", src, descs, "example.com/ent/gen")
			require.NoError(t, err)
			require.Contains(t, out, tc.want)
		})
	}
}

func TestRewriteEdgeNameConstant_SkipsAddedIDsAndRemovedIDs(t *testing.T) {
	descs := Descriptors{
		"Contact": &EntityDesc{
			Name:  "Contact",
			Edges: map[string]EdgeDesc{"properties": {}},
		},
	}
	src := `package x
import "example.com/ent/gen"
func _f(m *gen.ContactMutation) {
	_ = m.AddedIDs("properties")
	_ = m.RemovedIDs("properties")
}
`
	out, err := RewriteEdgeNameConstantSource("x.go", src, descs, "example.com/ent/gen")
	require.NoError(t, err)
	require.Contains(t, out, `m.AddedIDs("properties")`)
	require.Contains(t, out, `m.RemovedIDs("properties")`)
	require.NotContains(t, out, `contact.EdgeProperties`)
}

func TestRewriteEdgeNameConstant_SkipsNonLiteralArg(t *testing.T) {
	descs := Descriptors{
		"Contact": &EntityDesc{Name: "Contact", Edges: map[string]EdgeDesc{"properties": {}}},
	}
	src := `package x
import "example.com/ent/gen"
func _f(m *gen.ContactMutation, edge string) { _ = m.ClearEdge(edge) }
`
	out, err := RewriteEdgeNameConstantSource("x.go", src, descs, "example.com/ent/gen")
	require.NoError(t, err)
	require.Contains(t, out, `m.ClearEdge(edge)`)
}

func TestRewriteEdgeNameConstant_SkipsUnknownEdge(t *testing.T) {
	descs := Descriptors{
		"Contact": &EntityDesc{Name: "Contact", Edges: map[string]EdgeDesc{"properties": {}}},
	}
	src := `package x
import "example.com/ent/gen"
func _f(m *gen.ContactMutation) { _ = m.ClearEdge("not_an_edge") }
`
	out, err := RewriteEdgeNameConstantSource("x.go", src, descs, "example.com/ent/gen")
	require.NoError(t, err)
	require.Contains(t, out, `m.ClearEdge("not_an_edge")`)
}
