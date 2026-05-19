// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"fmt"

	"entgo.io/ent"
)

// GetField returns the typed value of a field on the mutation.
// Returns the zero value and false if the field is unset.
func GetField[V any, T any, I any](m *Mutation[T, I], name string) (V, bool) {
	v, ok := m.Field(name)
	if !ok {
		var z V
		return z, false
	}
	return v.(V), true
}

// OldFieldAs returns the typed pre-mutation value of a field on the
// mutation. Returns the zero value if OldField errors.
func OldFieldAs[V any, T any, I any](ctx context.Context, m *Mutation[T, I], name string) (V, error) {
	v, err := m.OldField(ctx, name)
	if err != nil {
		var z V
		return z, err
	}
	return v.(V), nil
}

// EdgeIDsAs returns the typed neighbor IDs on an edge.
func EdgeIDsAs[ID any, T any, I any](m *Mutation[T, I], edge string) []ID {
	ids := m.EdgeIDs(edge)
	if ids == nil {
		return nil
	}
	out := make([]ID, len(ids))
	for i, id := range ids {
		out[i] = id.(ID)
	}
	return out
}

// EdgeIDAs returns the typed neighbor ID on a unique edge.
func EdgeIDAs[ID any, T any, I any](m *Mutation[T, I], edge string) (ID, bool) {
	id, ok := m.EdgeID(edge)
	if !ok {
		var z ID
		return z, false
	}
	return id.(ID), true
}

// RemovedEdgeIDsAs returns the typed neighbor IDs marked as removed from
// an edge. Pre-PR-6 the per-entity Mutation exposed Removed<Edge>IDs() with
// the entity-specific ID type; post-PR-6 the generic RemovedEdgeIDs returns
// []any, leaving consumers to iterate with a type assertion. This helper
// supplies the typed slice in one call, matching the read-side EdgeIDsAs
// shape.
func RemovedEdgeIDsAs[ID any, T any, I any](m *Mutation[T, I], edge string) []ID {
	ids := m.RemovedEdgeIDs(edge)
	if ids == nil {
		return nil
	}
	out := make([]ID, len(ids))
	for i, id := range ids {
		out[i] = id.(ID)
	}
	return out
}

// ToAny converts a typed slice to a slice of any. Used by generated code to
// pass typed IDs through the string-keyed Mutation API.
func ToAny[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// edgeSetter is the post-PR-6 generic-mutation interface for setting a
// neighbor ID on a unique edge by name. Every *Mutation[T, I] satisfies it
// (SetEdgeID is declared on the generic type), but ent.Mutation does not
// include the method in its public interface — so consumer hooks that
// receive an `ent.Mutation` need a typed bridge. SetEdgeID and EdgeID below
// provide that bridge with a single clear failure path.
type edgeSetter interface {
	SetEdgeID(edge string, id any) error
}

type edgeGetter interface {
	EdgeID(edge string) (any, bool)
}

// SetEdgeID sets the neighbor ID on a unique edge of m. m must satisfy the
// post-PR-6 *Mutation[T, I] SetEdgeID method; consumers typically receive
// m typed as ent.Mutation in hooks and previously called SetField with the
// raw FK column name. Returns a wrapped error when m is not an *Mutation
// (defensive — every generated entity mutation implements it).
func SetEdgeID(m ent.Mutation, edge string, id any) error {
	es, ok := m.(edgeSetter)
	if !ok {
		return fmt.Errorf("entbuilder: SetEdgeID: %T does not implement SetEdgeID(string, any) error", m)
	}
	return es.SetEdgeID(edge, id)
}

// EdgeID returns the neighbor ID on a unique edge of m. Mirrors SetEdgeID's
// interface-bridge for the read path: lets consumer code that holds an
// ent.Mutation read an edge's neighbor ID by name without per-entity casts.
// Returns (nil, false) when the assertion fails so callers can fall back
// the same way as if the edge were simply unset.
func EdgeID(m ent.Mutation, edge string) (any, bool) {
	eg, ok := m.(edgeGetter)
	if !ok {
		return nil, false
	}
	return eg.EdgeID(edge)
}
