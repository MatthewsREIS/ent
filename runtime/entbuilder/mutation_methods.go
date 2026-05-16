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
