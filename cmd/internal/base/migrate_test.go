// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package base

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitAPIMigrateCmdDryRun(t *testing.T) {
	dir := writeMigrationFixture(t)

	cmd := splitAPIMigrateCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{dir})
	require.NoError(t, cmd.Execute())

	b, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)
	require.Contains(t, string(b), `NotFoundError{"user"}`)
	require.Contains(t, out.String(), "dry-run mode")
}

func TestSplitAPIMigrateCmdWriteAndReport(t *testing.T) {
	dir := writeMigrationFixture(t)
	report := filepath.Join(dir, "migration-report.md")

	cmd := splitAPIMigrateCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--write", "--report", report, dir})
	require.NoError(t, cmd.Execute())

	b, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)
	require.Contains(t, string(b), `NotFoundError{Label: "user"}`)

	r, err := os.ReadFile(report)
	require.NoError(t, err)
	require.Contains(t, string(r), "# Split API Migration Report")
	require.Contains(t, string(r), "notfound-keyed: 1")
	require.Contains(t, out.String(), "report written to")
}

func writeMigrationFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/migrate\n\ngo 1.24\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

type NotFoundError struct {
	Label string
}

func main() {
	_ = NotFoundError{"user"}
}
`), 0644))
	return dir
}
