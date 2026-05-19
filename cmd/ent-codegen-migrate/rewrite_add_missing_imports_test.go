// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteAddMissingImports_AddsKnownPackage(t *testing.T) {
	pkgImports = map[string]string{
		"time": "time",
		"uuid": "github.com/google/uuid",
	}
	defer func() { pkgImports = map[string]string{} }()

	src := `package x

import "entgo.io/ent/runtime/entbuilder"

func hook(m any) {
	_, _ = entbuilder.GetField[time.Time](m, "deleted_at")
}
`
	out, err := RewriteAddMissingImportsSource("hook.go", src, nil, "")
	require.NoError(t, err)
	require.Contains(t, out, `"time"`, "time import must be added")
	require.Contains(t, out, "entbuilder.GetField[time.Time]")
}

func TestRewriteAddMissingImports_SkipsAlreadyImported(t *testing.T) {
	pkgImports = map[string]string{"time": "time"}
	defer func() { pkgImports = map[string]string{} }()

	src := `package x

import (
	"time"

	"entgo.io/ent/runtime/entbuilder"
)

func hook(m any) {
	_, _ = entbuilder.GetField[time.Time](m, "deleted_at")
}
`
	out, err := RewriteAddMissingImportsSource("hook.go", src, nil, "")
	require.NoError(t, err)
	// Single time import — astutil.AddImport is a no-op when already present.
	count := 0
	for _, line := range splitLines(out) {
		if line == `	"time"` {
			count++
		}
	}
	require.Equal(t, 1, count, "time import must not be duplicated")
}

func TestRewriteAddMissingImports_LeavesUnknownPackageAlone(t *testing.T) {
	pkgImports = map[string]string{"time": "time"}
	defer func() { pkgImports = map[string]string{} }()

	src := `package x

import "entgo.io/ent/runtime/entbuilder"

func hook(m any) {
	_ = entbuilder.GetField[unknownpkg.Type](m, "field")
}
`
	out, err := RewriteAddMissingImportsSource("hook.go", src, nil, "")
	require.NoError(t, err)
	// The reference stays (compile will fail elsewhere — not our job to fix),
	// but we must NOT have added an import line for "unknownpkg".
	require.NotContains(t, out, `"unknownpkg"`, "unknown package alias must not become an import")
	// Also verify time wasn't spuriously added.
	require.NotContains(t, out, `"time"`)
}

func TestRewriteAddMissingImports_PkgImportsEmpty(t *testing.T) {
	pkgImports = map[string]string{}

	src := `package x
import "entgo.io/ent/runtime/entbuilder"
func hook() { _ = entbuilder.GetField[time.Time] }
`
	out, err := RewriteAddMissingImportsSource("hook.go", src, nil, "")
	require.NoError(t, err)
	require.Equal(t, src, out)
}

func TestRewriteAddMissingImports_Idempotent(t *testing.T) {
	pkgImports = map[string]string{"time": "time"}
	defer func() { pkgImports = map[string]string{} }()

	src := `package x

import "entgo.io/ent/runtime/entbuilder"

func hook(m any) {
	_, _ = entbuilder.GetField[time.Time](m, "deleted_at")
}
`
	pass1, err := RewriteAddMissingImportsSource("hook.go", src, nil, "")
	require.NoError(t, err)
	pass2, err := RewriteAddMissingImportsSource("hook.go", pass1, nil, "")
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
