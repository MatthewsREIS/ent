// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"entgo.io/ent/entc/integration/ent"
	"entgo.io/ent/entc/integration/ent/pet"
	"entgo.io/ent/entc/integration/ent/user"
	"entgo.io/ent/entql"

	"github.com/stretchr/testify/require"
)

func EntQL(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	a8m := client.User.Create().SetName("a8m").SetAge(30).SaveX(ctx)
	nati := client.User.Create().SetName("nati").SetAge(30).AddFriendIDs(a8m.ID).SaveX(ctx)

	uq := client.User.Query()
	uq.Filter().Where(entql.HasEdge("friends"))
	require.Equal(2, uq.CountX(ctx))

	uq = client.User.Query()
	uq.Filter().Where(
		entql.And(
			entql.FieldEQ("name", "nati"),
			entql.HasEdge("friends"),
		),
	)
	require.Equal(nati.ID, uq.OnlyIDX(ctx))

	u1, u2 := uuid.New(), uuid.New()
	xabi := client.Pet.Create().SetName("xabi").SetOwnerID(a8m.ID).SetUUID(u1).SaveX(ctx)
	luna := client.Pet.Create().SetName("luna").SetOwnerID(nati.ID).SetUUID(u2).SaveX(ctx)
	uq = client.User.Query()
	uq.Filter().Where(
		entql.And(
			entql.HasEdge("pets"),
			entql.HasEdgeWith("friends", entql.FieldEQ("name", "nati")),
			entql.HasEdgeWith("friends", entql.FieldIn("name", "nati")),
			entql.HasEdgeWith("friends", entql.FieldIn("name", "nati", "a8m")),
		),
	)
	require.Equal(a8m.ID, uq.OnlyIDX(ctx))
	uq = client.User.Query()
	uq.Filter().Where(
		entql.And(
			entql.HasEdgeWith("pets", entql.FieldEQ("name", "luna")),
			entql.HasEdge("friends"),
		),
	)
	require.Equal(nati.ID, uq.OnlyIDX(ctx))

	pq := client.Pet.Query()
	pq.Filter().WhereUUID(entql.ValueEQ(u1))
	require.Equal(xabi.ID, pq.OnlyIDX(ctx))
	pq = client.Pet.Query()
	pq.Filter().WhereUUID(entql.ValueEQ(u2))
	require.Equal(luna.ID, pq.OnlyIDX(ctx))

	uq = client.User.Query()
	uq.Filter().WhereName(entql.StringEQ("a8m"))
	require.Equal(a8m.ID, uq.OnlyIDX(ctx))
	pq = client.Pet.Query()
	pq.Filter().WhereName(entql.StringOr(entql.StringEQ("xabi"), entql.StringEQ("luna")))
	require.Equal([]int{luna.ID, xabi.ID}, pq.Order(ent.Asc(pet.FieldName)).IDsX(ctx))

	pq = client.Pet.Query()
	pq.Where(pet.Name(luna.Name)).Filter().WhereID(entql.IntEQ(luna.ID))
	require.Equal(luna.ID, pq.Order(ent.Asc(pet.FieldName)).OnlyIDX(ctx))
	pq = client.Pet.Query()
	pq.Where(pet.Name(luna.Name)).Filter().WhereID(entql.IntEQ(xabi.ID))
	require.False(pq.ExistX(ctx))

	update := client.User.Update().SetRole(user.RoleAdmin).Where(user.Name(a8m.Name))
	updated := update.SaveX(ctx)
	require.Equal(1, updated)
	uq = client.User.Query()
	uq.Filter().WhereRole(entql.StringEQ(string(user.RoleAdmin)))
	require.Equal(a8m.ID, uq.OnlyIDX(ctx))

	uq = client.User.Query()
	uq.Filter().WhereName(entql.StringEQ(a8m.Name))
	uq = uq.QueryFriends()
	uq.Filter().WhereName(entql.StringEQ(nati.Name))
	require.Equal(luna.ID, uq.QueryPets().OnlyIDX(ctx))
}
