// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"testing"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/entql"
	"entgo.io/ent/runtime/entbuilder"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

// userDesc/petDesc mirror a trimmed User/Pet pair (see
// entc/integration/ent/user/shared.go's pre-Task-2 schemaGraph literal): a
// single-field-ID User with an O2M "pets" edge to a single-field-ID Pet.
func userDesc() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name: "User", Table: "users", IDColumn: "id", IDSQLType: field.TypeInt,
		TableColumns: []string{"id", "name", "age"},
		GraphFields: map[string]field.Type{
			"name": field.TypeString,
			"age":  field.TypeInt,
		},
		GraphEdges: map[string]entbuilder.EdgeSpec{
			"pets": {Target: "Pet", Rel: sqlgraph.O2M, StorageTable: "pets", StorageColumns: []string{"pet_owner"}},
		},
	}
}

func petDesc() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name: "Pet", Table: "pets", IDColumn: "id", IDSQLType: field.TypeInt,
		TableColumns: []string{"id", "name"},
		GraphFields: map[string]field.Type{
			"name": field.TypeString,
		},
	}
}

func TestBuildSchemaGraph_Nodes(t *testing.T) {
	graph := entbuilder.BuildSchemaGraph([]*entbuilder.Descriptor{userDesc(), petDesc()})
	require.Len(t, graph.Nodes, 2)

	ui := entbuilder.NodeIndex(graph, "User")
	require.NotEqual(t, -1, ui)
	un := graph.Nodes[ui]
	require.Equal(t, "users", un.Table)
	require.Equal(t, []string{"id", "name", "age"}, un.Columns)
	require.NotNil(t, un.ID)
	require.Equal(t, field.TypeInt, un.ID.Type)
	require.Equal(t, "id", un.ID.Column)
	require.Nil(t, un.CompositeID)
	require.Len(t, un.Fields, 2)
	require.Equal(t, &sqlgraph.FieldSpec{Type: field.TypeString, Column: "name"}, un.Fields["name"])
	require.Equal(t, &sqlgraph.FieldSpec{Type: field.TypeInt, Column: "age"}, un.Fields["age"])

	pi := entbuilder.NodeIndex(graph, "Pet")
	require.NotEqual(t, -1, pi)
	require.Equal(t, "pets", graph.Nodes[pi].Table)

	require.Equal(t, -1, entbuilder.NodeIndex(graph, "Missing"))
}

// TestBuildSchemaGraph_Edge exercises the same path entql's generated
// Filter.Where uses (schemaGraph.EvalP -> evalEdge for entql.FuncHasEdge),
// confirming MustAddE wired the "pets" edge onto the User node so
// has_edge(pets) resolves without panicking.
func TestBuildSchemaGraph_Edge(t *testing.T) {
	graph := entbuilder.BuildSchemaGraph([]*entbuilder.Descriptor{userDesc(), petDesc()})

	sel := sql.Dialect("sqlite3").Select().From(sql.Table("users"))
	p := entql.HasEdge("pets")
	require.NoError(t, graph.EvalP("User", p, sel))

	query, _ := sel.Query()
	require.Contains(t, query, "EXISTS")
	require.Contains(t, query, "`pets`")
}

func TestBuildSchemaGraph_CompositeID(t *testing.T) {
	desc := &entbuilder.Descriptor{
		Name: "TweetLike", Table: "tweet_likes",
		TableColumns: []string{"liked_at", "user_id", "tweet_id"},
		CompositeID: []entbuilder.IDColumnSpec{
			{Column: "user_id", SQLType: field.TypeInt},
			{Column: "tweet_id", SQLType: field.TypeInt},
		},
		GraphFields: map[string]field.Type{
			"liked_at": field.TypeTime,
			"user_id":  field.TypeInt,
			"tweet_id": field.TypeInt,
		},
	}
	graph := entbuilder.BuildSchemaGraph([]*entbuilder.Descriptor{desc})
	n := graph.Nodes[0]
	require.Nil(t, n.ID)
	require.Len(t, n.CompositeID, 2)
	require.Equal(t, &sqlgraph.FieldSpec{Type: field.TypeInt, Column: "user_id"}, n.CompositeID[0])
	require.Equal(t, &sqlgraph.FieldSpec{Type: field.TypeInt, Column: "tweet_id"}, n.CompositeID[1])
}
