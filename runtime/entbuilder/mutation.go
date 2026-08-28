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
