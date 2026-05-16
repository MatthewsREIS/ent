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
