// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc

import (
	"testing"

	"entgo.io/ent/entc/gen"

	"github.com/stretchr/testify/require"
)

func TestSplitOption(t *testing.T) {
	cfg := &gen.Config{}
	require.NoError(t, Split()(cfg))
	require.NotNil(t, cfg.Split)
	require.Equal(t, gen.SplitModeType, cfg.Split.Mode)
}

func TestSplitOptionValidation(t *testing.T) {
	cfg := &gen.Config{}
	err := Split(gen.SplitInclude("["))(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), `invalid split include pattern "["`)
	require.Nil(t, cfg.Split)
}

func TestSplitOptionMerge(t *testing.T) {
	cfg := &gen.Config{
		Split: &gen.SplitConfig{
			Include: []string{"client.go"},
		},
	}
	require.NoError(t, Split(gen.SplitExclude("mutation.go"))(cfg))
	require.Equal(t, []string{"client.go"}, cfg.Split.Include)
	require.Equal(t, []string{"mutation.go"}, cfg.Split.Exclude)
}
