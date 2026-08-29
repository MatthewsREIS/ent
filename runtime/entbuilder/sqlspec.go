// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"reflect"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/dialect/sql/sqljson"
)

// ApplyUpdateSpec copies the field/edge state recorded on m onto spec,
// reproducing the exact statement shapes the old per-entity sqlSave bodies
// used to unroll (see entc/gen/template/dialect/sql/update.tmpl):
//
//   - Fields: SetField (if set), AddField (if incremented), an AddModifier
//     wrapping sqljson.Append (if JSON-appended), ClearField (if cleared) —
//     in that order, independently of one another.
//   - Edges: EdgeCleared appends to spec.Edges.Clear; RemovedEdgeIDs (guarded
//     by "!EdgeCleared" so a full clear isn't redundantly paired with a
//     per-ID remove) also appends to spec.Edges.Clear; EdgeIDs appends to
//     spec.Edges.Add. These three checks are independent ifs, not
//     if/else-if — a unique edge replaced via ClearEdge+SetEdgeID legitimately
//     emits both a Clear and an Add spec, matching generated code exactly.
//
// schemaOf resolves a Descriptor/EdgeSpec.SchemaKey to a runtime schema name
// for multischema apps; pass nil for single-schema apps (SchemaKey is then
// never read).
func ApplyUpdateSpec[T, I any](m *Mutation[T, I], spec *sqlgraph.UpdateSpec, schemaOf func(key string) string) {
	desc := m.desc
	if schemaOf != nil && desc.SchemaKey != "" {
		spec.Node.Schema = schemaOf(desc.SchemaKey)
	}
	if ps := m.MutationPredicates(); len(ps) > 0 {
		spec.Predicate = func(s *sql.Selector) {
			for i := range ps {
				ps[i](s)
			}
		}
	}
	// Map iteration order is nondeterministic; each field/edge below writes
	// an independent entry into spec.Fields.*/spec.Edges.* (or an
	// independent AddModifier), so the resulting spec doesn't depend on the
	// order fields/edges are visited in.
	for name, f := range desc.Fields {
		if v, ok := m.Field(name); ok {
			spec.SetField(f.Column, f.SQLType, v)
		}
		if v, ok := m.AddedField(name); ok {
			spec.AddField(f.Column, f.SQLType, v)
		}
		if v, ok := m.AppendedField(name); ok {
			column, elems := f.Column, sliceToAny(v)
			spec.AddModifier(func(u *sql.UpdateBuilder) {
				sqljson.Append(u, column, elems)
			})
		}
		if m.FieldCleared(name) {
			spec.ClearField(f.Column, f.SQLType)
		}
	}
	for name, e := range desc.Edges {
		build := func() *sqlgraph.EdgeSpec {
			edge := &sqlgraph.EdgeSpec{
				Rel:     e.Rel,
				Inverse: e.Inverse,
				Table:   e.StorageTable,
				Columns: e.StorageColumns,
				Bidi:    e.Bidi,
				Target: &sqlgraph.EdgeTarget{
					IDSpec: sqlgraph.NewFieldSpec(e.TargetIDColumn, e.TargetIDSQLType),
				},
			}
			if schemaOf != nil && e.SchemaKey != "" {
				edge.Schema = schemaOf(e.SchemaKey)
			}
			return edge
		}
		if m.EdgeCleared(name) {
			spec.Edges.Clear = append(spec.Edges.Clear, build())
		}
		if nodes := m.RemovedEdgeIDs(name); len(nodes) > 0 && !m.EdgeCleared(name) {
			edge := build()
			for _, k := range nodes {
				edge.Target.Nodes = append(edge.Target.Nodes, k)
			}
			spec.Edges.Clear = append(spec.Edges.Clear, edge)
		}
		if nodes := m.EdgeIDs(name); len(nodes) > 0 {
			edge := build()
			for _, k := range nodes {
				edge.Target.Nodes = append(edge.Target.Nodes, k)
			}
			spec.Edges.Add = append(spec.Edges.Add, edge)
		}
	}
}

// ApplyCreateSpec copies the field/edge state recorded on m onto spec,
// reproducing the create-side shapes createSpec() used to unroll: SetField
// per set field, and one Add-only edge append per populated edge (a create
// has nothing to clear or remove). See entc/integration/ent/*/create.go's
// createSpec for the generated shape this mirrors.
func ApplyCreateSpec[T, I any](m *Mutation[T, I], spec *sqlgraph.CreateSpec, schemaOf func(key string) string) {
	desc := m.desc
	if schemaOf != nil && desc.SchemaKey != "" {
		spec.Schema = schemaOf(desc.SchemaKey)
	}
	for name, f := range desc.Fields {
		if v, ok := m.Field(name); ok {
			spec.SetField(f.Column, f.SQLType, v)
		}
	}
	for name, e := range desc.Edges {
		nodes := m.EdgeIDs(name)
		if len(nodes) == 0 {
			continue
		}
		edge := &sqlgraph.EdgeSpec{
			Rel:     e.Rel,
			Inverse: e.Inverse,
			Table:   e.StorageTable,
			Columns: e.StorageColumns,
			Bidi:    e.Bidi,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(e.TargetIDColumn, e.TargetIDSQLType),
			},
		}
		if schemaOf != nil && e.SchemaKey != "" {
			edge.Schema = schemaOf(e.SchemaKey)
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		spec.Edges = append(spec.Edges, edge)
	}
}

// sliceToAny converts a boxed slice value (e.g. []string, []int) to []any by
// reflection, for sqljson.Append's generic elems parameter. Append[T any]
// itself does nothing more than this same element-by-element copy into an
// []any before handing it to the driver (see dialect/sql/sqljson.Append), so
// calling Append[any] with an already-[]any slice reproduces byte-identical
// behavior to the generated code's Append[T] call with the concrete slice
// type — this is just that same conversion, done here because the concrete
// element type T isn't known at this generic call site.
func sliceToAny(v any) []any {
	rv := reflect.ValueOf(v)
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out
}
