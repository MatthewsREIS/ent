// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package load

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeCodegenTag(t *testing.T) {
	t.Run("appends when no -tags present", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-gcflags=foo"})
		require.Equal(t, []string{"-gcflags=foo", "-tags=entcodegen"}, got)
	})

	t.Run("merges into single -tags=foo arg", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags=hidegroups"})
		require.Equal(t, []string{"-tags=hidegroups,entcodegen"}, got)
	})

	t.Run("merges into two-arg -tags foo form", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags", "hidegroups"})
		require.Equal(t, []string{"-tags", "hidegroups,entcodegen"}, got)
	})

	t.Run("idempotent when entcodegen already present in -tags=", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags=entcodegen,foo"})
		require.Equal(t, []string{"-tags=entcodegen,foo"}, got)
	})

	t.Run("idempotent when entcodegen already present in -tags foo", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags", "foo,entcodegen"})
		require.Equal(t, []string{"-tags", "foo,entcodegen"}, got)
	})

	t.Run("nil input yields just -tags=entcodegen", func(t *testing.T) {
		got := mergeCodegenTag(nil)
		require.Equal(t, []string{"-tags=entcodegen"}, got)
	})

	t.Run("preserves other flags around -tags", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-gcflags=foo", "-tags=hidegroups", "-race"})
		require.Equal(t, []string{"-gcflags=foo", "-tags=hidegroups,entcodegen", "-race"}, got)
	})

	t.Run("merges into last -tags when multiple present", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags=foo", "-tags=bar"})
		require.Equal(t, []string{"-tags=foo", "-tags=bar,entcodegen"}, got)
	})

	t.Run("handles empty value in -tags= form", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags="})
		require.Equal(t, []string{"-tags=entcodegen"}, got)
	})

	t.Run("handles empty value in two-arg form", func(t *testing.T) {
		got := mergeCodegenTag([]string{"-tags", ""})
		require.Equal(t, []string{"-tags", "entcodegen"}, got)
	})
}
