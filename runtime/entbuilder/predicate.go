package entbuilder

import "entgo.io/ent/dialect/sql"

// FieldEQ returns a predicate of type P that applies sql.FieldEQ under the hood.
// P must be a named function type whose underlying type is func(*sql.Selector)
// (i.e. the `predicate.Xxx` types produced by codegen).
//
// Purpose: codegen can replace the repeated body
//
//	return predicate.Xxx(sql.FieldEQ(FieldY, v))
//
// with a single generic call:
//
//	return FieldEQ[predicate.Xxx](FieldY, v)
//
// The type parameter is the only mechanism keeping predicate.Xxx from predicate.Yyy
// at the call site. Instantiation cost scales with the number of distinct named
// predicate types in the consumer's codebase (one per schema).
func FieldEQ[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldEQ(field, v))
}

// FieldNEQ returns a predicate of type P wrapping sql.FieldNEQ.
func FieldNEQ[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldNEQ(field, v))
}

// FieldGT returns a predicate of type P wrapping sql.FieldGT.
func FieldGT[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldGT(field, v))
}

// FieldGTE returns a predicate of type P wrapping sql.FieldGTE.
func FieldGTE[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldGTE(field, v))
}

// FieldLT returns a predicate of type P wrapping sql.FieldLT.
func FieldLT[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldLT(field, v))
}

// FieldLTE returns a predicate of type P wrapping sql.FieldLTE.
func FieldLTE[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldLTE(field, v))
}

// FieldIn returns a predicate of type P wrapping sql.FieldIn.
// The caller passes a []any slice; this helper spreads it into the variadic sql.FieldIn[any].
func FieldIn[P ~func(*sql.Selector)](field string, vs []any) P {
	return P(sql.FieldIn[any](field, vs...))
}

// FieldNotIn returns a predicate of type P wrapping sql.FieldNotIn.
// The caller passes a []any slice; this helper spreads it into the variadic sql.FieldNotIn[any].
func FieldNotIn[P ~func(*sql.Selector)](field string, vs []any) P {
	return P(sql.FieldNotIn[any](field, vs...))
}

// FieldIsNull returns a predicate of type P wrapping sql.FieldIsNull (niladic — no value arg).
func FieldIsNull[P ~func(*sql.Selector)](field string) P {
	return P(sql.FieldIsNull(field))
}

// FieldNotNull returns a predicate of type P wrapping sql.FieldNotNull (niladic — no value arg).
func FieldNotNull[P ~func(*sql.Selector)](field string) P {
	return P(sql.FieldNotNull(field))
}

// FieldContains returns a predicate of type P wrapping sql.FieldContains.
func FieldContains[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldContains(field, v))
}

// FieldHasPrefix returns a predicate of type P wrapping sql.FieldHasPrefix.
func FieldHasPrefix[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldHasPrefix(field, v))
}

// FieldHasSuffix returns a predicate of type P wrapping sql.FieldHasSuffix.
func FieldHasSuffix[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldHasSuffix(field, v))
}

// FieldEqualFold returns a predicate of type P wrapping sql.FieldEqualFold.
func FieldEqualFold[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldEqualFold(field, v))
}

// FieldContainsFold returns a predicate of type P wrapping sql.FieldContainsFold.
func FieldContainsFold[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldContainsFold(field, v))
}

// AndPreds returns a predicate of type P that ANDs all given predicates together,
// wrapping sql.AndPredicates. Because sql.AndPredicates is itself generic with the
// same P ~func(*sql.Selector) constraint, no slice conversion is needed.
func AndPreds[P ~func(*sql.Selector)](ps ...P) P {
	return P(sql.AndPredicates(ps...))
}

// OrPreds returns a predicate of type P that ORs all given predicates together,
// wrapping sql.OrPredicates.
func OrPreds[P ~func(*sql.Selector)](ps ...P) P {
	return P(sql.OrPredicates(ps...))
}

// NotPred returns a predicate of type P that negates the given predicate,
// wrapping sql.NotPredicates.
func NotPred[P ~func(*sql.Selector)](p P) P {
	return P(sql.NotPredicates(p))
}
