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
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
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
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
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
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
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
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
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
	pass1, err := RewriteEdgeMethodSource("x.go", src, descs)
	require.NoError(t, err)
	pass2, err := RewriteEdgeMethodSource("x.go", pass1, descs)
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
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, "o.WithTeams(", "non-Query receiver must be skipped")
	require.NotContains(t, out, "ent.WithTaskTeams", "no rewrite expected")
}
