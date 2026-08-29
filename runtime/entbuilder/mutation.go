// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"reflect"
	"sync"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
)

// Cardinality classifies an edge.
type Cardinality uint8

const (
	// O2OUnique edges hold at most one neighbor.
	O2OUnique Cardinality = iota
	// O2M edges hold zero-or-more neighbors with no inverse multiplicity.
	O2M
	// M2O edges are the inverse of O2M.
	M2O
	// M2M edges hold zero-or-more neighbors on both sides.
	M2M
)

// FieldSpec describes a scalar field on an entity.
type FieldSpec struct {
	// Type is the expected Go type for SetField validation.
	Type reflect.Type
	// GoName is the exported struct field name on the entity type
	// (e.g. "Title"). Used by OldField to read the field via reflect.
	GoName string
	// Nillable allows ClearField to operate on this field.
	Nillable bool
	// Numeric allows AddField to operate on this field (increment/decrement).
	Numeric bool
	// Default is the value ResetField restores if non-nil.
	Default any
	// Immutable blocks SetField/AddField/AppendField/ClearField on an
	// Update/UpdateOne mutation (Create is unaffected — an immutable field
	// is still settable once, at creation). Mirrors what the old per-field
	// codegen enforced structurally, by generating Set<F> only on the
	// create builder for an immutable field, never on Update/UpdateOne:
	// entfield's F.<Field> handles are shared across every builder type, so
	// that structural gate is gone and this replaces it.
	Immutable bool

	// Column is the SQL column name backing this field. Zero value ("")
	// means "not emitted" — old descriptors (or non-SQL dialects) are
	// unaffected; ApplyUpdateSpec/ApplyCreateSpec are SQL-only consumers.
	Column string
	// SQLType is the schema/field type enum passed to
	// sqlgraph.*Spec.SetField/AddField/ClearField.
	SQLType field.Type
}

// EdgeSpec describes an edge on an entity.
type EdgeSpec struct {
	Cardinality  Cardinality
	Target       string
	TargetIDType reflect.Type
	Inverse      bool
	// Field is the schema name of the foreign-key column that backs this
	// edge, declared in the schema as `edge.To(...).Field("X_id")` or
	// `edge.From(...).Ref(...).Field("X_id")`. Empty when the edge has no
	// dedicated FK column (e.g. M2M relations or edges where the FK was
	// not exposed as a field). Migration tools read this to rewrite
	// `m.SetField("X_id", v)` to `m.SetEdgeID("X", v)`.
	Field string
	// Immutable blocks SetEdgeID/AddEdgeIDs/RemoveEdgeIDs/ClearEdge on an
	// Update/UpdateOne mutation, mirroring FieldSpec.Immutable. entfield's
	// E.<Edge> and F.<EdgeField> handles both route through these methods
	// (see EdgeField[T].Set/Clear in runtime/entfield), so this one gate
	// covers both surfaces.
	Immutable bool

	// Rel is the sqlgraph relation kind (O2O/O2M/M2O/M2M) for this edge.
	Rel sqlgraph.Rel
	// StorageTable is the sqlgraph table name: the neighbor's table for
	// FK edges, or the join table for M2M edges. Named "Storage" (not
	// "Table") to avoid confusion with Descriptor.Table.
	StorageTable string
	// StorageColumns holds the relation column(s): one FK column for
	// O2O/O2M/M2O edges, or the two join-table columns (owner, reference)
	// for M2M edges — mirrors gen.Relation.Columns verbatim.
	StorageColumns []string
	// Bidi mirrors sqlgraph.EdgeSpec.Bidi (bidirectional self-reference,
	// e.g. User.friends).
	Bidi bool
	// TargetIDColumn is the neighbor's ID column name.
	TargetIDColumn string
	// TargetIDSQLType is the neighbor's ID field.Type.
	TargetIDSQLType field.Type
	// SchemaKey is the SchemaConfig struct-field name used to resolve this
	// edge's sqlgraph.EdgeSpec.Schema at runtime (via the ApplyUpdateSpec/
	// ApplyCreateSpec schemaOf callback); "" for single-schema apps.
	//
	// Deliberately not duplicating sqlgraph.EdgeSpec.Inverse as a separate
	// "StorageInverse" field: entc/gen's Edge.IsInverse() (used by
	// update.tmpl to set EdgeSpec.Inverse) and the graph-level Inverse
	// field above (used by internal_mutation.tmpl) are both exactly
	// `e.Inverse != ""` — the same boolean, computed from the same source.
	// Verified by diffing generated internal/user_mutation.go's Inverse:
	// true edges (followers, children) against entc/integration/ent/user/
	// update.go's per-edge Inverse: true/false lines — identical set.
	SchemaKey string
}

// Descriptor is the static, package-init-time descriptor for one entity.
// One *Descriptor instance per entity, shared across all Mutation[T] of
// that entity.
type Descriptor struct {
	Name   string
	IDType reflect.Type
	Fields map[string]FieldSpec
	Edges  map[string]EdgeSpec

	// IDField is the schema-declared name of a user-defined ID field (e.g.
	// "id"), or "" for an auto-generated ID. A user-defined ID is never
	// itself a key in Fields (see internal_mutation.tmpl's MutationFields) —
	// it's carried on the typed *Mutation.id, set/read via SetID/ID, not
	// through the generic Fields map. SetField routes a call addressed to
	// this name through SetID instead, so entfield's generic per-field Set
	// (which only knows the field's schema name, and calls SetField with
	// it) works for a user-defined ID field the same as any other field.
	IDField string

	// OldValueFn fetches the existing entity for OldField support.
	// Returns the entity boxed as `any` (Mutation reads via reflect).
	// The Config parameter is the per-package Config (opaque to entbuilder).
	OldValueFn func(ctx context.Context, c any, id any) (any, error)

	// IDsFn implements IDs(ctx) for Update/Delete mutations.
	// Returns []any (mutation type-asserts to the entity's actual ID slice).
	IDsFn func(ctx context.Context, c any, preds ...func(*sql.Selector)) ([]any, error)

	// Table is the SQL table name for this entity. Zero value ("") means
	// "not emitted" (non-SQL dialects, or old descriptors).
	Table string
	// TableColumns is the full column list for this table, including the
	// ID column first (when the entity has a single-field ID).
	TableColumns []string
	// IDColumn is the ID column name; empty for composite/edge-schema IDs.
	IDColumn string
	// IDSQLType is the ID field's field.Type; zero value for composite IDs.
	IDSQLType field.Type
	// SchemaKey is the SchemaConfig struct-field name used to resolve
	// sqlgraph.NodeSpec.Schema at runtime; "" for single-schema apps.
	SchemaKey string
}

// Mutation is the single generic mutation type used by every entity.
// T is a phantom marker used by typed helpers (GetField, OldFieldAs, etc.)
// but holds no T-typed fields on the mutation itself. I is the entity's ID
// type — propagated through ID() and SetID() so per-entity mutation aliases
// preserve the typed ID accessor that consumer hooks rely on.
type Mutation[T any, I any] struct {
	// Config is the per-package Config; opaque to entbuilder.
	Config any

	desc *Descriptor

	op  ent.Op
	id  *I
	typ string

	// Field state (lazy-allocated).
	fields   map[string]any      // set values keyed by schema field name
	cleared  map[string]struct{} // cleared fields + cleared edges
	added    map[string]any      // numeric increments
	appended map[string]any      // JSON slice appends

	// Edge state (lazy-allocated).
	edges        map[string]map[any]struct{} // edge name → neighbor ID set
	removedEdges map[string]map[any]struct{} // M2M only

	// Lifecycle.
	done       bool
	oldValue   func(context.Context) (any, error)
	oldOnce    sync.Once
	oldCached  any
	oldErr     error
	predicates []func(*sql.Selector)
	idsFunc    func(context.Context, ...func(*sql.Selector)) ([]any, error)
}

// NewMutation constructs a generic mutation for an entity.
func NewMutation[T any, I any](c any, op ent.Op, desc *Descriptor, opts ...func(*Mutation[T, I])) *Mutation[T, I] {
	m := &Mutation[T, I]{
		Config: c,
		desc:   desc,
		op:     op,
		typ:    desc.Name,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Compile-time assertion that Mutation[T, I] satisfies ent.Mutation.
// A single concrete instantiation suffices (ent.Mutation has no ID method,
// so I's choice here is immaterial — `any` keeps the assertion stable for
// entities with composite or otherwise non-uniform ID types).
var _ ent.Mutation = (*Mutation[struct{}, any])(nil)
