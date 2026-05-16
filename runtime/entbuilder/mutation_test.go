// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"reflect"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

// Fixture entity for unit tests.
type testEntity struct {
	ID    int
	Title string
}

func testDescriptor() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name:   "TestEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"title": {Type: reflect.TypeFor[string](), GoName: "Title"},
		},
		Edges: map[string]entbuilder.EdgeSpec{},
	}
}

func TestNewMutation_PopulatesDescriptorAndOp(t *testing.T) {
	desc := testDescriptor()
	m := entbuilder.NewMutation[testEntity](nil, ent.OpCreate, desc)
	require.NotNil(t, m)
}
