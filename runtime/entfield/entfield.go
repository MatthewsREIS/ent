package entfield

import (
	"database/sql/driver"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
)

// P is a predicate function that applies a condition to a SQL selector.
type P = func(*sql.Selector)

// Order is an ordering function that applies ordering to a SQL selector.
type Order = func(*sql.Selector)

// Numeric is a constraint that represents all numeric types.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// String is a generic handle for string-based fields.
//
// col and name are deliberately kept as two separate strings, not one: col
// is the DB column (used by every predicate/order method below, which build
// SQL against *sql.Selector), while name is the field's schema-declared
// name (used by every assignment method, which calls into
// entbuilder.Mutation — keyed by that logical name, not the column). They
// coincide for the overwhelmingly common case (no field.StorageKey
// override), which is why this split went unnoticed until a field with a
// customized storage key was exercised end to end.
type String[T ~string] struct {
	col, name string
	scan      func(T) (driver.Value, error)
}

// NewString creates a new String handle for the given column/field name.
func NewString[T ~string](col, name string) String[T] {
	return String[T]{col: col, name: name}
}

// NewStringScan creates a new String handle with a custom scanner function.
func NewStringScan[T ~string](col, name string, scan func(T) (driver.Value, error)) String[T] {
	return String[T]{col: col, name: name, scan: scan}
}

// Column returns the column name.
func (f String[T]) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f String[T]) EQ(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.EQ(s.C(f.col), scanned))
		}
	}
	return sql.FieldEQ(f.col, string(v))
}

// NEQ returns a predicate for inequality.
func (f String[T]) NEQ(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.NEQ(s.C(f.col), scanned))
		}
	}
	return sql.FieldNEQ(f.col, string(v))
}

// In returns a predicate for membership.
func (f String[T]) In(vs ...T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned := make([]any, len(vs))
			for i, v := range vs {
				val, err := f.scan(v)
				if err != nil {
					s.AddError(err)
					return
				}
				scanned[i] = val
			}
			s.Where(sql.In(s.C(f.col), scanned...))
		}
	}
	v := make([]any, len(vs))
	for i, val := range vs {
		v[i] = string(val)
	}
	return sql.FieldIn(f.col, v...)
}

// NotIn returns a predicate for non-membership.
func (f String[T]) NotIn(vs ...T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned := make([]any, len(vs))
			for i, v := range vs {
				val, err := f.scan(v)
				if err != nil {
					s.AddError(err)
					return
				}
				scanned[i] = val
			}
			s.Where(sql.NotIn(s.C(f.col), scanned...))
		}
	}
	v := make([]any, len(vs))
	for i, val := range vs {
		v[i] = string(val)
	}
	return sql.FieldNotIn(f.col, v...)
}

// GT returns a predicate for greater than.
func (f String[T]) GT(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.GT(s.C(f.col), scanned))
		}
	}
	return sql.FieldGT(f.col, string(v))
}

// GTE returns a predicate for greater than or equal.
func (f String[T]) GTE(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.GTE(s.C(f.col), scanned))
		}
	}
	return sql.FieldGTE(f.col, string(v))
}

// LT returns a predicate for less than.
func (f String[T]) LT(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.LT(s.C(f.col), scanned))
		}
	}
	return sql.FieldLT(f.col, string(v))
}

// LTE returns a predicate for less than or equal.
func (f String[T]) LTE(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.LTE(s.C(f.col), scanned))
		}
	}
	return sql.FieldLTE(f.col, string(v))
}

// scanStr resolves v to the string that should actually reach the SQL
// substring/prefix/suffix ops: the scanned (driver-encoded) value when this
// handle has a scan func (matching NewStringScan's other ops, which all
// scan first), or the plain string conversion otherwise. Substring/prefix
// ops are meaningless against the raw Go value once a scan func has
// transformed how it's stored (see the "custom" ValueScanner field in
// entc/integration/ent/exvaluescan for a case where this matters).
func (f String[T]) scanStr(v T) (string, error) {
	if f.scan == nil {
		return string(v), nil
	}
	scanned, err := f.scan(v)
	if err != nil {
		return "", err
	}
	s, ok := scanned.(string)
	if !ok {
		return "", fmt.Errorf("entfield: scanned value is not a string: %T", scanned)
	}
	return s, nil
}

// ponytail: full string op set on all String handles
// Contains returns a predicate for substring containment.
func (f String[T]) Contains(v T) P {
	return f.scannedOp(v, sql.FieldContains)
}

// HasPrefix returns a predicate for prefix matching.
func (f String[T]) HasPrefix(v T) P {
	return f.scannedOp(v, sql.FieldHasPrefix)
}

// HasSuffix returns a predicate for suffix matching.
func (f String[T]) HasSuffix(v T) P {
	return f.scannedOp(v, sql.FieldHasSuffix)
}

// EqualFold returns a predicate for case-insensitive equality.
func (f String[T]) EqualFold(v T) P {
	return f.scannedOp(v, sql.FieldEqualFold)
}

// ContainsFold returns a predicate for case-insensitive containment.
func (f String[T]) ContainsFold(v T) P {
	return f.scannedOp(v, sql.FieldContainsFold)
}

// scannedOp applies a sql.Field* string-comparison op to v, through
// scanStr's scan-or-convert resolution.
func (f String[T]) scannedOp(v T, op func(string, string) P) P {
	return func(s *sql.Selector) {
		str, err := f.scanStr(v)
		if err != nil {
			s.AddError(err)
			return
		}
		op(f.col, str)(s)
	}
}

// Order returns an ordering by this field.
func (f String[T]) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f String[T]) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f String[T]) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
// ponytail: identical across all handle types (String, Number, Bool, Time, Enum, Value, Bytes)
func (f String[T]) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
// ponytail: identical across all handle types (String, Number, Bool, Time, Enum, Value, Bytes)
func (f String[T]) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field, passing v as-is (T, not string(v)): the
// mutation's field descriptor expects the field's declared Go type (e.g.
// a `type Dir string` GoType), and boxing v straight into the ent.Value
// interface preserves that dynamic type — converting to a bare string here
// would make it mismatch the descriptor (entbuilder.Mutation.SetField
// type-checks reflect.TypeOf(value) against the descriptor) and panic
// GetField's later v.(V) assertion. Not the scan-func-encoded form either:
// scanning happens later at spec-build time. Routed by name (the schema
// field name), not col (the DB column) — see the type doc comment.
func (f String[T]) Set(v T) Assignment {
	return func(m Mutable) error { return m.SetField(f.name, v) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f String[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f String[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}

// Number is a generic handle for numeric fields. See String's doc comment
// for why col and name are kept separate.
type Number[T Numeric] struct {
	col, name string
}

// NewNumber creates a new Number handle for the given column/field name.
func NewNumber[T Numeric](col, name string) Number[T] {
	return Number[T]{col: col, name: name}
}

// Column returns the column name.
func (f Number[T]) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f Number[T]) EQ(v T) P {
	return sql.FieldEQ(f.col, v)
}

// NEQ returns a predicate for inequality.
func (f Number[T]) NEQ(v T) P {
	return sql.FieldNEQ(f.col, v)
}

// In returns a predicate for membership.
func (f Number[T]) In(vs ...T) P {
	return sql.FieldIn(f.col, vs...)
}

// NotIn returns a predicate for non-membership.
func (f Number[T]) NotIn(vs ...T) P {
	return sql.FieldNotIn(f.col, vs...)
}

// GT returns a predicate for greater than.
func (f Number[T]) GT(v T) P {
	return sql.FieldGT(f.col, v)
}

// GTE returns a predicate for greater than or equal.
func (f Number[T]) GTE(v T) P {
	return sql.FieldGTE(f.col, v)
}

// LT returns a predicate for less than.
func (f Number[T]) LT(v T) P {
	return sql.FieldLT(f.col, v)
}

// LTE returns a predicate for less than or equal.
func (f Number[T]) LTE(v T) P {
	return sql.FieldLTE(f.col, v)
}

// Order returns an ordering by this field.
func (f Number[T]) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f Number[T]) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f Number[T]) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
func (f Number[T]) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
func (f Number[T]) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field. Resets any prior AddField delta first (ignoring
// its error — the field may be unset), so Set overrides Add on an update
// builder; harmless no-op on create.
func (f Number[T]) Set(v T) Assignment {
	return func(m Mutable) error {
		_ = m.ResetField(f.name)
		return m.SetField(f.name, v)
	}
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f Number[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Add records a delta to add to the field.
func (f Number[T]) Add(v T) Assignment {
	return func(m Mutable) error { return m.AddField(f.name, v) }
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f Number[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}

// Bool is a generic handle for boolean fields. See String's doc comment for
// why col and name are kept separate.
type Bool[T ~bool] struct {
	col, name string
}

// NewBool creates a new Bool handle for the given column/field name.
func NewBool[T ~bool](col, name string) Bool[T] {
	return Bool[T]{col: col, name: name}
}

// Column returns the column name.
func (f Bool[T]) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f Bool[T]) EQ(v T) P {
	return sql.FieldEQ(f.col, bool(v))
}

// NEQ returns a predicate for inequality.
func (f Bool[T]) NEQ(v T) P {
	return sql.FieldNEQ(f.col, bool(v))
}

// Order returns an ordering by this field.
func (f Bool[T]) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f Bool[T]) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f Bool[T]) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
func (f Bool[T]) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
func (f Bool[T]) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field.
func (f Bool[T]) Set(v T) Assignment {
	return func(m Mutable) error { return m.SetField(f.name, bool(v)) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f Bool[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f Bool[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}

// Time is a handle for time.Time fields. See String's doc comment for why
// col and name are kept separate.
type Time struct {
	col, name string
}

// NewTime creates a new Time handle for the given column/field name.
func NewTime(col, name string) Time {
	return Time{col: col, name: name}
}

// Column returns the column name.
func (f Time) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f Time) EQ(v time.Time) P {
	return sql.FieldEQ(f.col, v)
}

// NEQ returns a predicate for inequality.
func (f Time) NEQ(v time.Time) P {
	return sql.FieldNEQ(f.col, v)
}

// In returns a predicate for membership.
func (f Time) In(vs ...time.Time) P {
	return sql.FieldIn(f.col, vs...)
}

// NotIn returns a predicate for non-membership.
func (f Time) NotIn(vs ...time.Time) P {
	return sql.FieldNotIn(f.col, vs...)
}

// GT returns a predicate for greater than.
func (f Time) GT(v time.Time) P {
	return sql.FieldGT(f.col, v)
}

// GTE returns a predicate for greater than or equal.
func (f Time) GTE(v time.Time) P {
	return sql.FieldGTE(f.col, v)
}

// LT returns a predicate for less than.
func (f Time) LT(v time.Time) P {
	return sql.FieldLT(f.col, v)
}

// LTE returns a predicate for less than or equal.
func (f Time) LTE(v time.Time) P {
	return sql.FieldLTE(f.col, v)
}

// Order returns an ordering by this field.
func (f Time) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f Time) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f Time) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
func (f Time) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
func (f Time) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field.
func (f Time) Set(v time.Time) Assignment {
	return func(m Mutable) error { return m.SetField(f.name, v) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f Time) SetNillable(v *time.Time) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f Time) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}

// Enum is a generic handle for enum fields. See String's doc comment for
// why col and name are kept separate.
type Enum[T ~string] struct {
	col, name string
}

// NewEnum creates a new Enum handle for the given column/field name.
func NewEnum[T ~string](col, name string) Enum[T] {
	return Enum[T]{col: col, name: name}
}

// Column returns the column name.
func (f Enum[T]) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f Enum[T]) EQ(v T) P {
	return sql.FieldEQ(f.col, string(v))
}

// NEQ returns a predicate for inequality.
func (f Enum[T]) NEQ(v T) P {
	return sql.FieldNEQ(f.col, string(v))
}

// In returns a predicate for membership.
func (f Enum[T]) In(vs ...T) P {
	v := make([]any, len(vs))
	for i, val := range vs {
		v[i] = string(val)
	}
	return sql.FieldIn(f.col, v...)
}

// NotIn returns a predicate for non-membership.
func (f Enum[T]) NotIn(vs ...T) P {
	v := make([]any, len(vs))
	for i, val := range vs {
		v[i] = string(val)
	}
	return sql.FieldNotIn(f.col, v...)
}

// Order returns an ordering by this field.
func (f Enum[T]) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f Enum[T]) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f Enum[T]) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
func (f Enum[T]) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
func (f Enum[T]) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field. Passes v as-is (T, not string(v)) — same
// reasoning as String[T].Set: the mutation descriptor expects the field's
// declared enum Go type, not a bare string.
func (f Enum[T]) Set(v T) Assignment {
	return func(m Mutable) error { return m.SetField(f.name, v) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f Enum[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f Enum[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}

// Value is a generic handle for arbitrary value fields. See String's doc
// comment for why col and name are kept separate.
type Value[T any] struct {
	col, name string
	scan      func(T) (driver.Value, error)
}

// NewValue creates a new Value handle for the given column/field name.
func NewValue[T any](col, name string) Value[T] {
	return Value[T]{col: col, name: name}
}

// NewValueScan creates a new Value handle with a custom scanner function.
func NewValueScan[T any](col, name string, scan func(T) (driver.Value, error)) Value[T] {
	return Value[T]{col: col, name: name, scan: scan}
}

// Column returns the column name.
func (f Value[T]) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f Value[T]) EQ(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.EQ(s.C(f.col), scanned))
		}
	}
	return sql.FieldEQ(f.col, v)
}

// NEQ returns a predicate for inequality.
func (f Value[T]) NEQ(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.NEQ(s.C(f.col), scanned))
		}
	}
	return sql.FieldNEQ(f.col, v)
}

// In returns a predicate for membership.
func (f Value[T]) In(vs ...T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned := make([]any, len(vs))
			for i, v := range vs {
				val, err := f.scan(v)
				if err != nil {
					s.AddError(err)
					return
				}
				scanned[i] = val
			}
			s.Where(sql.In(s.C(f.col), scanned...))
		}
	}
	return sql.FieldIn(f.col, vs...)
}

// NotIn returns a predicate for non-membership.
func (f Value[T]) NotIn(vs ...T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned := make([]any, len(vs))
			for i, v := range vs {
				val, err := f.scan(v)
				if err != nil {
					s.AddError(err)
					return
				}
				scanned[i] = val
			}
			s.Where(sql.NotIn(s.C(f.col), scanned...))
		}
	}
	return sql.FieldNotIn(f.col, vs...)
}

// GT returns a predicate for greater than.
func (f Value[T]) GT(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.GT(s.C(f.col), scanned))
		}
	}
	return sql.FieldGT(f.col, v)
}

// GTE returns a predicate for greater than or equal.
func (f Value[T]) GTE(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.GTE(s.C(f.col), scanned))
		}
	}
	return sql.FieldGTE(f.col, v)
}

// LT returns a predicate for less than.
func (f Value[T]) LT(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.LT(s.C(f.col), scanned))
		}
	}
	return sql.FieldLT(f.col, v)
}

// LTE returns a predicate for less than or equal.
func (f Value[T]) LTE(v T) P {
	if f.scan != nil {
		return func(s *sql.Selector) {
			scanned, err := f.scan(v)
			if err != nil {
				s.AddError(err)
				return
			}
			s.Where(sql.LTE(s.C(f.col), scanned))
		}
	}
	return sql.FieldLTE(f.col, v)
}

// Order returns an ordering by this field.
func (f Value[T]) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f Value[T]) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f Value[T]) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
func (f Value[T]) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
func (f Value[T]) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field, passing the raw Go value (not the scan-func-
// encoded form): scanning happens later at spec-build time.
func (f Value[T]) Set(v T) Assignment {
	return func(m Mutable) error { return m.SetField(f.name, v) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f Value[T]) SetNillable(v *T) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f Value[T]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}

// Bytes is a handle for byte slice fields. See String's doc comment for why
// col and name are kept separate.
type Bytes struct {
	col, name string
}

// NewBytes creates a new Bytes handle for the given column/field name.
func NewBytes(col, name string) Bytes {
	return Bytes{col: col, name: name}
}

// Column returns the column name.
func (f Bytes) Column() string { return f.col }

// EQ returns a predicate for equality.
func (f Bytes) EQ(v []byte) P {
	return sql.FieldEQ(f.col, v)
}

// NEQ returns a predicate for inequality.
func (f Bytes) NEQ(v []byte) P {
	return sql.FieldNEQ(f.col, v)
}

// In returns a predicate for membership.
func (f Bytes) In(vs ...[]byte) P {
	return sql.FieldIn(f.col, vs...)
}

// NotIn returns a predicate for non-membership.
func (f Bytes) NotIn(vs ...[]byte) P {
	return sql.FieldNotIn(f.col, vs...)
}

// GT returns a predicate for greater than.
func (f Bytes) GT(v []byte) P {
	return sql.FieldGT(f.col, v)
}

// GTE returns a predicate for greater than or equal.
func (f Bytes) GTE(v []byte) P {
	return sql.FieldGTE(f.col, v)
}

// LT returns a predicate for less than.
func (f Bytes) LT(v []byte) P {
	return sql.FieldLT(f.col, v)
}

// LTE returns a predicate for less than or equal.
func (f Bytes) LTE(v []byte) P {
	return sql.FieldLTE(f.col, v)
}

// Order returns an ordering by this field.
func (f Bytes) Order(opts ...sql.OrderTermOption) Order {
	return sql.OrderByField(f.col, opts...).ToFunc()
}

// Asc returns an ascending ordering by this field.
func (f Bytes) Asc() Order {
	return f.Order()
}

// Desc returns a descending ordering by this field.
func (f Bytes) Desc() Order {
	return f.Order(sql.OrderDesc())
}

// IsNil returns a predicate for checking NULL values.
func (f Bytes) IsNil() P {
	return sql.FieldIsNull(f.col)
}

// NotNil returns a predicate for checking non-NULL values.
func (f Bytes) NotNil() P {
	return sql.FieldNotNull(f.col)
}

// Set assigns v to the field.
func (f Bytes) Set(v []byte) Assignment {
	return func(m Mutable) error { return m.SetField(f.name, v) }
}

// SetNillable assigns *v when non-nil; no-op otherwise.
func (f Bytes) SetNillable(v *[]byte) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return f.Set(*v)(m)
	}
}

// Clear clears the field.
// ponytail: exposed on all handles; non-optional misuse surfaces at save/DB.
func (f Bytes) Clear() Assignment {
	return func(m Mutable) error { return m.ClearField(f.name) }
}
