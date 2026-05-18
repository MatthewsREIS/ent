// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteCollectionMethod_CollectFields_NoSatisfies(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
import "context"
type TodoQuery struct{}
type TodoConn struct{}
func (q *TodoQuery) CollectFields(ctx context.Context, satisfies ...string) (*TodoQuery, error) { return q, nil }
func use(ctx context.Context, q *TodoQuery) (*TodoQuery, error) { return q.CollectFields(ctx) }
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.TodoQueryCollectFields(q, ctx)")
	require.NotContains(t, out, "q.CollectFields(")
}

func TestRewriteCollectionMethod_CollectFields_WithSatisfies(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
import "context"
type TodoQuery struct{}
func (q *TodoQuery) CollectFields(ctx context.Context, satisfies ...string) (*TodoQuery, error) { return q, nil }
func use(ctx context.Context, q *TodoQuery) (*TodoQuery, error) { return q.CollectFields(ctx, "X", "Y") }
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `ent.TodoQueryCollectFields(q, ctx, "X", "Y")`)
	require.NotContains(t, out, "q.CollectFields(")
}

func TestRewriteCollectionMethod_Paginate_NoOpts(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
import "context"
type TodoQuery struct{}
type TodoConn struct{}
type Cursor struct{}
type PaginateOption func()
func (q *TodoQuery) Paginate(ctx context.Context, after *Cursor, first *int, before *Cursor, last *int, opts ...PaginateOption) (*TodoConn, error) { return nil, nil }
func use(ctx context.Context, q *TodoQuery, after *Cursor, first *int, before *Cursor, last *int) (*TodoConn, error) {
    return q.Paginate(ctx, after, first, before, last)
}
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.TodoQueryPaginate(q, ctx, after, first, before, last)")
	require.NotContains(t, out, "q.Paginate(")
}

func TestRewriteCollectionMethod_Paginate_WithOpts(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
import "context"
type TodoQuery struct{}
type TodoConn struct{}
type Cursor struct{}
type PaginateOption func()
func (q *TodoQuery) Paginate(ctx context.Context, after *Cursor, first *int, before *Cursor, last *int, opts ...PaginateOption) (*TodoConn, error) { return nil, nil }
func use(ctx context.Context, q *TodoQuery, after *Cursor, first *int, before *Cursor, last *int, opt1, opt2 PaginateOption) (*TodoConn, error) {
    return q.Paginate(ctx, after, first, before, last, opt1, opt2)
}
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.TodoQueryPaginate(q, ctx, after, first, before, last, opt1, opt2)")
	require.NotContains(t, out, "q.Paginate(")
}

func TestRewriteCollectionMethod_DifferentEntity(t *testing.T) {
	descs := Descriptors{
		"Category": &EntityDesc{Name: "Category"},
	}
	src := `package x
import "context"
type CategoryQuery struct{}
func (q *CategoryQuery) CollectFields(ctx context.Context, satisfies ...string) (*CategoryQuery, error) { return q, nil }
func use(ctx context.Context, cat *CategoryQuery) (*CategoryQuery, error) { return cat.CollectFields(ctx) }
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.CategoryQueryCollectFields(cat, ctx)")
	require.NotContains(t, out, "cat.CollectFields(")
}

// TestRewriteCollectionMethod_HonoursExplicitImportAlias verifies a custom
// import alias is preserved: `import myent "..."` → `myent.TodoQueryCollectFields(...)`.
func TestRewriteCollectionMethod_HonoursExplicitImportAlias(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
import myent "github.com/example/svc/internal/ent/gen"
type TodoQuery struct{}
func (q *TodoQuery) CollectFields(ctx interface{}, satisfies ...string) (*TodoQuery, error) { return q, nil }
var _ = myent.Foo
func use(q *TodoQuery, ctx interface{}) (*TodoQuery, error) { return q.CollectFields(ctx) }
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, "myent.TodoQueryCollectFields(q, ctx)")
	// Assert a bare " ent." selector is not present (avoid matching "myent.").
	require.NotContains(t, out, " ent.TodoQueryCollectFields")
}

// TestRewriteCollectionMethod_AddsMissingGenImport covers the case where
// a consumer file matches the rewrite pattern but doesn't yet import the
// gen package — the rewriter must add the import.
func TestRewriteCollectionMethod_AddsMissingGenImport(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
type TodoQuery struct{}
func (q *TodoQuery) CollectFields(ctx interface{}, satisfies ...string) (*TodoQuery, error) { return q, nil }
func use(q *TodoQuery, ctx interface{}) (*TodoQuery, error) { return q.CollectFields(ctx) }
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, "gen.TodoQueryCollectFields(q, ctx)")
	require.Contains(t, out, `"github.com/example/svc/internal/ent/gen"`,
		"missing gen-package import must be added when a rewrite fires")
}

// TestRewriteCollectionMethod_NoImportAddedWhenNoRewrite verifies the
// counterpart safety: if no rewrite actually fires in a file, the gen
// import must NOT be added (else go build fails with imported and not used).
func TestRewriteCollectionMethod_NoImportAddedWhenNoRewrite(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
type Helper struct{}
func (h *Helper) CollectFields(ctx interface{}) *Helper { return h }
func use(h *Helper, ctx interface{}) *Helper { return h.CollectFields(ctx) }
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.NotContains(t, out, genPath,
		"no rewrite emitted → no gen import should be added")
}

func TestRewriteCollectionMethod_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
import "context"
type TodoQuery struct{}
func (q *TodoQuery) CollectFields(ctx context.Context, satisfies ...string) (*TodoQuery, error) { return q, nil }
func use(ctx context.Context, q *TodoQuery) (*TodoQuery, error) { return q.CollectFields(ctx, "X") }
`
	pass1, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	pass2, err := RewriteCollectionMethodSource("x.go", pass1, descs, "")
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
}

// TestRewriteCollectionMethod_SkipsNonQueryReceiver guards against rewriting
// CollectFields/Paginate calls on unrelated user types that happen to share
// the method name (e.g. a hand-written helper, or a schema-DSL Paginate).
func TestRewriteCollectionMethod_SkipsNonQueryReceiver(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
type OtherType struct{}
func (o *OtherType) CollectFields(ctx interface{}) *OtherType { return o }
func (o *OtherType) Paginate(ctx interface{}, a, b, c, d interface{}) *OtherType { return o }
func use(o *OtherType, ctx interface{}) *OtherType {
    _ = o.CollectFields(ctx)
    return o.Paginate(ctx, nil, nil, nil, nil)
}
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "o.CollectFields(", "non-Query receiver must be skipped")
	require.Contains(t, out, "o.Paginate(", "non-Query receiver must be skipped")
	require.NotContains(t, out, "ent.TodoQueryCollectFields", "no rewrite expected")
	require.NotContains(t, out, "ent.TodoQueryPaginate", "no rewrite expected")
}

// TestRewriteCollectionMethod_CrossPackage_ChainWalker covers the cross-package
// consumer shape where go/types can't resolve the receiver — the syntactic
// chain walker must infer the *<Entity>Query receiver from a `q := &gen.TodoQuery{}`
// short-var declaration and rewrite the call.
func TestRewriteCollectionMethod_CrossPackage_ChainWalker(t *testing.T) {
	descs := Descriptors{
		"Todo": &EntityDesc{Name: "Todo"},
	}
	src := `package x
import "external/gen"
import "context"
func use(ctx context.Context) {
    q := &gen.TodoQuery{}
    _, _ = q.CollectFields(ctx, "X")
}
`
	out, err := RewriteCollectionMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `ent.TodoQueryCollectFields(q, ctx, "X")`,
		"cross-package CollectFields must be rewritten via the chain walker")
	require.NotContains(t, out, "q.CollectFields(", "original CollectFields call must not remain")
}
