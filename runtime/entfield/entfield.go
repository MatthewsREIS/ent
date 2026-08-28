package entfield

import (
	"database/sql/driver"
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

// anySlice converts a slice of type T to a slice of any.
func anySlice[T any](vs []T) []any {
	result := make([]any, len(vs))
	for i, v := range vs {
		result[i] = v
	}
	return result
}

// String is a generic handle for string-based fields.
type String[T ~string] struct {
	col  string
	scan func(T) (driver.Value, error)
}

// NewString creates a new String handle for the given column.
func NewString[T ~string](col string) String[T] {
	return String[T]{col: col}
}

// NewStringScan creates a new String handle with a custom scanner function.
func NewStringScan[T ~string](col string, scan func(T) (driver.Value, error)) String[T] {
	return String[T]{col: col, scan: scan}
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

// ponytail: full string op set on all String handles
// Contains returns a predicate for substring containment.
func (f String[T]) Contains(v T) P {
	return sql.FieldContains(f.col, string(v))
}

// HasPrefix returns a predicate for prefix matching.
func (f String[T]) HasPrefix(v T) P {
	return sql.FieldHasPrefix(f.col, string(v))
}

// HasSuffix returns a predicate for suffix matching.
func (f String[T]) HasSuffix(v T) P {
	return sql.FieldHasSuffix(f.col, string(v))
}

// EqualFold returns a predicate for case-insensitive equality.
func (f String[T]) EqualFold(v T) P {
	return sql.FieldEqualFold(f.col, string(v))
}

// ContainsFold returns a predicate for case-insensitive containment.
func (f String[T]) ContainsFold(v T) P {
	return sql.FieldContainsFold(f.col, string(v))
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

// Number is a generic handle for numeric fields.
type Number[T Numeric] struct {
	col string
}

// NewNumber creates a new Number handle for the given column.
func NewNumber[T Numeric](col string) Number[T] {
	return Number[T]{col: col}
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

// Bool is a generic handle for boolean fields.
type Bool[T ~bool] struct {
	col string
}

// NewBool creates a new Bool handle for the given column.
func NewBool[T ~bool](col string) Bool[T] {
	return Bool[T]{col: col}
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

// Time is a handle for time.Time fields.
type Time struct {
	col string
}

// NewTime creates a new Time handle for the given column.
func NewTime(col string) Time {
	return Time{col: col}
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

// Enum is a generic handle for enum fields.
type Enum[T ~string] struct {
	col string
}

// NewEnum creates a new Enum handle for the given column.
func NewEnum[T ~string](col string) Enum[T] {
	return Enum[T]{col: col}
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

// Value is a generic handle for arbitrary value fields.
type Value[T any] struct {
	col  string
	scan func(T) (driver.Value, error)
}

// NewValue creates a new Value handle for the given column.
func NewValue[T any](col string) Value[T] {
	return Value[T]{col: col}
}

// NewValueScan creates a new Value handle with a custom scanner function.
func NewValueScan[T any](col string, scan func(T) (driver.Value, error)) Value[T] {
	return Value[T]{col: col, scan: scan}
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

// Bytes is a handle for byte slice fields.
type Bytes struct {
	col string
}

// NewBytes creates a new Bytes handle for the given column.
func NewBytes(col string) Bytes {
	return Bytes{col: col}
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
