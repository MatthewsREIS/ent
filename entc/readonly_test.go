// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc_test

import (
	"testing"

	"entgo.io/ent/entc"

	"github.com/stretchr/testify/require"
)

func TestReadOnly_AnnotationShape(t *testing.T) {
	a := entc.ReadOnly()
	require.Equal(t, "ReadOnly", a.Name(), "annotation must return canonical name templates key on")
}
