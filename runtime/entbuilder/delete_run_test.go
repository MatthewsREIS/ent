// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

func TestRunDelete_NoHooks_CallsSqlExec(t *testing.T) {
	state := &entbuilder.DeleteState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpDelete},
	}
	called := false
	sqlExec := func(context.Context) (int, error) {
		called = true
		return 5, nil
	}
	n, err := entbuilder.RunDelete(context.Background(), state, sqlExec)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.True(t, called)
}
