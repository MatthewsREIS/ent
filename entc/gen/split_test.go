// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_normalizeSplit(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, cfg.normalizeSplit())
	require.Nil(t, cfg.Split)
}

func TestSplitConfig_NormalizeDefaults(t *testing.T) {
	cfg := &SplitConfig{}
	require.NoError(t, cfg.Normalize())
	require.Equal(t, SplitModeType, cfg.Mode)
	require.Nil(t, cfg.Include)
	require.Nil(t, cfg.Exclude)
}

func TestSplitConfig_NormalizeErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *SplitConfig
		contain string
	}{
		{
			name: "invalid mode",
			cfg: &SplitConfig{
				Mode: "unknown",
			},
			contain: `invalid split mode "unknown"`,
		},
		{
			name: "invalid include",
			cfg: &SplitConfig{
				Include: []string{"["},
			},
			contain: `invalid split include pattern "["`,
		},
		{
			name: "empty exclude",
			cfg: &SplitConfig{
				Exclude: []string{""},
			},
			contain: "split exclude pattern cannot be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Normalize()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.contain)
		})
	}
}

func TestSplitConfig_shouldSplit(t *testing.T) {
	cfg := SplitConfig{}
	require.NoError(t, cfg.Normalize())
	require.True(t, cfg.shouldSplit(splitAssetMeta{
		Template: "client",
		Output:   "client.go",
		Core:     true,
	}))
	require.False(t, cfg.shouldSplit(splitAssetMeta{
		Template: "entgql/template",
		Output:   "gql_node.go",
		Core:     false,
	}))

	cfg = SplitConfig{
		Include: []string{"gql_*.go"},
	}
	require.NoError(t, cfg.Normalize())
	require.True(t, cfg.shouldSplit(splitAssetMeta{
		Template: "entgql/template",
		Output:   "gql_node.go",
		Core:     false,
	}))

	cfg = SplitConfig{
		Include: []string{"*.go"},
		Exclude: []string{"gql_*.go"},
	}
	require.NoError(t, cfg.Normalize())
	require.False(t, cfg.shouldSplit(splitAssetMeta{
		Template: "entgql/template",
		Output:   "gql_node.go",
		Core:     false,
	}))
}

func TestAssets_cleanupSplitPreservesOtherGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "user.go")
	base := filepath.Join(dir, "user_base.go")
	other := filepath.Join(dir, "user_create.go")
	stale := filepath.Join(dir, "user_stale.go")

	for _, path := range []string{origin, base, other, stale} {
		require.NoError(t, os.WriteFile(path, []byte("package ent\n"), 0644))
	}

	a := assets{
		files: map[string]assetFile{
			origin: {
				content: []byte("package ent\n"),
				meta:    splitAssetMeta{Origin: origin},
			},
			base: {
				content: []byte("package ent\n"),
				meta:    splitAssetMeta{Origin: origin},
			},
			other: {
				content: []byte("package ent\n"),
				meta:    splitAssetMeta{Origin: other},
			},
		},
	}

	require.NoError(t, a.cleanupSplit())
	_, err := os.Stat(other)
	require.NoError(t, err)
	_, err = os.Stat(stale)
	require.True(t, os.IsNotExist(err))
}
