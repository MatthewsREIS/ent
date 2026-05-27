// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package load

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoadStripsHookRefsOnRetry verifies that a schema whose Hooks() method
// references a symbol defined only in a //go:build !entcodegen file still loads.
// A plain typecheck under the injected entcodegen tag fails ("undefined:
// userHook"), so the loader must stage a hook-stripped copy and retry — the same
// robustness entc.Generate gets via SkipHookCompilation, but now available to any
// loader consumer (e.g. atlas's ent:// migration provider).
func TestLoadStripsHookRefsOnRetry(t *testing.T) {
	cfg := &Config{Path: "./testdata/hookref"}
	spec, err := cfg.Load()
	require.NoError(t, err)
	require.Len(t, spec.Schemas, 1)
	require.Equal(t, "User", spec.Schemas[0].Name)
	// PkgPath must be the real package, not the ent-bootstrap-* staging dir.
	require.Equal(t, "entgo.io/ent/entc/load/testdata/hookref", spec.PkgPath)
	require.Len(t, spec.Schemas[0].Fields, 1)
	require.Equal(t, "name", spec.Schemas[0].Fields[0].Name)
}
