// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypeScope_SiblingImportsInSubPackage(t *testing.T) {
	ty := &Type{Name: "Task", Config: &Config{Package: "example.com/ent"}}
	scope := &typeScope{Type: ty, Scope: map[any]any{"InSubPackage": true}}
	require.Nil(t, scope.SiblingImports(), "sub-package context must emit zero sibling imports")
}

func TestTypeScope_SiblingImportsAtRoot(t *testing.T) {
	ty := &Type{Name: "Task", Config: &Config{Package: "example.com/ent"}}
	scope := &typeScope{Type: ty, Scope: map[any]any{}}
	got := scope.SiblingImports()
	require.NotNil(t, got, "root context must delegate to Type.SiblingImports()")
	require.NotEmpty(t, got, "self-import always present")
}
