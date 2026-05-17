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
