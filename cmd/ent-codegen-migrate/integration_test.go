// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegration_AllPassesAgainstEdgesFixture(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	descsDir := filepath.Join(root, "entc/integration/privacy/ent/internal")

	descs, err := LoadDescriptors(descsDir)
	require.NoError(t, err)
	require.NotEmpty(t, descs)

	before, err := os.ReadFile(filepath.Join("testdata", "before", "edges.go.txt"))
	require.NoError(t, err)
	wantAfter, err := os.ReadFile(filepath.Join("testdata", "after", "edges.go.txt"))
	require.NoError(t, err)

	out := string(before)
	// Bridge RewriteEdgeMethodSource (which takes a genImportPath param)
	// to the genPackage-less per-pass signature used by the other three
	// rewriters. Empty path → "ent" alias fallback, matching the fixture.
	edgeMethod := func(filename, src string, d Descriptors) (string, error) {
		return RewriteEdgeMethodSource(filename, src, d, "")
	}
	for _, fn := range []func(string, string, Descriptors) (string, error){
		RewriteMutationSource,
		RewritePredicateSource,
		edgeMethod,
		RewriteTypedEdgeAccessorSource,
	} {
		out, err = fn("edges.go", out, descs)
		require.NoError(t, err)
	}
	require.Equal(t, string(wantAfter), out)
}

func TestIntegration_RewritePrivacyFixtureHooks(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	descsDir := filepath.Join(root, "entc/integration/privacy/ent/internal")

	descs, err := LoadDescriptors(descsDir)
	require.NoError(t, err)
	require.NotEmpty(t, descs)

	before, err := os.ReadFile(filepath.Join("testdata", "before", "hook.go.txt"))
	require.NoError(t, err)
	wantAfter, err := os.ReadFile(filepath.Join("testdata", "after", "hook.go.txt"))
	require.NoError(t, err)

	out, err := RewriteMutationSource("hook.go", string(before), descs)
	require.NoError(t, err)
	out, err = RewritePredicateSource("hook.go", out, descs)
	require.NoError(t, err)

	require.Equal(t, string(wantAfter), out)
}
