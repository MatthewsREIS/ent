// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDescriptors_FromFixturePackage(t *testing.T) {
	// Resolve the in-repo privacy fixture path relative to the test file.
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	pkg := filepath.Join(root, "entc/integration/privacy/ent/internal")

	descs, err := LoadDescriptors(pkg)
	require.NoError(t, err)
	require.Contains(t, descs, "Task")
	require.Contains(t, descs, "User")
	require.Contains(t, descs, "Team")

	task := descs["Task"]
	require.Contains(t, task.Fields, "title")
	require.Equal(t, "Title", task.Fields["title"].GoName)
	require.Equal(t, "string", task.Fields["title"].Type)
	require.Contains(t, task.Edges, "teams")
	require.Contains(t, task.Edges, "owner")
}
