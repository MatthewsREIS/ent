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
func (m *Mutation[T]) Op() ent.Op { return m.op }

// SetOp allows setting the mutation operation.
func (m *Mutation[T]) SetOp(op ent.Op) { m.op = op }

// Type returns the schema type for this mutation.
func (m *Mutation[T]) Type() string { return m.typ }

// SetID stores the entity ID on the mutation. Called by per-entity option
// constructors (WithXID) and by the CUD builders after a successful create.
func (m *Mutation[T]) SetID(id any) { m.id = id }

// SetOldValueLoader installs a function the mutation calls (at most once,
// behind sync.Once) to fetch the entity's pre-mutation state.
func (m *Mutation[T]) SetOldValueLoader(fn func(context.Context) (any, error)) {
	m.oldValue = fn
}

// SetDone marks the mutation as completed.
func (m *Mutation[T]) SetDone() { m.done = true }

// Fields returns all fields that were changed during this mutation.
func (m *Mutation[T]) Fields() []string {
	out := make([]string, 0, len(m.fields))
	for k := range m.fields {
		out = append(out, k)
	}
	return out
}

// Field returns the value of a field with the given name. The second
// return value indicates whether the field was set.
func (m *Mutation[T]) Field(name string) (ent.Value, bool) {
	if m.fields == nil {
		return nil, false
	}
	v, ok := m.fields[name]
	return v, ok
}

// SetField sets the value for the given field. Returns an error if the
// field is not in the descriptor or the value type does not match the
// field's expected Go type.
func (m *Mutation[T]) SetField(name string, value ent.Value) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if value != nil && reflect.TypeOf(value) != spec.Type {
		return fmt.Errorf("unexpected type %T for field %s (want %s)", value, name, spec.Type)
	}
	if m.fields == nil {
		m.fields = make(map[string]any)
	}
	m.fields[name] = value
	// Clearing a previously-cleared field re-set should drop the cleared marker.
	delete(m.cleared, name)
	return nil
}

// OldField returns the old value of the field from the database. Returns
// an error if the mutation op is not UpdateOne or the database query fails.
func (m *Mutation[T]) OldField(ctx context.Context, name string) (ent.Value, error) {
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
func (m *Mutation[T]) ClearedFields() []string {
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
func (m *Mutation[T]) FieldCleared(name string) bool {
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
func (m *Mutation[T]) ClearField(name string) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if !spec.Nillable {
		return fmt.Errorf("entbuilder: %s field %s is not nillable", m.desc.Name, name)
	}
	delete(m.fields, name)
	if m.cleared == nil {
		m.cleared = make(map[string]struct{})
	}
	m.cleared[name] = struct{}{}
	return nil
}

// ResetField resets all changes in the mutation for the field with the
// given name.
func (m *Mutation[T]) ResetField(name string) error {
	_, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	delete(m.fields, name)
	delete(m.cleared, name)
	delete(m.added, name)
	return nil
}

// AddedFields returns all numeric fields that were incremented/decremented
// during this mutation.
func (m *Mutation[T]) AddedFields() []string {
	out := make([]string, 0, len(m.added))
	for k := range m.added {
		out = append(out, k)
	}
	return out
}

// AddedField returns the numeric value that was incremented/decremented on
// the field with the given name. The second return value indicates whether
// the field was set.
func (m *Mutation[T]) AddedField(name string) (ent.Value, bool) {
	if m.added == nil {
		return nil, false
	}
	v, ok := m.added[name]
	return v, ok
}

// AddField adds the value to the field with the given name. Returns an
// error if the field is not in the descriptor or is not Numeric.
func (m *Mutation[T]) AddField(name string, value ent.Value) error {
	spec, ok := m.desc.Fields[name]
	if !ok {
		return fmt.Errorf("unknown %s field %s", m.desc.Name, name)
	}
	if !spec.Numeric {
		return fmt.Errorf("entbuilder: %s field %s is not numeric", m.desc.Name, name)
	}
	if value != nil && reflect.TypeOf(value) != spec.Type {
		return fmt.Errorf("unexpected type %T for field %s (want %s)", value, name, spec.Type)
	}
	if m.added == nil {
		m.added = make(map[string]any)
	}
	m.added[name] = value
	return nil
}

// AddEdgeIDs adds neighbor IDs to the given edge.
func (m *Mutation[T]) AddEdgeIDs(edge string, ids ...any) error {
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
func (m *Mutation[T]) RemoveEdgeIDs(edge string, ids ...any) error {
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
func (m *Mutation[T]) SetEdgeID(edge string, id any) error {
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
func (m *Mutation[T]) EdgeID(edge string) (any, bool) {
	if m.edges == nil || m.edges[edge] == nil {
		return nil, false
	}
	for id := range m.edges[edge] {
		return id, true
	}
	return nil, false
}

// EdgeIDs returns all neighbor IDs on the edge.
func (m *Mutation[T]) EdgeIDs(edge string) []any {
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
func (m *Mutation[T]) RemovedEdgeIDs(edge string) []any {
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
func (m *Mutation[T]) ClearEdge(edge string) error {
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
func (m *Mutation[T]) EdgeCleared(edge string) bool {
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
func (m *Mutation[T]) ResetEdge(edge string) error {
	if _, ok := m.desc.Edges[edge]; !ok {
		return fmt.Errorf("unknown %s edge %s", m.desc.Name, edge)
	}
	delete(m.edges, edge)
	delete(m.removedEdges, edge)
	delete(m.cleared, edge)
	return nil
}

// AddedEdges returns the names of edges that had IDs added in this mutation.
func (m *Mutation[T]) AddedEdges() []string {
	out := make([]string, 0, len(m.edges))
	for k := range m.edges {
		out = append(out, k)
	}
	return out
}

// AddedIDs returns the IDs added to the given edge.
func (m *Mutation[T]) AddedIDs(edge string) []ent.Value {
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
func (m *Mutation[T]) RemovedEdges() []string {
	out := make([]string, 0, len(m.removedEdges))
	for k := range m.removedEdges {
		out = append(out, k)
	}
	return out
}

// RemovedIDs returns the IDs removed from the given edge.
func (m *Mutation[T]) RemovedIDs(edge string) []ent.Value {
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
func (m *Mutation[T]) ClearedEdges() []string {
	out := make([]string, 0, len(m.cleared))
	for k := range m.cleared {
		if _, isEdge := m.desc.Edges[k]; isEdge {
			out = append(out, k)
		}
	}
	return out
}

// ID returns the ID value set on the mutation. The second return indicates
// whether the ID was set.
func (m *Mutation[T]) ID() (any, bool) {
	if m.id == nil {
		return nil, false
	}
	return m.id, true
}

// IDs queries the database and returns entity IDs that match the mutation's
// predicates. Only valid on Update/Delete operations.
func (m *Mutation[T]) IDs(ctx context.Context) ([]any, error) {
	switch {
	case m.op.Is(ent.OpUpdateOne | ent.OpDeleteOne):
		if id, ok := m.ID(); ok {
			return []any{id}, nil
		}
		fallthrough
	case m.op.Is(ent.OpUpdate | ent.OpDelete):
		if m.idsFunc == nil {
			return nil, fmt.Errorf("IDs is not allowed on %s operations without an IDsFunc", m.op)
		}
		return m.idsFunc(ctx, m.predicates...)
	default:
		return nil, fmt.Errorf("IDs is not allowed on %s operations", m.op)
	}
}

// SetIDsFunc installs the function used to query entity IDs.
func (m *Mutation[T]) SetIDsFunc(fn func(context.Context, ...func(*sql.Selector)) ([]any, error)) {
	m.idsFunc = fn
}

// WhereP appends predicates to the mutation.
func (m *Mutation[T]) WhereP(ps ...func(*sql.Selector)) {
	m.predicates = append(m.predicates, ps...)
}

// AddPredicate is an alias for WhereP usable from entql filters.
func (m *Mutation[T]) AddPredicate(p func(*sql.Selector)) {
	m.predicates = append(m.predicates, p)
}

// MutationPredicates returns the predicates registered on the mutation.
func (m *Mutation[T]) MutationPredicates() []func(*sql.Selector) {
	return m.predicates
}
