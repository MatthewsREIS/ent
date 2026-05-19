// Package wherehelpers provides generic predicate-accumulator helpers used by
// entgql-generated where_input code. They collapse what would otherwise be
// per-field-per-operator boilerplate blocks of the form:
//
//	if i.X != nil { preds = append(preds, entity.XOp(*i.X)) }
//
// to a single helper call:
//
//	preds = wherehelpers.AppendPtr(preds, i.X, entity.XOp)
//
// The three helpers cover the three operator shapes the entgql where_input
// templates emit: pointer-comparable (EQ, NEQ, GT, GTE, LT, LTE, Contains,
// HasPrefix, HasSuffix, EqualFold, ContainsFold), variadic-slice (In, NotIn),
// and niladic-bool (IsNil, NotNil).
package wherehelpers

// AppendPtr appends op(*value) to preds when value is non-nil.
// Use for single-value field predicates (EQ, NEQ, GT, GTE, LT, LTE, Contains, etc.).
func AppendPtr[T any, P any](preds []P, value *T, op func(T) P) []P {
	if value != nil {
		preds = append(preds, op(*value))
	}
	return preds
}

// AppendSlice appends op(values...) to preds when values is non-empty.
// Use for variadic field predicates (In, NotIn).
func AppendSlice[T any, P any](preds []P, values []T, op func(...T) P) []P {
	if len(values) > 0 {
		preds = append(preds, op(values...))
	}
	return preds
}

// AppendBool appends op() to preds when value is true.
// Use for niladic field predicates (IsNil, NotNil).
func AppendBool[P any](preds []P, value bool, op func() P) []P {
	if value {
		preds = append(preds, op())
	}
	return preds
}
