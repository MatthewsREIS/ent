// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteHasAllFields_VariadicSpread(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	src := `package x
type PropertyQuery struct{}
func (q *PropertyQuery) HasAllFields(fields ...string) bool { return true }
func use(q *PropertyQuery, requiredFields []string) bool {
    return q.HasAllFields(requiredFields...)
}
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.PropertyHasAllFields(q, requiredFields...)")
	require.NotContains(t, out, "q.HasAllFields(")
}

func TestRewriteHasAllFields_LiteralArgs(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	src := `package x
type PropertyQuery struct{}
func (q *PropertyQuery) HasAllFields(fields ...string) bool { return true }
func use(q *PropertyQuery) bool {
    return q.HasAllFields("street_address", "city")
}
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `ent.PropertyHasAllFields(q, "street_address", "city")`)
	require.NotContains(t, out, "q.HasAllFields(")
}

func TestRewriteHasAllFields_DifferentEntity(t *testing.T) {
	descs := Descriptors{
		"Listing": &EntityDesc{Name: "Listing"},
	}
	src := `package x
type ListingQuery struct{}
func (q *ListingQuery) HasAllFields(fields ...string) bool { return true }
func use(lq *ListingQuery) bool {
    return lq.HasAllFields("price")
}
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, `ent.ListingHasAllFields(lq, "price")`)
	require.NotContains(t, out, "lq.HasAllFields(")
}

// TestRewriteHasAllFields_TypeAssertionVariable covers the real consumer
// shape from property_hooks.go: the query receiver is bound through a
// type-assertion (`q, ok := query.(*gen.PropertyQuery)`), and the chain
// walker must follow that assignment to extract the entity name.
func TestRewriteHasAllFields_TypeAssertionVariable(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	src := `package x
import "external/gen"
type GraphQLContext interface{}
func ensure(ctx GraphQLContext, requiredFields []string, query any) error {
    q, ok := query.(*gen.PropertyQuery)
    if !ok {
        return nil
    }
    if q.HasAllFields(requiredFields...) {
        return nil
    }
    return nil
}
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.PropertyHasAllFields(q, requiredFields...)",
		"HasAllFields on a type-assertion-bound receiver must be rewritten")
	require.NotContains(t, out, "q.HasAllFields(", "original HasAllFields call must not remain")
}

// TestRewriteHasAllFields_HonoursExplicitImportAlias verifies a custom
// import alias is preserved: `import myent "..."` → `myent.PropertyHasAllFields(...)`.
func TestRewriteHasAllFields_HonoursExplicitImportAlias(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
import myent "github.com/example/svc/internal/ent/gen"
type PropertyQuery struct{}
func (q *PropertyQuery) HasAllFields(fields ...string) bool { return true }
var _ = myent.Foo
func use(q *PropertyQuery) bool { return q.HasAllFields("x") }
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, `myent.PropertyHasAllFields(q, "x")`)
	require.NotContains(t, out, ` ent.PropertyHasAllFields`,
		"facade call must use the file's explicit alias, not a bare 'ent.' selector")
}

// TestRewriteHasAllFields_AddsMissingGenImport covers the case where a
// consumer file matches the rewrite pattern but doesn't yet import the gen
// package — the rewriter must add the import.
func TestRewriteHasAllFields_AddsMissingGenImport(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
type PropertyQuery struct{}
func (q *PropertyQuery) HasAllFields(fields ...string) bool { return true }
func use(q *PropertyQuery) bool { return q.HasAllFields("x") }
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, `gen.PropertyHasAllFields(q, "x")`)
	require.Contains(t, out, `"github.com/example/svc/internal/ent/gen"`,
		"missing gen-package import must be added when a rewrite fires")
}

// TestRewriteHasAllFields_NoImportAddedWhenNoRewrite verifies the
// counterpart safety: if no rewrite actually fires in a file, the gen
// import must NOT be added (else go build fails with imported and not used).
func TestRewriteHasAllFields_NoImportAddedWhenNoRewrite(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
type Helper struct{}
func (h *Helper) HasAllFields(fields ...string) bool { return true }
func use(h *Helper) bool { return h.HasAllFields("x") }
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.NotContains(t, out, genPath,
		"no rewrite emitted → no gen import should be added")
}

func TestRewriteHasAllFields_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	src := `package x
type PropertyQuery struct{}
func (q *PropertyQuery) HasAllFields(fields ...string) bool { return true }
func use(q *PropertyQuery) bool { return q.HasAllFields("x") }
`
	pass1, err := RewriteHasAllFieldsSource("x.go", src, descs, "")
	require.NoError(t, err)
	pass2, err := RewriteHasAllFieldsSource("x.go", pass1, descs, "")
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
}

// TestRewriteHasAllFields_SkipsNonQueryReceiver guards against rewriting
// HasAllFields calls on unrelated user types that happen to share the
// method name (e.g. a hand-written helper).
func TestRewriteHasAllFields_SkipsNonQueryReceiver(t *testing.T) {
	descs := Descriptors{
		"Property": &EntityDesc{Name: "Property"},
	}
	src := `package x
type OtherType struct{}
func (o *OtherType) HasAllFields(fields ...string) bool { return true }
func use(o *OtherType) bool { return o.HasAllFields("x") }
`
	out, err := RewriteHasAllFieldsSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "o.HasAllFields(", "non-Query receiver must be skipped")
	require.NotContains(t, out, "ent.PropertyHasAllFields", "no rewrite expected")
}
