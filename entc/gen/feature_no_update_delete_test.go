// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// featureByName returns the Feature with the given name from the slice, or
// the zero value if not found. Used to avoid DeepEqual on function fields.
func featureByName(features []Feature, name string) (Feature, bool) {
	for _, f := range features {
		if f.Name == name {
			return f, true
		}
	}
	return Feature{}, false
}

func TestFeatureNoUpdate_DefinitionPresent(t *testing.T) {
	require.Equal(t, "no-update", FeatureNoUpdate.Name, "feature must register the canonical name template authors will gate on")
	require.False(t, FeatureNoUpdate.Default, "default must be off so existing consumers see no behavior change")
	require.NotEmpty(t, FeatureNoUpdate.Description)
	f, ok := featureByName(AllFeatures, "no-update")
	require.True(t, ok, "FeatureNoUpdate must be registered in AllFeatures so --feature no-update works")
	require.Equal(t, FeatureNoUpdate.Name, f.Name)
}

func TestFeatureNoDelete_DefinitionPresent(t *testing.T) {
	require.Equal(t, "no-delete", FeatureNoDelete.Name)
	require.False(t, FeatureNoDelete.Default)
	require.NotEmpty(t, FeatureNoDelete.Description)
	f, ok := featureByName(AllFeatures, "no-delete")
	require.True(t, ok, "FeatureNoDelete must be registered in AllFeatures")
	require.Equal(t, FeatureNoDelete.Name, f.Name)
}
