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

func TestSetContextOp_AttachesOpWhenAbsent(t *testing.T) {
	qc := &ent.QueryContext{Type: "User"}
	ctx := entbuilder.SetContextOp(context.Background(), qc, "QueryAll")
	gotQC := ent.QueryFromContext(ctx)
	require.NotNil(t, gotQC)
	require.Equal(t, "QueryAll", gotQC.Op)
	require.Equal(t, "User", gotQC.Type)
}

func TestSetContextOp_DoesNotOverrideExisting(t *testing.T) {
	parent := &ent.QueryContext{Op: "Outer", Type: "User"}
	ctx := ent.NewQueryContext(context.Background(), parent)
	qc := &ent.QueryContext{Type: "User"}
	ctx = entbuilder.SetContextOp(ctx, qc, "Inner")
	gotQC := ent.QueryFromContext(ctx)
	require.NotNil(t, gotQC)
	require.Equal(t, "Outer", gotQC.Op, "outer QueryContext must win when one is already attached")
}

func TestWithInterceptors_NoInters_CallsQuerier(t *testing.T) {
	called := false
	qr := ent.QuerierFunc(func(_ context.Context, _ ent.Query) (ent.Value, error) {
		called = true
		return 42, nil
	})
	v, err := entbuilder.WithInterceptors[int](context.Background(), nil, qr, nil)
	require.NoError(t, err)
	require.Equal(t, 42, v)
	require.True(t, called)
}

func TestWithInterceptors_ChainsInReverseOrder(t *testing.T) {
	var order []string
	mk := func(name string) ent.Interceptor {
		return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
				order = append(order, name+":before")
				v, err := next.Query(ctx, q)
				order = append(order, name+":after")
				return v, err
			})
		})
	}
	qr := ent.QuerierFunc(func(_ context.Context, _ ent.Query) (ent.Value, error) {
		order = append(order, "inner")
		return 0, nil
	})
	_, err := entbuilder.WithInterceptors[int](context.Background(), nil, qr, []ent.Interceptor{mk("A"), mk("B")})
	require.NoError(t, err)
	require.Equal(t, []string{"A:before", "B:before", "inner", "B:after", "A:after"}, order)
}
