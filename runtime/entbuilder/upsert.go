package entbuilder

import (
	"entgo.io/ent/dialect/sql"
)

// UpsertMeta is the per-entity metadata codegen emits once per entity to
// drive the generic upsert builders.
type UpsertMeta struct {
	// Pkg and Builder are used in error messages, e.g. "gen", "EscrowCreate".
	Pkg, Builder string
	// IDColumn is the entity's ID column ("" when the entity has no single ID field).
	IDColumn string
	// UserDefinedID reports whether the ID is set by the user (schema-defined default/no autoincrement).
	UserDefinedID bool
	// NumericID reports whether the ID type is numeric (MySQL can return those from upserts).
	NumericID bool
	// Immutable lists immutable field columns skipped by UpdateNewValues when set on create.
	Immutable []string
}

// Upsert is the generic "ON CONFLICT" setter passed to Update callbacks.
// Columns are addressed by their field constants (e.g. escrow.FieldName).
// ponytail: no column/op validation — unknown columns and invalid ops
// (Clear on NOT NULL, Add on non-numeric) surface as database errors.
type Upsert struct {
	*sql.UpdateSet
}

// Set sets the column to the given value.
func (u *Upsert) Set(column string, v any) *Upsert {
	u.UpdateSet.Set(column, v)
	return u
}

// Add adds v to the column's current value.
func (u *Upsert) Add(column string, v any) *Upsert {
	u.UpdateSet.Add(column, v)
	return u
}

// Clear sets the column to NULL.
func (u *Upsert) Clear(column string) *Upsert {
	u.UpdateSet.SetNull(column)
	return u
}

// Update sets each column to the value proposed for insertion ("excluded").
func (u *Upsert) Update(columns ...string) *Upsert {
	for _, c := range columns {
		u.UpdateSet.SetExcluded(c)
	}
	return u
}
