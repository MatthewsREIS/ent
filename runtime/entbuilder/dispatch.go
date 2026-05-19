// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"fmt"

	"entgo.io/ent"
)

// SetContextOp attaches the given QueryContext (including its op) to ctx, but only
// if no QueryContext is already present. This mirrors the per-entity-package
// setContextOp helper that ent codegen has emitted for years, hoisted here so
// the generic Run* helpers don't need to call back into the generated package.
func SetContextOp(ctx context.Context, qc *ent.QueryContext, op string) context.Context {
	if ent.QueryFromContext(ctx) == nil {
		qc.Op = op
		ctx = ent.NewQueryContext(ctx, qc)
	}
	return ctx
}

// WithInterceptors invokes the querier through the given interceptor chain.
// Interceptors are applied in reverse order (last registered runs outermost),
// matching the per-entity-package withInterceptors helper that ent codegen has
// emitted for years.
func WithInterceptors[V ent.Value](ctx context.Context, q ent.Query, qr ent.Querier, inters []ent.Interceptor) (V, error) {
	for i := len(inters) - 1; i >= 0; i-- {
		qr = inters[i].Intercept(qr)
	}
	rv, err := qr.Query(ctx, q)
	if err != nil {
		var zero V
		return zero, err
	}
	v, ok := rv.(V)
	if !ok {
		var zero V
		return zero, fmt.Errorf("entbuilder: unexpected query result type %T", rv)
	}
	return v, nil
}
