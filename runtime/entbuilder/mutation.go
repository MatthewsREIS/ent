package entbuilder

import (
	"context"
	"fmt"
)

// Op is a mutation operation kind. Mirrors ent.Op bit layout at the
// integer level so that callers can convert back and forth without an
// explicit map lookup; the enum is redeclared here to avoid an import
// cycle between entbuilder and the ent root package.
type Op uint

const (
	OpCreate    Op = 1 << iota // 1
	OpUpdate                   // 2
	OpUpdateOne                // 4
	OpDelete                   // 8
	OpDeleteOne                // 16
)

// Mutation is the generic, descriptor-driven state container shared by all
// per-schema mutation façades. Per-schema façades (e.g. *CardMutation)
// wrap a *Mutation[T] and expose typed accessors that delegate to the
// name-based helpers here.
type Mutation[T any] struct {
	schema *Schema[T]
	op     Op

	// setFields maps Field.Name -> the current-mutation value for that field.
	// Values are stored as `any` so heterogeneous types can share one map;
	// typed accessors on the façade perform the assertion on read.
	setFields map[string]any

	// clearedFields is the set of Field.Name values that have been cleared
	// in this mutation (as opposed to merely set-to-zero).
	clearedFields map[string]struct{}

	// addedFields maps Field.Name -> the numeric delta recorded by AddX on
	// numeric fields.
	addedFields map[string]any

	// edgeIDs maps Edge.Name -> target ID for unique-to-one edges.
	edgeIDs map[string]any
	// clearedEdges is the set of Edge.Name entries explicitly cleared.
	clearedEdges map[string]struct{}

	// id is the target ID for UpdateOne/DeleteOne mutations; nil otherwise.
	id any
}

// NewMutation constructs an empty mutation for the given schema and op.
func NewMutation[T any](schema *Schema[T], op Op) *Mutation[T] {
	return &Mutation[T]{
		schema:        schema,
		op:            op,
		setFields:     make(map[string]any),
		clearedFields: make(map[string]struct{}),
		addedFields:   make(map[string]any),
		edgeIDs:       make(map[string]any),
		clearedEdges:  make(map[string]struct{}),
	}
}

// Op returns the mutation's op kind.
func (m *Mutation[T]) Op() Op { return m.op }

// Fields returns the names of fields that have been explicitly set (not cleared).
// Order is not guaranteed.
func (m *Mutation[T]) Fields() []string {
	out := make([]string, 0, len(m.setFields))
	for k := range m.setFields {
		out = append(out, k)
	}
	return out
}

// AddedFields returns the names of numeric fields that have recorded deltas.
func (m *Mutation[T]) AddedFields() []string {
	out := make([]string, 0, len(m.addedFields))
	for k := range m.addedFields {
		out = append(out, k)
	}
	return out
}

// ClearedFields returns the names of fields that have been explicitly cleared.
func (m *Mutation[T]) ClearedFields() []string {
	out := make([]string, 0, len(m.clearedFields))
	for k := range m.clearedFields {
		out = append(out, k)
	}
	return out
}

// SetField records v as the mutation's value for the field named `name`.
// Returns an error if `name` is not declared on the schema. Does not
// validate v's type against the field's descriptor — the façade's typed
// setters are responsible for that.
func (m *Mutation[T]) SetField(name string, v any) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	m.setFields[name] = v
	// Setting a field after clearing it un-clears it.
	delete(m.clearedFields, name)
	return nil
}

// Field returns the mutation's recorded value for `name` and ok=true, or
// (nil, false) if the field was not set (or was cleared). Unknown field
// names return (nil, false) without error to match the existing
// ent.Mutation.Field contract.
func (m *Mutation[T]) Field(name string) (any, bool) {
	v, ok := m.setFields[name]
	return v, ok
}

// ClearField marks `name` as cleared and removes any previously-set value.
// Returns an error if `name` is unknown or is a non-nullable field (per the
// schema descriptor).
func (m *Mutation[T]) ClearField(name string) error {
	f, ok := m.schema.FindField(name)
	if !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	if !f.Nullable {
		return fmt.Errorf("entbuilder: schema %q field %q is not Optional and cannot be cleared",
			m.schema.Name, name)
	}
	delete(m.setFields, name)
	m.clearedFields[name] = struct{}{}
	return nil
}

// FieldCleared reports whether `name` was cleared by this mutation.
// Returns false for unknown fields (matching the existing ent contract).
func (m *Mutation[T]) FieldCleared(name string) bool {
	_, ok := m.clearedFields[name]
	return ok
}

// AddField records a numeric delta for `name`. The caller is responsible
// for ensuring the field is a numeric type — this matches the existing
// ent.Mutation.AddField contract.
func (m *Mutation[T]) AddField(name string, v any) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	m.addedFields[name] = v
	return nil
}

// AddedField returns the recorded delta for `name` (from AddField) and ok=true,
// or (nil, false) if no delta has been recorded.
func (m *Mutation[T]) AddedField(name string) (any, bool) {
	v, ok := m.addedFields[name]
	return v, ok
}

// ResetField clears all mutation state for `name`: set value, cleared flag,
// and added delta. Matches the existing generated ResetX method behavior.
func (m *Mutation[T]) ResetField(name string) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	delete(m.setFields, name)
	delete(m.clearedFields, name)
	delete(m.addedFields, name)
	return nil
}

// SetID sets the mutation's target ID. Used by UpdateOne / DeleteOne paths.
func (m *Mutation[T]) SetID(id any) { m.id = id }

// ID returns the target ID and ok=true if SetID was called.
func (m *Mutation[T]) ID() (any, bool) {
	if m.id == nil {
		return nil, false
	}
	return m.id, true
}

// SetEdgeID stores the target ID for a unique edge. Returns an error if
// the edge is unknown or not unique.
func (m *Mutation[T]) SetEdgeID(name string, id any) error {
	e, ok := m.schema.FindEdge(name)
	if !ok {
		return fmt.Errorf("entbuilder: schema %q has no edge %q", m.schema.Name, name)
	}
	if !e.Unique {
		return fmt.Errorf("entbuilder: edge %q on schema %q is not unique; use AddEdgeIDs instead",
			name, m.schema.Name)
	}
	m.edgeIDs[name] = id
	delete(m.clearedEdges, name)
	return nil
}

// EdgeID returns the stored target ID for a unique edge. ok=false if unset.
func (m *Mutation[T]) EdgeID(name string) (any, bool) {
	v, ok := m.edgeIDs[name]
	return v, ok
}

// ClearEdge marks a unique edge as cleared and removes any stored target ID.
func (m *Mutation[T]) ClearEdge(name string) error {
	if _, ok := m.schema.FindEdge(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no edge %q", m.schema.Name, name)
	}
	delete(m.edgeIDs, name)
	m.clearedEdges[name] = struct{}{}
	return nil
}

// EdgeCleared reports whether an edge was cleared by this mutation.
func (m *Mutation[T]) EdgeCleared(name string) bool {
	_, ok := m.clearedEdges[name]
	return ok
}

// AddedEdges returns the names of edges that have a target ID set by this mutation.
// For the spike's unique-edges-only scope, this is the set of edges with a SetEdgeID call.
func (m *Mutation[T]) AddedEdges() []string {
	out := make([]string, 0, len(m.edgeIDs))
	for k := range m.edgeIDs {
		out = append(out, k)
	}
	return out
}

// ClearedEdges returns the names of edges explicitly cleared.
func (m *Mutation[T]) ClearedEdges() []string {
	out := make([]string, 0, len(m.clearedEdges))
	for k := range m.clearedEdges {
		out = append(out, k)
	}
	return out
}

// RemovedEdges returns the names of edges that have had specific targets
// removed (for non-unique edges). In the spike's unique-edges-only scope,
// this is always empty; full through-table support is Phase 4C scope.
func (m *Mutation[T]) RemovedEdges() []string { return nil }

// OldField retrieves the pre-mutation value of `name` by delegating to the
// schema's OldFieldFetcher closure. Returns an error if:
//   - `name` is not a declared field on the schema
//   - the schema has no OldFieldFetcher wired (e.g., Create-only schemas)
//   - this mutation has no SetID yet (bulk updates cannot resolve a single row)
func (m *Mutation[T]) OldField(ctx context.Context, name string) (any, error) {
	if _, ok := m.schema.FindField(name); !ok {
		return nil, fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	if m.schema.OldFieldFetcher == nil {
		return nil, fmt.Errorf("entbuilder: schema %q has no OldFieldFetcher configured", m.schema.Name)
	}
	id, ok := m.ID()
	if !ok {
		return nil, fmt.Errorf("entbuilder: OldField(%q) requires a target ID (SetID not called)", name)
	}
	return m.schema.OldFieldFetcher(ctx, id, name)
}
