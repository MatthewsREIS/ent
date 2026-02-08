// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigSplitGenerationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		want    SplitMode
		wantErr string
	}{
		{
			name: "default legacy when split disabled",
			cfg:  Config{},
			want: SplitModeLegacy,
		},
		{
			name: "split enabled defaults to compat",
			cfg: Config{
				Features: []Feature{FeatureSplitPackages},
			},
			want: SplitModeCompat,
		},
		{
			name: "split enabled coerces legacy to compat",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeLegacy,
			},
			want: SplitModeCompat,
		},
		{
			name: "split enabled supports compat",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeCompat,
			},
			want: SplitModeCompat,
		},
		{
			name: "split enabled supports native",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeNative,
			},
			want: SplitModeNative,
		},
		{
			name: "split mode ignored when split disabled",
			cfg: Config{
				SplitMode: SplitModeNative,
			},
			want: SplitModeLegacy,
		},
		{
			name: "invalid split mode",
			cfg: Config{
				SplitMode: SplitMode("wat"),
			},
			wantErr: `invalid split mode "wat"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.cfg.SplitGenerationMode()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGraphGenerationTemplates(t *testing.T) {
	origLegacyType := legacyTypeTemplates
	origLegacyGraph := legacyGraphTemplates
	origCompatType := splitCompatTypeTemplates
	origCompatGraph := splitCompatGraphTemplates
	origNativeType := splitNativeTypeTemplates
	origNativeGraph := splitNativeGraphTemplates
	t.Cleanup(func() {
		legacyTypeTemplates = origLegacyType
		legacyGraphTemplates = origLegacyGraph
		splitCompatTypeTemplates = origCompatType
		splitCompatGraphTemplates = origCompatGraph
		splitNativeTypeTemplates = origNativeType
		splitNativeGraphTemplates = origNativeGraph
	})

	legacyTypeTemplates = []TypeTemplate{{Name: "legacy/type"}}
	legacyGraphTemplates = []GraphTemplate{{Name: "legacy/graph"}}
	splitCompatTypeTemplates = []TypeTemplate{{Name: "compat/type"}}
	splitCompatGraphTemplates = []GraphTemplate{{Name: "compat/graph"}}
	splitNativeTypeTemplates = []TypeTemplate{{Name: "native/type"}}
	splitNativeGraphTemplates = []GraphTemplate{{Name: "native/graph"}}

	tests := []struct {
		name      string
		cfg       Config
		wantType  string
		wantGraph string
		wantErr   string
	}{
		{
			name:      "legacy selection when split disabled",
			cfg:       Config{},
			wantType:  "legacy/type",
			wantGraph: "legacy/graph",
		},
		{
			name: "compat selection when split enabled",
			cfg: Config{
				Features: []Feature{FeatureSplitPackages},
			},
			wantType:  "compat/type",
			wantGraph: "compat/graph",
		},
		{
			name: "native selection when split mode native",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeNative,
			},
			wantType:  "native/type",
			wantGraph: "native/graph",
		},
		{
			name: "invalid mode returns error",
			cfg: Config{
				SplitMode: SplitMode("bad"),
			},
			wantErr: `invalid split mode "bad"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			g := &Graph{Config: &tt.cfg}
			ttmpl, gtmpl, err := g.generationTemplates()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantType, ttmpl[0].Name)
			require.Equal(t, tt.wantGraph, gtmpl[0].Name)
		})
	}
}
