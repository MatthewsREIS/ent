// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import "context"

// GetField returns the typed value of a field on the mutation.
// Returns the zero value and false if the field is unset.
func GetField[V any, T any](m *Mutation[T], name string) (V, bool) {
	v, ok := m.Field(name)
	if !ok {
		var z V
		return z, false
	}
	return v.(V), true
}

// OldFieldAs returns the typed pre-mutation value of a field on the
// mutation. Returns the zero value if OldField errors.
func OldFieldAs[V any, T any](ctx context.Context, m *Mutation[T], name string) (V, error) {
	v, err := m.OldField(ctx, name)
	if err != nil {
		var z V
		return z, err
	}
	return v.(V), nil
}

// EdgeIDsAs returns the typed neighbor IDs on an edge.
func EdgeIDsAs[ID any, T any](m *Mutation[T], edge string) []ID {
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
func EdgeIDAs[ID any, T any](m *Mutation[T], edge string) (ID, bool) {
	id, ok := m.EdgeID(edge)
	if !ok {
		var z ID
		return z, false
	}
	return id.(ID), true
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
