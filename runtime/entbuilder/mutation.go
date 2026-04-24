package entbuilder

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
