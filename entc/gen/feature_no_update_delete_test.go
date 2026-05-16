// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeatureNoUpdate_DefinitionPresent(t *testing.T) {
	require.Equal(t, "no-update", FeatureNoUpdate.Name, "feature must register the canonical name template authors will gate on")
	require.False(t, FeatureNoUpdate.Default, "default must be off so existing consumers see no behavior change")
	require.NotEmpty(t, FeatureNoUpdate.Description)
	require.Contains(t, AllFeatures, FeatureNoUpdate, "feature must be registered in AllFeatures so --feature no-update works")
}

func TestFeatureNoDelete_DefinitionPresent(t *testing.T) {
	require.Equal(t, "no-delete", FeatureNoDelete.Name)
	require.False(t, FeatureNoDelete.Default)
	require.NotEmpty(t, FeatureNoDelete.Description)
	require.Contains(t, AllFeatures, FeatureNoDelete)
}
