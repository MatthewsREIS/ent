// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFormatCacheSkipsUnchangedFiles verifies that format() does not rewrite
// files whose formatted content is identical to what is already on disk,
// preserving file modification times for the Go build cache.
func TestFormatCacheSkipsUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unchanged.go")

	// Valid Go source that goimports will not modify.
	src := []byte("package foo\n")

	a := assets{}
	a.add(path, src)

	// First format: should create the file.
	require.NoError(t, a.format())
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(src), string(got))

	// Record modification time after first write.
	fi1, err := os.Stat(path)
	require.NoError(t, err)
	mtime1 := fi1.ModTime()

	// Ensure filesystem time resolution allows us to detect changes.
	time.Sleep(50 * time.Millisecond)

	// Second format with identical content: file should NOT be rewritten.
	a2 := assets{}
	a2.add(path, src)
	require.NoError(t, a2.format())

	fi2, err := os.Stat(path)
	require.NoError(t, err)
	mtime2 := fi2.ModTime()

	require.Equal(t, mtime1, mtime2, "file mtime should not change when content is identical")
}

// TestFormatCacheWritesChangedFiles verifies that format() does rewrite files
// whose formatted content differs from what is already on disk.
func TestFormatCacheWritesChangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changed.go")

	// Write initial content.
	src1 := []byte("package foo\n\nvar X = 1\n")
	a1 := assets{}
	a1.add(path, src1)
	require.NoError(t, a1.format())

	fi1, err := os.Stat(path)
	require.NoError(t, err)
	mtime1 := fi1.ModTime()

	// Ensure filesystem time resolution allows us to detect changes.
	time.Sleep(50 * time.Millisecond)

	// Format with different content: file SHOULD be rewritten.
	src2 := []byte("package foo\n\nvar Y = 2\n")
	a2 := assets{}
	a2.add(path, src2)
	require.NoError(t, a2.format())

	fi2, err := os.Stat(path)
	require.NoError(t, err)
	mtime2 := fi2.ModTime()

	require.True(t, mtime2.After(mtime1), "file mtime should change when content differs")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(got), "var Y = 2")
}

// TestFormatCacheCreatesNewFiles verifies that format() creates files that
// do not yet exist on disk (first generation run).
func TestFormatCacheCreatesNewFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.go")

	src := []byte("package foo\n\nvar Z = 3\n")
	a := assets{}
	a.add(path, src)
	require.NoError(t, a.format())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(got), "var Z = 3")
}

// TestWriteOnlyCreatesDirectories verifies that write() creates directories
// but does not write files (format() handles file writes with caching).
func TestWriteOnlyCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub", "nested")
	path := filepath.Join(subdir, "file.go")

	a := assets{}
	a.addDir(subdir)
	a.add(path, []byte("package foo\n"))

	require.NoError(t, a.write())

	// Directory should exist.
	info, err := os.Stat(subdir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// File should NOT exist (format() will write it).
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err), "write() should not create files; format() handles that")
}
