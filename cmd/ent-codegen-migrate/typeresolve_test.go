// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchesReceiverType_MutationType(t *testing.T) {
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) SetTitle(s string) {}
func hook(m *TaskMutation) { m.SetTitle("hi") }
`
	r, err := NewResolver("hook.go", src)
	require.NoError(t, err)

	call := r.FindFirstCall("SetTitle")
	require.NotNil(t, call)

	ok := r.MatchesReceiverType(call, "*x.TaskMutation")
	require.True(t, ok)
}

func TestMatchesReceiverType_SchemaDSLNotMutation(t *testing.T) {
	src := `package x
type EdgeBuilder struct{}
func (e *EdgeBuilder) Ref(name string) *EdgeBuilder { return e }
func From(name string) *EdgeBuilder { return &EdgeBuilder{} }
func use() { From("owner").Ref("tasks") }
`
	r, err := NewResolver("hook.go", src)
	require.NoError(t, err)

	call := r.FindFirstCall("Ref")
	require.NotNil(t, call)

	ok := r.MatchesReceiverType(call, "*x.TaskMutation")
	require.False(t, ok, "EdgeBuilder must not match TaskMutation")
}
