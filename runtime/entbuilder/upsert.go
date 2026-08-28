package entbuilder

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
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

// FieldReader is the subset of ent.Mutation the upsert builders need.
type FieldReader interface {
	Field(name string) (ent.Value, bool)
}

// UpsertConfig wires a generated create builder into the generic upsert builders.
type UpsertConfig[ID any] struct {
	Meta          *UpsertMeta
	Conflict      *[]sql.ConflictOption
	Err           func() error
	ChildConflict func() int
	Exec          func(context.Context) error
	SaveID        func(context.Context) (ID, error)
	Mutations     func() []FieldReader
	IDsSet        func() []bool
	Dialect       func() string
}

// UpsertOne is the generic builder for upserting one entity.
type UpsertOne[ID any] struct {
	cfg UpsertConfig[ID]
}

// NewUpsertOne returns a generic single-row upsert builder.
func NewUpsertOne[ID any](cfg UpsertConfig[ID]) *UpsertOne[ID] {
	return &UpsertOne[ID]{cfg: cfg}
}

// UpsertBulk is the generic builder for upserting many entities.
type UpsertBulk[ID any] struct {
	cfg UpsertConfig[ID]
}

// NewUpsertBulk returns a generic bulk upsert builder.
func NewUpsertBulk[ID any](cfg UpsertConfig[ID]) *UpsertBulk[ID] {
	return &UpsertBulk[ID]{cfg: cfg}
}

// updateNewValues appends ResolveWithNewValues plus the ignore-resolver for
// user-defined IDs and immutable fields that were explicitly set on create.
func updateNewValues[ID any](cfg *UpsertConfig[ID]) {
	*cfg.Conflict = append(*cfg.Conflict, sql.ResolveWithNewValues())
	if !cfg.Meta.UserDefinedID && len(cfg.Meta.Immutable) == 0 {
		return
	}
	*cfg.Conflict = append(*cfg.Conflict, sql.ResolveWith(func(s *sql.UpdateSet) {
		var idsSet []bool
		if cfg.IDsSet != nil {
			idsSet = cfg.IDsSet()
		}
		for i, m := range cfg.Mutations() {
			if cfg.Meta.UserDefinedID && i < len(idsSet) && idsSet[i] {
				s.SetIgnore(cfg.Meta.IDColumn)
			}
			for _, col := range cfg.Meta.Immutable {
				if _, exists := m.Field(col); exists {
					s.SetIgnore(col)
				}
			}
		}
	}))
}

func resolveUpdate(conflict *[]sql.ConflictOption, set func(*Upsert)) {
	*conflict = append(*conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&Upsert{UpdateSet: update})
	}))
}

func execUpsert[ID any](ctx context.Context, cfg *UpsertConfig[ID]) error {
	if cfg.Err != nil {
		if err := cfg.Err(); err != nil {
			return err
		}
	}
	if cfg.ChildConflict != nil {
		if i := cfg.ChildConflict(); i >= 0 {
			return fmt.Errorf("%s: OnConflict was set for builder %d. Set it on the %s instead", cfg.Meta.Pkg, i, cfg.Meta.Builder)
		}
	}
	if len(*cfg.Conflict) == 0 {
		return fmt.Errorf("%s: missing options for %s.OnConflict", cfg.Meta.Pkg, cfg.Meta.Builder)
	}
	return cfg.Exec(ctx)
}

// UpdateNewValues updates mutable fields using the values proposed for
// insertion, skipping user-defined IDs and immutable fields set on create.
func (u *UpsertOne[ID]) UpdateNewValues() *UpsertOne[ID] { updateNewValues(&u.cfg); return u }

// Ignore sets each column to itself in case of conflict.
func (u *UpsertOne[ID]) Ignore() *UpsertOne[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING` (SQLite/PostgreSQL only).
func (u *UpsertOne[ID]) DoNothing() *UpsertOne[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.DoNothing())
	return u
}

// Update allows setting the `UPDATE` clause columns via the generic Upsert setter.
func (u *UpsertOne[ID]) Update(set func(*Upsert)) *UpsertOne[ID] {
	resolveUpdate(u.cfg.Conflict, set)
	return u
}

// Set sets the column to v on conflict.
func (u *UpsertOne[ID]) Set(column string, v any) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Set(column, v) })
}

// Add adds v to the column on conflict.
func (u *UpsertOne[ID]) Add(column string, v any) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Add(column, v) })
}

// Clear sets the column to NULL on conflict.
func (u *UpsertOne[ID]) Clear(column string) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Clear(column) })
}

// UpdateFields sets each column to the value proposed for insertion.
func (u *UpsertOne[ID]) UpdateFields(columns ...string) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Update(columns...) })
}

// Exec executes the upsert.
func (u *UpsertOne[ID]) Exec(ctx context.Context) error { return execUpsert(ctx, &u.cfg) }

// ExecX is like Exec, but panics on error.
func (u *UpsertOne[ID]) ExecX(ctx context.Context) {
	if err := u.Exec(ctx); err != nil {
		panic(err)
	}
}

// ID executes the upsert and returns the inserted/updated ID.
func (u *UpsertOne[ID]) ID(ctx context.Context) (id ID, err error) {
	if u.cfg.SaveID == nil {
		return id, fmt.Errorf("%s: %s.OnConflict ID is not supported on entities without a single ID field", u.cfg.Meta.Pkg, u.cfg.Meta.Builder)
	}
	if u.cfg.Meta.UserDefinedID && !u.cfg.Meta.NumericID && u.cfg.Dialect() == dialect.MySQL {
		// MySQL does not support RETURNING, so non-numeric IDs cannot be read back.
		return id, errors.New(u.cfg.Meta.Pkg + ": UpsertOne.ID is not supported by MySQL driver. Use UpsertOne.Exec instead")
	}
	if len(*u.cfg.Conflict) == 0 {
		return id, fmt.Errorf("%s: missing options for %s.OnConflict", u.cfg.Meta.Pkg, u.cfg.Meta.Builder)
	}
	return u.cfg.SaveID(ctx)
}

// IDX is like ID, but panics on error.
func (u *UpsertOne[ID]) IDX(ctx context.Context) ID {
	id, err := u.ID(ctx)
	if err != nil {
		panic(err)
	}
	return id
}

// UpdateNewValues updates mutable fields using the values proposed for
// insertion, skipping user-defined IDs and immutable fields set on create.
func (u *UpsertBulk[ID]) UpdateNewValues() *UpsertBulk[ID] { updateNewValues(&u.cfg); return u }

// Ignore sets each column to itself in case of conflict.
func (u *UpsertBulk[ID]) Ignore() *UpsertBulk[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING` (SQLite/PostgreSQL only).
func (u *UpsertBulk[ID]) DoNothing() *UpsertBulk[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.DoNothing())
	return u
}

// Update allows setting the `UPDATE` clause columns via the generic Upsert setter.
func (u *UpsertBulk[ID]) Update(set func(*Upsert)) *UpsertBulk[ID] {
	resolveUpdate(u.cfg.Conflict, set)
	return u
}

// Set sets the column to v on conflict.
func (u *UpsertBulk[ID]) Set(column string, v any) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Set(column, v) })
}

// Add adds v to the column on conflict.
func (u *UpsertBulk[ID]) Add(column string, v any) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Add(column, v) })
}

// Clear sets the column to NULL on conflict.
func (u *UpsertBulk[ID]) Clear(column string) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Clear(column) })
}

// UpdateFields sets each column to the value proposed for insertion.
func (u *UpsertBulk[ID]) UpdateFields(columns ...string) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Update(columns...) })
}

// Exec executes the upsert.
func (u *UpsertBulk[ID]) Exec(ctx context.Context) error { return execUpsert(ctx, &u.cfg) }

// ExecX is like Exec, but panics on error.
func (u *UpsertBulk[ID]) ExecX(ctx context.Context) {
	if err := u.Exec(ctx); err != nil {
		panic(err)
	}
}
