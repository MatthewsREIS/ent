// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
)

// Op returns the operation name.
func (m *Mutation[T, I]) Op() ent.Op { return m.op }

// SetOp allows setting the mutation operation.
func (m *Mutation[T, I]) SetOp(op ent.Op) { m.op = op }

// Type returns the schema type for this mutation.
func (m *Mutation[T, I]) Type() string { return m.typ }

// SetID stores the entity ID on the mutation. Called by per-entity option
// constructors (WithXID) and by the CUD builders after a successful create.
// Storage is via a *I so the zero value of I (e.g. 0 / uuid.UUID{}) remains
// distinguishable from "unset".
func (m *Mutation[T, I]) SetID(id I) { m.id = &id }

// SetOldValueLoader installs a function the mutation calls (at most once,
// behind sync.Once) to fetch the entity's pre-mutation state.
func (m *Mutation[T, I]) SetOldValueLoader(fn func(context.Context) (any, error)) {
	m.oldValue = fn
}

// SetDone marks the mutation as completed.
func (m *Mutation[T, I]) SetDone() { m.done = true }

// Fields returns all fields that were changed during this mutation.
func (m *Mutation[T, I]) Fields() []string {
	out := make([]string, 0, len(m.fields))
	for k := range m.fields {
		out = append(out, k)
	}
	return out
}

// Field returns the value of a field with the given name. The second
// return value indicates whether the field was set.
func (m *Mutation[T, I]) Field(name string) (ent.Value, bool) {
	if m.fields == nil {
		return nil, false
	}
	v, ok := m.fields[name]
	return v, ok
}

// SetField sets the value for the given field. Returns an error if the
// field is not in the descriptor or the value type does not match the
// field's expected Go type. Fields typed as the empty interface (e.g.
// schema.Any) accept any value.
func (m *Mutation[T, I]) SetField(name string, value ent.Value) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if value != nil && spec.Type.Kind() != reflect.Interface && reflect.TypeOf(value) != spec.Type {
		return fmt.Errorf("unexpected type %T for field %s (want %s)", value, name, spec.Type)
	}
	if m.fields == nil {
		m.fields = make(map[string]any)
	}
	m.fields[name] = value
	// Clearing a previously-cleared field re-set should drop the cleared marker.
	delete(m.cleared, name)
	// Setting a field should override any prior append for the same field.
	delete(m.appended, name)
	return nil
}

// OldField returns the old value of the field from the database. Returns
// an error if the mutation op is not UpdateOne or the database query fails.
func (m *Mutation[T, I]) OldField(ctx context.Context, name string) (ent.Value, error) {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return nil, fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if !m.op.Is(ent.OpUpdateOne) {
		return nil, fmt.Errorf("OldField is only allowed on UpdateOne operations")
	}
	if m.id == nil || m.oldValue == nil {
		return nil, errors.New("OldField requires an ID + old-value loader on the mutation")
	}
	m.oldOnce.Do(func() {
		if m.done {
			m.oldErr = errors.New("querying old values post mutation is not allowed")
			return
		}
		m.oldCached, m.oldErr = m.oldValue(ctx)
	})
	if m.oldErr != nil {
		return nil, m.oldErr
	}
	rv := reflect.ValueOf(m.oldCached)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	fv := rv.FieldByName(spec.GoName)
	if !fv.IsValid() {
		return nil, fmt.Errorf("entbuilder: descriptor for %s lists field %s but entity has no struct field %s", m.desc.Name, name, spec.GoName)
	}
	return fv.Interface(), nil
}

// ClearedFields returns all nullable fields that were cleared during this
// mutation.
func (m *Mutation[T, I]) ClearedFields() []string {
	out := make([]string, 0, len(m.cleared))
	for k := range m.cleared {
		// Only field clears, not edge clears. Edge clears are tracked separately
		// via map keys; disambiguate by checking the descriptor.
		if _, isField := m.desc.Fields[k]; isField {
			out = append(out, k)
		}
	}
	return out
}

// FieldCleared returns whether a field with the given name was cleared in
// this mutation.
func (m *Mutation[T, I]) FieldCleared(name string) bool {
	if m.cleared == nil {
		return false
	}
	_, ok := m.cleared[name]
	if !ok {
		return false
	}
	_, isField := m.desc.Fields[name]
	return isField
}

// ClearField clears the value of the field with the given name. Returns
// an error if the field is not in the descriptor or is not Nillable.
func (m *Mutation[T, I]) ClearField(name string) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if !spec.Nillable {
		return fmt.Errorf("entbuilder: %s field %s is not nillable", m.desc.Name, name)
	}
	delete(m.fields, name)
	delete(m.added, name)
	delete(m.appended, name)
	if m.cleared == nil {
		m.cleared = make(map[string]struct{})
	}
	m.cleared[name] = struct{}{}
	return nil
}

// ResetField resets all changes in the mutation for the field with the
// given name.
func (m *Mutation[T, I]) ResetField(name string) error {
	_, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	delete(m.fields, name)
	delete(m.cleared, name)
	delete(m.added, name)
	delete(m.appended, name)
	return nil
}

// AddedFields returns all numeric fields that were incremented/decremented
// during this mutation.
func (m *Mutation[T, I]) AddedFields() []string {
	out := make([]string, 0, len(m.added))
	for k := range m.added {
		out = append(out, k)
	}
	return out
}

// AddedField returns the numeric value that was incremented/decremented on
// the field with the given name. The second return value indicates whether
// the field was set.
func (m *Mutation[T, I]) AddedField(name string) (ent.Value, bool) {
	if m.added == nil {
		return nil, false
	}
	v, ok := m.added[name]
	return v, ok
}

// AddField adds the value to the field with the given name. Returns an
// error if the field is not in the descriptor or is not Numeric. The
// value's Go type is intentionally not asserted against the field's main
// type: generated builders pass the field's *signed* delta type (e.g.
// int64 for a uint64 field) so subtraction works, and the downstream
// dialect layer re-asserts the expected delta type. Multiple calls
// accumulate: AddField(10) followed by AddField(-1) records a delta of 9.
func (m *Mutation[T, I]) AddField(name string, value ent.Value) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if !spec.Numeric {
		return fmt.Errorf("entbuilder: %s field %s is not numeric", m.desc.Name, name)
	}
	if m.added == nil {
		m.added = make(map[string]any)
	}
	if prev, exists := m.added[name]; exists {
		if sum, ok := addValues(prev, value); ok {
			m.added[name] = sum
			return nil
		}
	}
	m.added[name] = value
	return nil
}

// addValues attempts to add two numeric values of compatible types via
// reflect. Returns the sum and true on success, or the zero value and
// false if the kinds differ or the kind is non-numeric.
func addValues(a, b any) (any, bool) {
	if a == nil || b == nil {
		return nil, false
	}
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)
	if av.Kind() != bv.Kind() {
		return nil, false
	}
	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(av.Int() + bv.Int()).Convert(av.Type()).Interface(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(av.Uint() + bv.Uint()).Convert(av.Type()).Interface(), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(av.Float() + bv.Float()).Convert(av.Type()).Interface(), true
	}
	return nil, false
}

// AppendedField returns the value that was appended to the field with the
// given name. The second return value indicates whether anything was appended.
func (m *Mutation[T, I]) AppendedField(name string) (ent.Value, bool) {
	if m.appended == nil {
		return nil, false
	}
	v, ok := m.appended[name]
	return v, ok
}

// AppendField records a value to append to the field with the given name.
// Used for JSON list fields. Returns an error if the field is not in the
// descriptor or the value type does not match the field's expected Go type.
func (m *Mutation[T, I]) AppendField(name string, value ent.Value) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if value != nil && reflect.TypeOf(value) != spec.Type {
		return fmt.Errorf("unexpected type %T for field %s (want %s)", value, name, spec.Type)
	}
	if m.appended == nil {
		m.appended = make(map[string]any)
	}
	m.appended[name] = value
	return nil
}

// AddEdgeIDs adds neighbor IDs to the given edge.
func (m *Mutation[T, I]) AddEdgeIDs(edge string, ids ...any) error {
	spec, ok := m.desc.Edges[edge]
	if !ok {
		return fmt.Errorf("unknown %s edge %s", m.desc.Name, edge)
	}
	for _, id := range ids {
		if id != nil && reflect.TypeOf(id) != spec.TargetIDType {
			return fmt.Errorf("unexpected ID type %T for edge %s (want %s)", id, edge, spec.TargetIDType)
		}
	}
	if m.edges == nil {
		m.edges = make(map[string]map[any]struct{})
	}
	if m.edges[edge] == nil {
		m.edges[edge] = make(map[any]struct{})
	}
	for _, id := range ids {
		m.edges[edge][id] = struct{}{}
	}
	return nil
}

// RemoveEdgeIDs marks neighbor IDs as removed from the edge. M2M only.
// Errors on unique edges; callers must use ClearEdge then SetEdgeID instead.
func (m *Mutation[T, I]) RemoveEdgeIDs(edge string, ids ...any) error {
	spec, ok := m.desc.Edges[edge]
	if !ok {
		return fmt.Errorf("unknown %s edge %s", m.desc.Name, edge)
	}
	if spec.Cardinality == O2OUnique {
		return fmt.Errorf("entbuilder: RemoveEdgeIDs not supported on unique edge %s; use ClearEdge", edge)
	}
	for _, id := range ids {
		if id != nil && reflect.TypeOf(id) != spec.TargetIDType {
			return fmt.Errorf("unexpected ID type %T for edge %s (want %s)", id, edge, spec.TargetIDType)
		}
	}
	if m.removedEdges == nil {
		m.removedEdges = make(map[string]map[any]struct{})
	}
	if m.removedEdges[edge] == nil {
		m.removedEdges[edge] = make(map[any]struct{})
	}
	for _, id := range ids {
		if m.edges != nil {
			delete(m.edges[edge], id)
		}
		m.removedEdges[edge][id] = struct{}{}
	}
	return nil
}

// SetEdgeID sets the neighbor ID on a unique edge. Errors on non-unique edges.
func (m *Mutation[T, I]) SetEdgeID(edge string, id any) error {
	spec, ok := m.desc.Edges[edge]
	if !ok {
		return fmt.Errorf("unknown %s edge %s", m.desc.Name, edge)
	}
	if spec.Cardinality != O2OUnique {
		return fmt.Errorf("entbuilder: SetEdgeID requires a unique edge; %s is %v", edge, spec.Cardinality)
	}
	if id != nil && reflect.TypeOf(id) != spec.TargetIDType {
		return fmt.Errorf("unexpected ID type %T for edge %s (want %s)", id, edge, spec.TargetIDType)
	}
	if m.edges == nil {
		m.edges = make(map[string]map[any]struct{})
	}
	m.edges[edge] = map[any]struct{}{id: {}}
	return nil
}

// EdgeID returns the neighbor ID on a unique edge.
func (m *Mutation[T, I]) EdgeID(edge string) (any, bool) {
	if m.edges == nil || m.edges[edge] == nil {
		return nil, false
	}
	for id := range m.edges[edge] {
		return id, true
	}
	return nil, false
}

// EdgeIDs returns all neighbor IDs on the edge.
func (m *Mutation[T, I]) EdgeIDs(edge string) []any {
	if m.edges == nil || m.edges[edge] == nil {
		return nil
	}
	out := make([]any, 0, len(m.edges[edge]))
	for id := range m.edges[edge] {
		out = append(out, id)
	}
	return out
}

// RemovedEdgeIDs returns all neighbor IDs marked as removed from the edge.
func (m *Mutation[T, I]) RemovedEdgeIDs(edge string) []any {
	if m.removedEdges == nil || m.removedEdges[edge] == nil {
		return nil
	}
	out := make([]any, 0, len(m.removedEdges[edge]))
	for id := range m.removedEdges[edge] {
		out = append(out, id)
	}
	return out
}

// ClearEdge marks the edge as cleared.
func (m *Mutation[T, I]) ClearEdge(edge string) error {
	if _, ok := m.desc.Edges[edge]; !ok {
		return fmt.Errorf("unknown %s edge %s", m.desc.Name, edge)
	}
	if m.cleared == nil {
		m.cleared = make(map[string]struct{})
	}
	m.cleared[edge] = struct{}{}
	return nil
}

// EdgeCleared returns whether the edge was cleared in this mutation.
func (m *Mutation[T, I]) EdgeCleared(edge string) bool {
	if m.cleared == nil {
		return false
	}
	_, ok := m.cleared[edge]
	if !ok {
		return false
	}
	_, isEdge := m.desc.Edges[edge]
	return isEdge
}

// ResetEdge resets all changes to the edge.
func (m *Mutation[T, I]) ResetEdge(edge string) error {
	if _, ok := m.desc.Edges[edge]; !ok {
		return fmt.Errorf("unknown %s edge %s", m.desc.Name, edge)
	}
	delete(m.edges, edge)
	delete(m.removedEdges, edge)
	delete(m.cleared, edge)
	return nil
}

// AddedEdges returns the names of edges that had IDs added in this mutation.
func (m *Mutation[T, I]) AddedEdges() []string {
	out := make([]string, 0, len(m.edges))
	for k := range m.edges {
		out = append(out, k)
	}
	return out
}

// AddedIDs returns the IDs added to the given edge.
func (m *Mutation[T, I]) AddedIDs(edge string) []ent.Value {
	if m.edges == nil || m.edges[edge] == nil {
		return nil
	}
	out := make([]ent.Value, 0, len(m.edges[edge]))
	for id := range m.edges[edge] {
		out = append(out, id)
	}
	return out
}

// RemovedEdges returns the names of edges that had IDs removed in this mutation.
func (m *Mutation[T, I]) RemovedEdges() []string {
	out := make([]string, 0, len(m.removedEdges))
	for k := range m.removedEdges {
		out = append(out, k)
	}
	return out
}

// RemovedIDs returns the IDs removed from the given edge.
func (m *Mutation[T, I]) RemovedIDs(edge string) []ent.Value {
	if m.removedEdges == nil || m.removedEdges[edge] == nil {
		return nil
	}
	out := make([]ent.Value, 0, len(m.removedEdges[edge]))
	for id := range m.removedEdges[edge] {
		out = append(out, id)
	}
	return out
}

// ClearedEdges returns the names of edges that were cleared.
func (m *Mutation[T, I]) ClearedEdges() []string {
	out := make([]string, 0, len(m.cleared))
	for k := range m.cleared {
		if _, isEdge := m.desc.Edges[k]; isEdge {
			out = append(out, k)
		}
	}
	return out
}

// ID returns the ID value set on the mutation. The second return indicates
// whether the ID was set. The return type is I — the entity's ID type as
// declared by the per-entity Mutation[T, I] instantiation — so consumer
// hooks that do `id, _ := m.ID()` get the typed value, not an `any`.
func (m *Mutation[T, I]) ID() (I, bool) {
	if m.id == nil {
		var z I
		return z, false
	}
	return *m.id, true
}

// IDs queries the database and returns entity IDs that match the mutation's
// predicates. Only valid on Update/Delete operations. Returns the strongly
// typed ID slice ([]I) to match ID() (I, bool) — callers should not need a
// per-element type assertion to use predicate helpers like X.IDIn(ids...).
func (m *Mutation[T, I]) IDs(ctx context.Context) ([]I, error) {
	switch {
	case m.op.Is(ent.OpUpdateOne | ent.OpDeleteOne):
		if id, ok := m.ID(); ok {
			return []I{id}, nil
		}
		fallthrough
	case m.op.Is(ent.OpUpdate | ent.OpDelete):
		if m.idsFunc == nil {
			return nil, fmt.Errorf("IDs is not allowed on %s operations without an IDsFunc", m.op)
		}
		raw, err := m.idsFunc(ctx, m.predicates...)
		if err != nil {
			return nil, err
		}
		out := make([]I, len(raw))
		for i, v := range raw {
			id, ok := v.(I)
			if !ok {
				var z I
				return nil, fmt.Errorf("IDs: idsFunc returned %T at index %d, expected %T", v, i, z)
			}
			out[i] = id
		}
		return out, nil
	default:
		return nil, fmt.Errorf("IDs is not allowed on %s operations", m.op)
	}
}

// SetIDsFunc installs the function used to query entity IDs.
func (m *Mutation[T, I]) SetIDsFunc(fn func(context.Context, ...func(*sql.Selector)) ([]any, error)) {
	m.idsFunc = fn
}

// WhereP appends predicates to the mutation.
func (m *Mutation[T, I]) WhereP(ps ...func(*sql.Selector)) {
	m.predicates = append(m.predicates, ps...)
}

// AddPredicate is an alias for WhereP usable from entql filters.
func (m *Mutation[T, I]) AddPredicate(p func(*sql.Selector)) {
	m.predicates = append(m.predicates, p)
}

// MutationPredicates returns the predicates registered on the mutation.
func (m *Mutation[T, I]) MutationPredicates() []func(*sql.Selector) {
	return m.predicates
}
