// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import "entgo.io/ent/dialect/sql/sqlgraph"

// BuildSchemaGraph assembles one sqlgraph.Schema from every entity's
// descriptor in a generated package tree, replacing the per-entity
// schemaGraph literal each entity's shared.go used to build on its own
// (self node plus a stub node per distinct edge-target type). With every
// entity's descriptor available here, all nodes are real — no stubs needed.
//
// Node order follows descs order; NodeIndex (or sqlgraph.Schema.EvalP,
// which does its own linear scan by node.Type) is how callers find a node,
// so descs order carries no meaning beyond determinism.
func BuildSchemaGraph(descs []*Descriptor) *sqlgraph.Schema {
	graph := &sqlgraph.Schema{Nodes: make([]*sqlgraph.Node, len(descs))}
	for i, d := range descs {
		graph.Nodes[i] = buildNode(d)
	}
	// Edges are added in a second pass: MustAddE looks up both the from and
	// to node by Type name, so every node must already be in the graph.
	for _, d := range descs {
		for name, e := range d.GraphEdges {
			graph.MustAddE(name, &sqlgraph.EdgeSpec{
				Rel:     e.Rel,
				Inverse: e.Inverse,
				Table:   e.StorageTable,
				Columns: e.StorageColumns,
				Bidi:    e.Bidi,
			}, d.Name, e.Target)
		}
	}
	return graph
}

func buildNode(d *Descriptor) *sqlgraph.Node {
	n := &sqlgraph.Node{
		NodeSpec: sqlgraph.NodeSpec{
			Table:   d.Table,
			Columns: d.TableColumns,
		},
		Type:   d.Name,
		Fields: make(map[string]*sqlgraph.FieldSpec, len(d.GraphFields)),
	}
	if len(d.CompositeID) > 0 {
		n.CompositeID = make([]*sqlgraph.FieldSpec, len(d.CompositeID))
		for i, c := range d.CompositeID {
			n.CompositeID[i] = &sqlgraph.FieldSpec{Type: c.SQLType, Column: c.Column}
		}
	} else {
		n.ID = &sqlgraph.FieldSpec{Type: d.IDSQLType, Column: d.IDColumn}
	}
	// Keyed by storage column name (not the schema field name that keys
	// Descriptor.Fields/GraphFields' Go-side counterpart) — entql predicate
	// field references are always by storage key (see entql_type.tmpl's
	// p.Field(<Constant>) calls, where Constant's value is the storage key),
	// matching what the old per-entity schemaGraph literal keyed on too.
	for column, typ := range d.GraphFields {
		n.Fields[column] = &sqlgraph.FieldSpec{Type: typ, Column: column}
	}
	return n
}

// NodeIndex returns the index of the node with the given type name in
// graph.Nodes, or -1 if not found. Not used by entql (sqlgraph.Schema.EvalP
// already looks nodes up by name on its own), but generated per-package as
// a convenience for other by-name node lookups.
func NodeIndex(graph *sqlgraph.Schema, typ string) int {
	for i, n := range graph.Nodes {
		if n.Type == typ {
			return i
		}
	}
	return -1
}
