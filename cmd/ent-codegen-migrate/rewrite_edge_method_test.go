// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteEdgeMethod_With(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskQuery struct{}
type TeamQuery struct{}
func (q *TaskQuery) WithTeams(opts ...func(*TeamQuery)) *TaskQuery { return q }
func use(q *TaskQuery) *TaskQuery { return q.WithTeams(func(q *TeamQuery) {}) }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.WithTaskTeams(q,")
	require.NotContains(t, out, "q.WithTeams(")
}

func TestRewriteEdgeMethod_QueryEdge(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskClient struct{}
type Task struct{}
type TeamQuery struct{}
func (c *TaskClient) QueryTeams(t *Task) *TeamQuery { return &TeamQuery{} }
func use(c *TaskClient, t *Task) *TeamQuery { return c.QueryTeams(t) }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.QueryTaskTeams(c, t)")
	require.NotContains(t, out, "c.QueryTeams(t)")
}

func TestRewriteEdgeMethod_QueryEdgeFromQuery(t *testing.T) {
	descs := Descriptors{
		"Proposal": &EntityDesc{
			Name:  "Proposal",
			Edges: map[string]EdgeDesc{"marketing": {Cardinality: "entbuilder.O2M", TargetIDType: "int", Target: "Marketing"}},
		},
	}
	src := `package x
type ProposalQuery struct{}
type MarketingQuery struct{}
func (q *ProposalQuery) QueryMarketing() *MarketingQuery { return &MarketingQuery{} }
func use(q *ProposalQuery) *MarketingQuery { return q.QueryMarketing() }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.QueryProposalMarketingFromQuery(q)")
	require.NotContains(t, out, "q.QueryMarketing()")
}

// TestRewriteEdgeMethod_QueryEdgeFromQuery_Chained covers the real consumer
// shape: gen.FromContext(ctx).Proposal.Query().Where(...).QueryMarketing()
// where cross-package types prevent go/types resolution. The syntactic
// chain walker must recognise the *<Entity>Query receiver via the
// .<Entity>.Query() prefix and rewrite to the FromQuery facade variant.
func TestRewriteEdgeMethod_QueryEdgeFromQuery_Chained(t *testing.T) {
	descs := Descriptors{
		"Proposal": &EntityDesc{
			Name:  "Proposal",
			Edges: map[string]EdgeDesc{"marketing": {Cardinality: "entbuilder.O2OUnique", TargetIDType: "uuid.UUID", Target: "Marketing"}},
		},
		"Marketing": &EntityDesc{
			Name:  "Marketing",
			Edges: map[string]EdgeDesc{"wrike_project": {Cardinality: "entbuilder.O2OUnique", TargetIDType: "uuid.UUID", Target: "WrikeProject"}},
		},
		"WrikeProject": &EntityDesc{Name: "WrikeProject"},
	}
	src := `package x
import "external/proposal"
import "external/gen"
import "context"
func use(ctx context.Context, id int) {
	_, _ = gen.FromContext(ctx).Proposal.Query().
		Where(proposal.ID(id)).
		QueryMarketing().
		QueryWrikeProject().
		IDs(ctx)
}
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.QueryProposalMarketingFromQuery(gen.FromContext(ctx).Proposal.Query().\n\t\tWhere(proposal.ID(id)))")
	require.Contains(t, out, "ent.QueryMarketingWrikeProjectFromQuery(ent.QueryProposalMarketingFromQuery(")
	require.NotContains(t, out, "QueryMarketing()")
	require.NotContains(t, out, "QueryWrikeProject()")
}

func TestRewriteEdgeMethod_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskQuery struct{}
type TeamQuery struct{}
func (q *TaskQuery) WithTeams(opts ...func(*TeamQuery)) *TaskQuery { return q }
func use(q *TaskQuery) *TaskQuery { return q.WithTeams(func(q *TeamQuery) {}) }
`
	pass1, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	pass2, err := RewriteEdgeMethodSource("x.go", pass1, descs, "")
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
}

func TestRewriteEdgeMethod_SkipsNonQueryReceiver(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type OtherType struct{}
func (o *OtherType) WithTeams(opts ...func()) *OtherType { return o }
func use(o *OtherType) *OtherType { return o.WithTeams(func() {}) }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "o.WithTeams(", "non-Query receiver must be skipped")
	require.NotContains(t, out, "ent.WithTaskTeams", "no rewrite expected")
}

// TestRewriteEdgeMethod_AliasesFacadeCallToGenPackage covers the gemini
// consumer shape: the generated ent package lives at .../api-graphql/src/ent/gen
// and is imported as "gen" (the default leaf-segment binding). Before the
// alias fix, the rewriter hardcoded "ent" as the package selector, which
// either resolved to upstream entgo.io/ent (compile error, undefined symbol)
// or to nothing. The rewriter must qualify facade calls with the file's
// actual local alias.
func TestRewriteEdgeMethod_AliasesFacadeCallToGenPackage(t *testing.T) {
	descs := Descriptors{
		"Proposal": &EntityDesc{
			Name:  "Proposal",
			Edges: map[string]EdgeDesc{"marketing": {Cardinality: "entbuilder.O2OUnique", TargetIDType: "uuid.UUID", Target: "Marketing"}},
		},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
import "github.com/example/svc/internal/ent/gen"
type ProposalQuery struct{}
type MarketingQuery struct{}
func (q *ProposalQuery) QueryMarketing() *MarketingQuery { return &MarketingQuery{} }
var _ = gen.Foo
func use(q *ProposalQuery) *MarketingQuery { return q.QueryMarketing() }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, "gen.QueryProposalMarketingFromQuery(q)")
	require.NotContains(t, out, "ent.QueryProposalMarketingFromQuery",
		"facade call must NOT use the literal 'ent.' prefix when the gen package is imported as 'gen'")
}

// TestRewriteEdgeMethod_HonoursExplicitImportAlias verifies a custom
// import alias is preserved: `import myent "..."` → `myent.Query...`.
func TestRewriteEdgeMethod_HonoursExplicitImportAlias(t *testing.T) {
	descs := Descriptors{
		"Proposal": &EntityDesc{
			Name:  "Proposal",
			Edges: map[string]EdgeDesc{"marketing": {Cardinality: "entbuilder.O2OUnique", TargetIDType: "uuid.UUID", Target: "Marketing"}},
		},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
import myent "github.com/example/svc/internal/ent/gen"
type ProposalQuery struct{}
type MarketingQuery struct{}
func (q *ProposalQuery) QueryMarketing() *MarketingQuery { return &MarketingQuery{} }
var _ = myent.Foo
func use(q *ProposalQuery) *MarketingQuery { return q.QueryMarketing() }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, "myent.QueryProposalMarketingFromQuery(q)")
	// Substring check must avoid matching "myent." — assert with a
	// space prefix so we only catch a bare "ent." selector.
	require.NotContains(t, out, " ent.QueryProposalMarketingFromQuery")
	require.NotContains(t, out, " gen.QueryProposalMarketingFromQuery")
}

// TestRewriteEdgeMethod_AddsMissingGenImport covers the edge case where a
// consumer file matches the rewrite pattern but doesn't yet import the
// gen package (e.g. a hand-written schema/hook file that previously only
// called methods on locally-defined types). The rewriter must add the
// import so the emitted call resolves at compile time.
func TestRewriteEdgeMethod_AddsMissingGenImport(t *testing.T) {
	descs := Descriptors{
		"Proposal": &EntityDesc{
			Name:  "Proposal",
			Edges: map[string]EdgeDesc{"marketing": {Cardinality: "entbuilder.O2OUnique", TargetIDType: "uuid.UUID", Target: "Marketing"}},
		},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
type ProposalQuery struct{}
type MarketingQuery struct{}
func (q *ProposalQuery) QueryMarketing() *MarketingQuery { return &MarketingQuery{} }
func use(q *ProposalQuery) *MarketingQuery { return q.QueryMarketing() }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.Contains(t, out, "gen.QueryProposalMarketingFromQuery(q)")
	require.Contains(t, out, `"github.com/example/svc/internal/ent/gen"`,
		"missing gen-package import must be added when a rewrite fires")
}

// TestRewriteEdgeMethod_NoImportAddedWhenNoRewrite verifies the
// counterpart safety: if no rewrite actually fires in a file, the gen
// import must NOT be added (else go build fails with imported and not used).
func TestRewriteEdgeMethod_NoImportAddedWhenNoRewrite(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	const genPath = "github.com/example/svc/internal/ent/gen"
	src := `package x
type Helper struct{}
func (h *Helper) QueryTeams() *Helper { return h }
func use(h *Helper) *Helper { return h.QueryTeams() }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, genPath)
	require.NoError(t, err)
	require.NotContains(t, out, genPath,
		"no rewrite emitted → no gen import should be added")
}

// TestRewriteEdgeMethod_WithEdge_Chained covers the cross-package shape:
// gen.FromContext(ctx).ChatterMessage.Query().WithThread(...).AllX(ctx)
// where go/types cannot resolve the receiver because the consumer file
// imports an external gen package. The syntactic chain walker must infer
// the *ChatterMessageQuery receiver from the .<Entity>.Query() prefix
// and rewrite to the With<Entity><Edge> facade variant.
func TestRewriteEdgeMethod_WithEdge_Chained(t *testing.T) {
	descs := Descriptors{
		"ChatterMessage": &EntityDesc{
			Name: "ChatterMessage",
			Edges: map[string]EdgeDesc{
				"thread": {Cardinality: "entbuilder.M2O", TargetIDType: "int", Target: "ChatterThread"},
			},
		},
		"ChatterThread": &EntityDesc{
			Name:  "ChatterThread",
			Edges: map[string]EdgeDesc{},
		},
	}
	src := `package x
import "external/gen"
import "context"
func use(ctx context.Context) {
    _ = gen.FromContext(ctx).ChatterMessage.Query().
        WithThread(func(q *gen.ChatterThreadQuery) {}).
        AllX(ctx)
}
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.WithChatterMessageThread(gen.FromContext(ctx).ChatterMessage.Query(),",
		"cross-package WithThread must be rewritten to WithChatterMessageThread facade")
	require.NotContains(t, out, ".WithThread(", "original WithThread call must not remain")
}

// TestRewriteEdgeMethod_WithEdge_AfterSelect covers the real consumer shape:
// q.Clone().Select(...).WithThread(...).All(ctx) where after .Select(...)
// the receiver becomes *<Entity>Select (not *<Entity>Query), which the chain
// walker must recognise as still belonging to the same entity.
func TestRewriteEdgeMethod_WithEdge_AfterSelect(t *testing.T) {
	descs := Descriptors{
		"ChatterMessage": &EntityDesc{
			Name: "ChatterMessage",
			Edges: map[string]EdgeDesc{
				"thread": {Cardinality: "entbuilder.M2O", TargetIDType: "int", Target: "ChatterThread"},
			},
		},
		"ChatterThread": &EntityDesc{
			Name:  "ChatterThread",
			Edges: map[string]EdgeDesc{},
		},
	}
	src := `package x
import "external/gen"
import "external/chattermessage"
import "context"
func use(ctx context.Context) {
    q := &gen.ChatterMessageQuery{}
    _, _ = q.Clone().
        Select(chattermessage.FieldID).
        WithThread(func(qq *gen.ChatterThreadQuery) {}).
        All(ctx)
}
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "ent.WithChatterMessageThread(q.Clone().",
		"WithThread after Select must be rewritten to WithChatterMessageThread facade with correct chain receiver")
	require.NotContains(t, out, ".WithThread(", "original WithThread call must not remain")
}

// TestRewriteEdgeMethod_WithEdge_AfterSelect_NoMatchOnUnknownEdge verifies
// that a .Select().WithFoo() chain where "foo" is NOT an edge on the
// inferred entity is NOT rewritten. This guards the new Select-handling
// path against false-positive matches on unrelated method names.
func TestRewriteEdgeMethod_WithEdge_AfterSelect_NoMatchOnUnknownEdge(t *testing.T) {
	descs := Descriptors{
		"ChatterMessage": &EntityDesc{
			Name: "ChatterMessage",
			Edges: map[string]EdgeDesc{
				"thread": {Cardinality: "entbuilder.M2O", TargetIDType: "int", Target: "ChatterThread"},
			},
		},
		"ChatterThread": &EntityDesc{
			Name:  "ChatterThread",
			Edges: map[string]EdgeDesc{},
		},
	}
	src := `package x
import "external/gen"
import "external/chattermessage"
import "context"
func use(ctx context.Context) {
    q := &gen.ChatterMessageQuery{}
    _, _ = q.Clone().
        Select(chattermessage.FieldID).
        WithUnknownEdge(func() {}).
        All(ctx)
}
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs, "")
	require.NoError(t, err)
	require.Contains(t, out, "WithUnknownEdge(",
		"unknown edge must NOT be rewritten")
	require.NotContains(t, out, "ent.WithChatterMessage",
		"no rewrite must fire for unknown edge")
}
