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
