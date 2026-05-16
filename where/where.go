// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package where

import "entgo.io/ent/dialect/sql"

// EQ returns a predicate that matches rows where the given field equals v.
func EQ[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldEQ(field, v)
}

// NEQ returns a predicate that matches rows where the given field does not equal v.
func NEQ[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldNEQ(field, v)
}

// In returns a predicate that matches rows where the field value is in vs.
func In[T any](field string, vs ...T) func(*sql.Selector) {
	return sql.FieldIn(field, vs...)
}

// NotIn returns a predicate that matches rows where the field value is not in vs.
func NotIn[T any](field string, vs ...T) func(*sql.Selector) {
	return sql.FieldNotIn(field, vs...)
}

// GT returns a predicate that matches rows where the field value is greater than v.
func GT[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldGT(field, v)
}

// GTE returns a predicate that matches rows where the field value is greater than or equal to v.
func GTE[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldGTE(field, v)
}

// LT returns a predicate that matches rows where the field value is less than v.
func LT[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldLT(field, v)
}

// LTE returns a predicate that matches rows where the field value is less than or equal to v.
func LTE[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldLTE(field, v)
}

// IsNull returns a predicate that matches rows where the field is NULL.
func IsNull(field string) func(*sql.Selector) {
	return sql.FieldIsNull(field)
}

// NotNull returns a predicate that matches rows where the field is NOT NULL.
func NotNull(field string) func(*sql.Selector) {
	return sql.FieldNotNull(field)
}
