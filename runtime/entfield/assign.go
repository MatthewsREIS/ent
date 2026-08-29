package entfield

import "entgo.io/ent"

// Mutable is the subset of entbuilder.Mutation[T, I]'s method set that field
// and edge handles need to record assignments. Matched exactly against
// runtime/entbuilder/mutation_methods.go so *entbuilder.Mutation[T, I]
// satisfies it without an adapter.
type Mutable interface {
	SetField(name string, value ent.Value) error
	AddField(name string, value ent.Value) error
	AppendField(name string, value ent.Value) error
	ClearField(name string) error
	ResetField(name string) error
	SetEdgeID(edge string, id any) error
	AddEdgeIDs(edge string, ids ...any) error
	RemoveEdgeIDs(edge string, ids ...any) error
	ClearEdge(edge string) error
}

// Assignment is a single deferred mutation-builder call.
type Assignment = func(m Mutable) error

// Apply runs assignments in order against m, returning the first error.
func Apply(m Mutable, as ...Assignment) error {
	for _, a := range as {
		if err := a(m); err != nil {
			return err
		}
	}
	return nil
}

// toAny boxes a typed slice into []any, for the ID-boxed edge mutation calls
// (AddEdgeIDs/RemoveEdgeIDs take ...any).
func toAny[T any](vs []T) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}

// JSON is a setter-only handle for JSON fields (no predicates — matches 2a,
// which never generated where-clauses for JSON columns).
type JSON[T any] struct{ col string }

// NewJSON creates a new JSON handle for the given column.
func NewJSON[T any](col string) JSON[T] { return JSON[T]{col: col} }

// Set assigns v to the field.
func (f JSON[T]) Set(v T) Assignment {
	return func(m Mutable) error { return m.SetField(f.col, v) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f JSON[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Append records v to append to the field (JSON list fields).
func (f JSON[T]) Append(v T) Assignment {
	return func(m Mutable) error { return m.AppendField(f.col, v) }
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f JSON[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.col) }
}

// EdgeField wraps a Value handle for a field that also backs an edge (e.g. a
// foreign-key column). Predicates/order stay column-based via the embedded
// Value; assignments route through the edge itself, matching the old
// setter.tmpl's EdgeField semantics.
type EdgeField[T any] struct {
	// Value is embedded for its predicate/order methods (column-based) only.
	// Do not call Value.Set/Value.SetNillable/Value.Clear directly on an
	// EdgeField — that bypasses edge routing entirely (writes m.fields under
	// the column name via SetField instead of going through
	// SetEdgeID/ClearEdge) and, for a StorageKey-diverging edge-field, uses
	// the wrong mutation key. Always use EdgeField's own Set/SetNillable/
	// Clear below, which shadow Value's and route through the edge.
	Value[T]
	edge string
}

// NewEdgeField creates a new EdgeField handle for the given column and edge.
// The embedded Value's "name" is set to col too — it's only reachable
// through Value's own (predicate) methods, which need the DB column; every
// EdgeField assignment method below is defined directly on EdgeField
// (shadowing Value's) and routes through edge, not through Value at all.
func NewEdgeField[T any](col, edge string) EdgeField[T] {
	return EdgeField[T]{Value: NewValue[T](col, col), edge: edge}
}

// Set assigns v via the edge (SetEdgeID), not the column.
func (f EdgeField[T]) Set(v T) Assignment {
	return func(m Mutable) error { return m.SetEdgeID(f.edge, v) }
}

// SetNillable assigns *v via the edge when non-nil; no-op otherwise.
func (f EdgeField[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the edge (not just the column).
func (f EdgeField[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearEdge(f.edge) }
}
