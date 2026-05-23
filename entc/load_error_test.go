// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestWrapLoadError_PreservesNonPackageErrors(t *testing.T) {
	base := errors.New("connection refused")
	got := wrapLoadError(base, "./schema")
	require.ErrorIs(t, got, base, "non-packages error should pass through unchanged")
}

func TestWrapLoadError_AnnotatesGeneratedCodeReference(t *testing.T) {
	pkgErr := packages.Error{
		Pos:  "/repo/src/ent/schema/user_hooks.go:14:2",
		Msg:  "undefined: hook.UserFunc",
		Kind: packages.TypeError,
	}
	got := wrapLoadError(pkgErr, "./schema").Error()

	require.Contains(t, got, "/repo/src/ent/schema/user_hooks.go", "must name the failing file")
	require.Contains(t, got, "//go:build !entcodegen", "must suggest the build-tag escape hatch")
	require.Contains(t, got, "hook.UserFunc", "must show the undefined symbol")
}

func TestWrapLoadError_NoHintWhenSymbolDoesNotLookGenerated(t *testing.T) {
	pkgErr := packages.Error{
		Pos:  "/repo/src/ent/schema/user.go:9:2",
		Msg:  "undefined: time.Tomorrow",
		Kind: packages.TypeError,
	}
	got := wrapLoadError(pkgErr, "./schema").Error()

	require.Contains(t, got, "undefined: time.Tomorrow")
	require.NotContains(t, got, "//go:build !entcodegen", "should not suggest the tag for non-generated-looking symbols")
}
