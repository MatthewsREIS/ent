// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import "fmt"

// SplitMode defines the generation mode for split-package codegen.
type SplitMode string

const (
	// SplitModeLegacy keeps the current monolithic generation layout.
	SplitModeLegacy SplitMode = "legacy"
	// SplitModeCompat uses split internals while keeping compatibility-facing APIs.
	SplitModeCompat SplitMode = "compat"
	// SplitModeNative uses split internals and native package surfaces.
	SplitModeNative SplitMode = "native"
)

var (
	legacyTypeTemplates  = Templates
	legacyGraphTemplates = GraphTemplates

	splitCompatTypeTemplates  = withSplitTypeTemplates(Templates)
	splitCompatGraphTemplates = GraphTemplates
	splitNativeTypeTemplates  = withSplitTypeTemplates(Templates)
	splitNativeGraphTemplates = GraphTemplates
)

func (m SplitMode) valid() bool {
	switch m {
	case "", SplitModeLegacy, SplitModeCompat, SplitModeNative:
		return true
	default:
		return false
	}
}

// SplitGenerationMode returns the effective split generation mode after
// resolving feature flag and defaults.
func (c Config) SplitGenerationMode() (SplitMode, error) {
	if !c.SplitMode.valid() {
		return "", fmt.Errorf("invalid split mode %q", c.SplitMode)
	}
	// Split generation is only active when the feature-flag is enabled.
	if !c.featureEnabled(FeatureSplitPackages) {
		return SplitModeLegacy, nil
	}
	// With split enabled, default to compat mode unless native is explicitly set.
	switch c.SplitMode {
	case "", SplitModeLegacy, SplitModeCompat:
		return SplitModeCompat, nil
	case SplitModeNative:
		return SplitModeNative, nil
	default:
		return "", fmt.Errorf("invalid split mode %q", c.SplitMode)
	}
}

func (g *Graph) generationTemplates() ([]TypeTemplate, []GraphTemplate, error) {
	mode, err := g.Config.SplitGenerationMode()
	if err != nil {
		return nil, nil, err
	}
	switch mode {
	case SplitModeLegacy:
		return legacyTypeTemplates, legacyGraphTemplates, nil
	case SplitModeCompat:
		return splitCompatTypeTemplates, splitCompatGraphTemplates, nil
	case SplitModeNative:
		return splitNativeTypeTemplates, splitNativeGraphTemplates, nil
	default:
		return nil, nil, fmt.Errorf("invalid split mode %q", mode)
	}
}
