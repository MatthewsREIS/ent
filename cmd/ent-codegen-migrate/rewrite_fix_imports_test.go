// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteFixImports_RebindsUniqueSymbol(t *testing.T) {
	fixture := filepath.Join("testdata", "fix_imports", "basic")
	before, err := os.ReadFile(filepath.Join(fixture, "consumer", "before.go.txt"))
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join(fixture, "consumer", "after.go.txt"))
	require.NoError(t, err)

	cfg := FixImportsConfig{
		ModuleRoot:   fixture,
		EntRootPaths: []string{"example.com/fixmod/gen/"},
	}
	got, err := RewriteFixImportsSource("consumer/before.go.txt", string(before), cfg)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func TestRewriteFixImports_LeavesIntactImports(t *testing.T) {
	// Source is already goimports-normalized: stdlib and third-party imports
	// are separated by a blank line, all imports are used.
	src := `package consumer

import (
	"fmt"

	"example.com/fixmod/gen/contact"
)

func _f() { fmt.Println(contact.Find("a")) }
`
	cfg := FixImportsConfig{
		ModuleRoot:   filepath.Join("testdata", "fix_imports", "basic"),
		EntRootPaths: []string{"example.com/fixmod/gen/"},
	}
	got, err := RewriteFixImportsSource("x.go", src, cfg)
	require.NoError(t, err)
	require.Equal(t, src, got)
}

func TestRewriteFixImports_NeverTouchesThirdParty(t *testing.T) {
	src := `package consumer

import (
	"someorg.example/external/oldquery"
)

func _f() { _ = oldquery.Find("a") }
`
	cfg := FixImportsConfig{
		ModuleRoot:   filepath.Join("testdata", "fix_imports", "basic"),
		EntRootPaths: []string{"example.com/fixmod/gen/"},
	}
	got, err := RewriteFixImportsSource("x.go", src, cfg)
	require.NoError(t, err)
	require.Equal(t, src, got)
}

func TestRewriteFixImports_LeavesAmbiguousAlone(t *testing.T) {
	fixture := filepath.Join("testdata", "fix_imports", "ambiguous")
	before, err := os.ReadFile(filepath.Join(fixture, "consumer", "before.go.txt"))
	require.NoError(t, err)
	cfg := FixImportsConfig{
		ModuleRoot:   fixture,
		EntRootPaths: []string{"example.com/ambig/gen/"},
	}
	got, err := RewriteFixImportsSource("consumer/before.go.txt", string(before), cfg)
	require.NoError(t, err)
	require.Equal(t, string(before), got, "ambiguous symbol must leave import unchanged")
}

func TestRewriteFixImports_LeavesUnusedImportsWhenNoRebinding(t *testing.T) {
	// The pass is intentionally scoped to imports it actually rebinds — it
	// no longer runs goimports globally to clean up unused imports from
	// other passes (that caused whole-tree whitespace churn on consumer
	// runs). Verifies the file is returned untouched when no rebinding
	// happens, even if it has obviously-unused imports.
	src := `package consumer

import (
	"fmt"
	"strings"
)

func _f() { fmt.Println("x") }
`
	cfg := FixImportsConfig{
		ModuleRoot:   filepath.Join("testdata", "fix_imports", "basic"),
		EntRootPaths: []string{"example.com/fixmod/gen/"},
	}
	got, err := RewriteFixImportsSource("x.go", src, cfg)
	require.NoError(t, err)
	require.Equal(t, src, got, "file with no rebindings must be returned unchanged")
}
