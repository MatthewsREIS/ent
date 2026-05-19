// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountLOC(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package p\n\nfunc A() {}\n")            // 3 lines
	mustWrite(t, filepath.Join(root, "b.go"), "package p\nvar B = 1\n")                 // 2 lines
	mustWrite(t, filepath.Join(root, "sub", "c.go"), "package q\nfunc C() {\n\treturn\n}\n") // 4 lines
	mustWrite(t, filepath.Join(root, "ignore.txt"), "should be ignored\n")              // skipped: not .go

	total, count, top, err := CountLOC(root, 3)
	require.NoError(t, err)
	require.Equal(t, 9, total, "total LOC across all .go files")
	require.Equal(t, 3, count, "file count")
	require.Len(t, top, 3)

	// Top files are sorted by LOC desc; sort by path within equal-LOC ties handled by test order.
	sort.SliceStable(top, func(i, j int) bool { return top[i].LOC > top[j].LOC })
	require.Equal(t, 4, top[0].LOC)
	require.True(t, filepath.Base(top[0].Path) == "c.go", "largest file should be c.go, got %s", top[0].Path)
}

func TestCountLOC_TopNZero(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package p\n")
	total, count, top, err := CountLOC(root, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, count)
	require.Empty(t, top, "topN=0 returns no top-files list")
}

func TestCountLOC_MissingRoot(t *testing.T) {
	_, _, _, err := CountLOC("/this/path/does/not/exist", 5)
	require.Error(t, err)
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
