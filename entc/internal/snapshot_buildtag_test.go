// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"go/build"
	"os"
	"path/filepath"
	"testing"

	"entgo.io/ent/entc/gen"

	"github.com/stretchr/testify/require"
)

// TestSnapshotIsBuildTagged proves that the schema snapshot file produced by
// FeatureSnapshot is excluded from the consumer's default Go build via the
// "tools" build constraint. This is the design property that lets the snapshot
// be a safe recovery mechanism: it cannot bloat the consumer's compile graph,
// and a malformed snapshot cannot prevent the rest of the generated package
// from compiling.
//
// Regression guard: if a future template change drops the build constraint,
// the snapshot const becomes part of the consumer's compiled binary and the
// recovery contract is silently broken.
func TestSnapshotIsBuildTagged(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "integration", "*", "ent", "internal", "schema.go"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected snapshot fixtures under entc/integration/*/ent/internal/schema.go")

	for _, path := range matches {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			body := string(data)

			require.Contains(t, body, "//go:build tools",
				"modern build constraint missing: snapshot would be compiled into the consumer binary by default")
			require.Contains(t, body, "// +build tools",
				"legacy build constraint missing: required for older toolchains and gofmt round-trip")

			ctx := build.Default
			pkg, err := ctx.ImportDir(filepath.Dir(path), 0)
			require.NoError(t, err)
			require.NotContains(t, pkg.GoFiles, "schema.go",
				"schema.go is in default GoFiles: snapshot would contribute to the consumer's compile cost and break the recovery contract")
			require.Contains(t, pkg.IgnoredGoFiles, "schema.go",
				"schema.go should be in IgnoredGoFiles under the default build context")

			ctx.BuildTags = []string{"tools"}
			pkg, err = ctx.ImportDir(filepath.Dir(path), 0)
			require.NoError(t, err)
			require.Contains(t, pkg.GoFiles, "schema.go",
				"schema.go should be in GoFiles when -tags tools is set, so tooling that needs the snapshot can opt into it")
		})
	}
}

// TestSnapshotRestoreIsTextBased proves the snapshot recovery path does not
// require the snapshot file to be parseable as Go. Snapshot.Restore() scans
// the file as raw text for the `const Schema = "..."` line and extracts the
// quoted JSON; the surrounding Go declarations are never compiled or parsed
// by go/parser during recovery. This is what guarantees that "broken
// generated code" cannot lock out the regeneration that would fix it.
//
// Regression guard: if a future change makes Restore() depend on go/parser
// or on the snapshot file being a valid Go source file, then a partially
// corrupt snapshot would deadlock the recovery loop.
func TestSnapshotRestoreIsTextBased(t *testing.T) {
	// Copy the privacy fixture's snapshot to a temp file, then strip
	// everything except the const Schema line. The result is NOT a valid Go
	// source file (no package clause, no header) -- if Restore() relied on
	// go/parser, it would fail.
	srcPath := filepath.Join("..", "integration", "privacy", "ent", "internal", "schema.go")
	src, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	var schemaLine string
	for _, line := range splitLines(string(src)) {
		if hasPrefix(line, schemaIdent) {
			schemaLine = line
			break
		}
	}
	require.NotEmpty(t, schemaLine, "fixture snapshot is missing the const Schema line")

	tmp := t.TempDir()
	bareSnapshot := filepath.Join(tmp, "schema.go")
	require.NoError(t, os.WriteFile(bareSnapshot, []byte(schemaLine+"\n"), 0o644))

	storage, err := gen.NewStorage("sql")
	require.NoError(t, err)

	// We only need to prove parseSnapshot reads this file without error.
	// Construct a Snapshot and call the internal parser directly; running
	// the full Restore() would also exercise codegen on disk, which is
	// out of scope for this assertion.
	snap := &Snapshot{
		Path:   bareSnapshot,
		Config: &gen.Config{Storage: storage, Target: tmp},
	}
	buf, err := os.ReadFile(bareSnapshot)
	require.NoError(t, err)
	parsed, err := snap.parseSnapshot(buf)
	require.NoError(t, err, "parseSnapshot must succeed on a file containing only the const Schema line -- the snapshot loader is text-based, not Go-parser-based")
	require.NotNil(t, parsed)
	require.NotEmpty(t, parsed.Schemas, "parsed snapshot should contain the original schemas")
}

// Helpers kept local to avoid pulling in `strings` and `bytes` patterns from
// the production code into the test, which would make the assertion about
// "no go/parser dependency" muddier.
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

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
