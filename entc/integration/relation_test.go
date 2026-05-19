// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/entc/integration/ent"
	"entgo.io/ent/entc/integration/ent/card"
	"entgo.io/ent/entc/integration/ent/group"
	"entgo.io/ent/entc/integration/ent/node"
	"entgo.io/ent/entc/integration/ent/pet"
	"entgo.io/ent/entc/integration/ent/user"

	"github.com/stretchr/testify/require"
)

// Demonstrate a O2O relation between two different types. A User and a CreditCard.
// The user is the owner of the edge, named "owner", and the card has an inverse edge
// named "owner" that points to the User.
func O2OTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without card")
	usr := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.Zero(ent.QueryUserCard(client.User, usr).CountX(ctx))

	t.Log("add card to user on card creation (inverse creation)")
	crd := client.Card.Create().SetNumber("1").SetOwnerID(usr.ID).SaveX(ctx)
	require.Equal(ent.QueryUserCard(client.User, usr).CountX(ctx), 1)
	require.Equal(ent.QueryCardOwner(client.Card, crd).CountX(ctx), 1)

	t.Log("delete inverse should delete association")
	client.Card.DeleteOne(crd).ExecX(ctx)
	require.Zero(client.Card.Query().CountX(ctx))
	require.Zero(ent.QueryUserCard(client.User, usr).CountX(ctx), "user should not have card")

	t.Log("add card to user by updating user (the owner of the edge)")
	crd = client.Card.Create().SetNumber("10").SaveX(ctx)
	client.User.UpdateOne(usr).SetCardID(crd.ID).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryCardOwner(client.Card, crd).OnlyX(ctx).Name)
	require.Equal(crd.Number, ent.QueryUserCard(client.User, usr).OnlyX(ctx).Number)

	t.Log("delete assoc should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.Zero(ent.QueryCardOwner(client.Card, crd).CountX(ctx), "card should not have an owner")

	t.Log("add card to user by updating card (the inverse edge)")
	usr = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.Card.UpdateOne(crd).SetOwnerID(usr.ID).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryCardOwner(client.Card, crd).OnlyX(ctx).Name)
	require.Equal(crd.Number, ent.QueryUserCard(client.User, usr).OnlyX(ctx).Number)

	t.Log("query with side lookup on inverse")
	ocrd := client.Card.Create().SetNumber("orphan card").SaveX(ctx)
	require.Equal(crd.Number, client.Card.Query().Where(card.HasOwner()).OnlyX(ctx).Number)
	require.Equal(ocrd.Number, client.Card.Query().Where(card.Not(card.HasOwner())).OnlyX(ctx).Number)

	t.Log("query with side lookup on assoc")
	ousr := client.User.Create().SetAge(10).SetName("user without card").SaveX(ctx)
	require.Equal(usr.Name, client.User.Query().Where(user.HasCard()).OnlyX(ctx).Name)
	require.Equal(ousr.Name, client.User.Query().Where(user.Not(user.HasCard())).OnlyX(ctx).Name)

	t.Log("query with side lookup condition on inverse")
	require.Equal(crd.Number, client.Card.Query().Where(card.HasOwnerWith(user.Name(usr.Name))).OnlyX(ctx).Number)
	// has owner, but with name != "bar".
	require.Zero(client.Card.Query().Where(card.HasOwnerWith(user.Not(user.Name(usr.Name)))).CountX(ctx))
	// either has no owner, or has owner with name != "bar".
	require.Equal(
		ocrd.Number,
		client.Card.Query().
			Where(
				card.Or(
					// has no owner.
					card.Not(card.HasOwner()),
					// has owner with name != "bar".
					card.HasOwnerWith(user.Not(user.Name(usr.Name))),
				),
			).
			OnlyX(ctx).Number,
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(usr.Name, client.User.Query().Where(user.HasCardWith(card.Number(crd.Number))).OnlyX(ctx).Name)
	require.Zero(client.User.Query().Where(user.HasCardWith(card.Not(card.Number(crd.Number)))).CountX(ctx))
	// either has no card, or has card with number != "10".
	require.Equal(
		ousr.Name,
		client.User.Query().
			Where(
				user.Or(
					// has no card.
					user.Not(user.HasCard()),
					// has card with number != "10".
					user.HasCardWith(card.Not(card.Number(crd.Number))),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query long path from inverse")
	require.Equal(crd.Number, ent.QueryUserCardFromQuery(ent.QueryCardOwner(client.Card, crd)).OnlyX(ctx).Number, "should get itself")
	require.Equal(usr.Name, ent.QueryCardOwnerFromQuery(ent.QueryUserCardFromQuery(ent.QueryCardOwner(client.Card, crd))).OnlyX(ctx).Name, "should get its owner")
	require.Equal(
		usr.Name,
		ent.QueryCardOwnerFromQuery(
			ent.QueryUserCardFromQuery(
				ent.QueryCardOwner(client.Card, crd).
					Where(user.HasCard()),
			),
		).
			Where(user.HasCard()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(usr.Name, ent.QueryCardOwnerFromQuery(ent.QueryUserCard(client.User, usr)).OnlyX(ctx).Name, "should get itself")
	require.Equal(crd.Number, ent.QueryUserCardFromQuery(ent.QueryCardOwnerFromQuery(ent.QueryUserCard(client.User, usr))).OnlyX(ctx).Number, "should get its card")
	require.Equal(
		crd.Number,
		ent.QueryUserCardFromQuery(
			ent.QueryCardOwnerFromQuery(
				ent.QueryUserCard(client.User, usr).
					Where(card.HasOwner()),
			).
				Where(user.HasCard()),
		).
			OnlyX(ctx).Number,
		"should get its card",
	)
}

// Demonstrate a O2O relation between two instances of the same type. A linked-list
// nodes, where each node has an edge named "next" with inverse named "prev".
func O2OSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("head of the list")
	head := client.Node.Create().SetValue(1).SaveX(ctx)
	require.Zero(ent.QueryNodePrev(client.Node, head).CountX(ctx))
	require.Zero(ent.QueryNodeNext(client.Node, head).CountX(ctx))

	t.Log("add node to the linked-list and connect it to the head (inverse creation)")
	sec := client.Node.Create().SetValue(2).SetPrevID(head.ID).SaveX(ctx)
	require.Zero(ent.QueryNodeNext(client.Node, sec).CountX(ctx), "should not have next")
	require.Equal(head.ID, ent.QueryNodePrev(client.Node, sec).OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, ent.QueryNodeNext(client.Node, head).OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete inverse should delete association")
	client.Node.DeleteOne(sec).ExecX(ctx)
	require.Zero(ent.QueryNodeNext(client.Node, head).CountX(ctx))
	require.Equal(1, client.Node.Query().CountX(ctx), "linked-list should have 1 node")

	t.Log("add node to the linked-list by updating the head (the owner of the edge)")
	sec = client.Node.Create().SetValue(2).SaveX(ctx)
	client.Node.UpdateOne(head).SetNextID(sec.ID).ExecX(ctx)
	require.Zero(ent.QueryNodeNext(client.Node, sec).CountX(ctx), "should not have next")
	require.Equal(head.ID, ent.QueryNodePrev(client.Node, sec).OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, ent.QueryNodeNext(client.Node, head).OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete assoc should delete inverse edge")
	client.Node.DeleteOne(head).ExecX(ctx)
	require.Zero(ent.QueryNodePrev(client.Node, sec).CountX(ctx), "second node should be the head now")
	require.Zero(ent.QueryNodeNext(client.Node, sec).CountX(ctx), "second node should be the head now")

	t.Log("update second node value to be 1")
	head = client.Node.UpdateOne(sec).SetValue(1).SaveX(ctx)
	require.Equal(1, head.Value)

	t.Log("create a linked-list 1->2->3->4->5")
	nodes := []*ent.Node{head}
	for i := 0; i < 4; i++ {
		next := client.Node.Create().SetValue(nodes[i].Value + 1).SetPrevID(nodes[i].ID).SaveX(ctx)
		nodes = append(nodes, next)
	}
	require.Equal(len(nodes), client.Node.Query().CountX(ctx))

	t.Log("check correctness of the list values")
	for i, n := range nodes[:3] {
		require.Equal(i+1, n.Value)
		require.Equal(nodes[i+1].Value, ent.QueryNodeNext(client.Node, n).OnlyX(ctx).Value)
	}
	require.Zero(ent.QueryNodeNext(client.Node, nodes[len(nodes)-1]).CountX(ctx), "last node should point to nil")

	t.Log("query with side lookup on inverse/assoc")
	require.Equal(4, client.Node.Query().Where(node.HasNext()).CountX(ctx))
	require.Equal(4, client.Node.Query().Where(node.HasPrev()).CountX(ctx))

	t.Log("make the linked-list to be circular")
	client.Node.UpdateOne(nodes[len(nodes)-1]).SetNextID(head.ID).SaveX(ctx)
	require.Equal(nodes[0].Value, ent.QueryNodeNext(client.Node, nodes[len(nodes)-1]).OnlyX(ctx).Value, "last node should point to head")
	require.Equal(nodes[len(nodes)-1].Value, ent.QueryNodePrev(client.Node, nodes[0]).OnlyX(ctx).Value, "head should have a reference to the tail")

	t.Log("query with side lookup on inverse/assoc")
	require.Equal(5, client.Node.Query().Where(node.HasNext()).CountX(ctx))
	require.Equal(5, client.Node.Query().Where(node.HasPrev()).CountX(ctx))
	// node that points (with "next") to other node with value 2 (the head).
	require.Equal(nodes[0].Value, client.Node.Query().Where(node.HasNextWith(node.ValueEQ(2))).OnlyX(ctx).Value)
	// node that points (with "next") to other node with value 1 (the tail).
	require.Equal(nodes[len(nodes)-1].Value, client.Node.Query().Where(node.HasNextWith(node.ValueEQ(1))).OnlyX(ctx).Value)
	// nodes that points to nodes with value greater than 2 (X->2->3->4->X).
	values, err := client.Node.Query().
		Where(node.HasNextWith(node.ValueGT(2))).
		Order(ent.Asc(node.FieldValue)).
		GroupBy(node.FieldValue).
		Ints(ctx)
	require.NoError(err)
	require.Equal([]int{2, 3, 4}, values)

	t.Log("query long path from inverse")
	// going back from head to tail until we reach the head.
	require.Equal(
		head.Value,
		ent.QueryNodePrevFromQuery( // 1 (head)
			ent.QueryNodePrevFromQuery( // 2
				ent.QueryNodePrevFromQuery( // 3
					ent.QueryNodePrevFromQuery( // 4
						ent.QueryNodePrev(client.Node, head), // 5 (tail)
					),
				),
			),
		).
			OnlyX(ctx).Value,
	)
	// disrupt the query in the middle.
	require.Zero(ent.QueryNodePrevFromQuery(ent.QueryNodePrevFromQuery(ent.QueryNodePrevFromQuery(ent.QueryNodePrevFromQuery(ent.QueryNodePrev(client.Node, head)).Where(node.ValueGT(10))))).CountX(ctx))

	t.Log("query long path from assoc")
	// going forward from head to next until we reach the head.
	require.Equal(
		head.Value,
		ent.QueryNodeNextFromQuery( // 1 (head)
			ent.QueryNodeNextFromQuery( // 5 (tail)
				ent.QueryNodeNextFromQuery( // 4
					ent.QueryNodeNextFromQuery( // 3
						ent.QueryNodeNext(client.Node, head), // 2
					),
				),
			),
		).
			OnlyX(ctx).Value,
	)
	// disrupt the query in the middle.
	require.Zero(ent.QueryNodeNextFromQuery(ent.QueryNodeNextFromQuery(ent.QueryNodeNextFromQuery(ent.QueryNodeNextFromQuery(ent.QueryNodeNext(client.Node, head)).Where(node.ValueGT(10))))).CountX(ctx))

	t.Log("delete all nodes except the head")
	client.Node.Delete().Where(node.ValueGT(1)).ExecX(ctx)
	head = client.Node.Query().OnlyX(ctx)

	t.Log("node points to itself (circular linked-list with 1 node)")
	head = client.Node.UpdateOne(head).SetNextID(head.ID).SaveX(ctx)
	require.Equal(head.ID, ent.QueryNodePrev(client.Node, head).OnlyIDX(ctx))
	require.Equal(head.ID, ent.QueryNodeNext(client.Node, head).OnlyIDX(ctx))
	head = client.Node.UpdateOne(head).ClearNext().SaveX(ctx)
	require.Zero(ent.QueryNodePrev(client.Node, head).CountX(ctx))
	require.Zero(ent.QueryNodeNext(client.Node, head).CountX(ctx))
}

// Demonstrate a O2O relation between two instances of the same type, where the relation
// has the same name in both directions. A couple. User A has "spouse" B (and vice versa).
// When setting B as a spouse of A, this sets A as spouse of B as well. In other words:
//
//	foo := client.User.Create().SetName("foo").SaveX(ctx)
//	bar := client.User.Create().SetName("bar").SetSpouse(foo).SaveX(ctx)
//	count := client.User.Query.Where(user.HasSpouse()).CountX(ctx)
//	// count will be 2, even though we've created only one relation above.
func O2OSelfRef(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without spouse")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))

	t.Log("sets spouse on user creation (inverse creation)")
	bar := client.User.Create().SetAge(10).SetName("bar").SetSpouseID(foo.ID).SaveX(ctx)
	require.True(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("add spouse to user by updating a user")
	bar = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.User.UpdateOne(foo).SetSpouseID(bar.ID).ExecX(ctx)
	require.True(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("remove a spouse using update")
	client.User.UpdateOne(foo).ClearSpouse().ExecX(ctx)
	require.False(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasSpouse()).CountX(ctx))
	// return back the spouse.
	client.User.UpdateOne(foo).SetSpouseID(bar.ID).ExecX(ctx)

	t.Log("create a user without spouse")
	baz := client.User.Create().SetAge(10).SetName("baz").SaveX(ctx)
	require.False(ent.QueryUserSpouse(client.User, baz).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("set a new spouse")
	client.User.UpdateOne(foo).ClearSpouse().SetSpouseID(baz.ID).ExecX(ctx)
	require.True(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserSpouse(client.User, baz).ExistX(ctx))
	require.False(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	// return back the spouse.
	client.User.UpdateOne(foo).ClearSpouse().SetSpouseID(bar.ID).ExecX(ctx)

	t.Log("spouse is a unique edge")
	require.Error(client.User.UpdateOne(baz).SetSpouseID(bar.ID).Exec(ctx))
	require.Error(client.User.UpdateOne(baz).SetSpouseID(foo.ID).Exec(ctx))

	t.Log("query with side lookup")
	require.Equal(
		bar.Name,
		client.User.Query().
			Where(user.HasSpouseWith(user.Name("foo"))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.HasSpouseWith(user.Name("bar"))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		baz.Name,
		client.User.Query().
			Where(user.Not(user.HasSpouse())).
			OnlyX(ctx).Name,
	)
	// has spouse that has a spouse with name "foo" (which actually means itself).
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.HasSpouseWith(user.HasSpouseWith(user.Name("foo")))).
			OnlyX(ctx).Name,
	)
	// has spouse that has a spouse with name "bar" (which actually means itself).
	require.Equal(
		bar.Name,
		client.User.Query().
			Where(user.HasSpouseWith(user.HasSpouseWith(user.Name("bar")))).
			OnlyX(ctx).Name,
	)

	t.Log("query path from a user")
	require.Equal(
		foo.Name,
		ent.QueryUserSpouseFromQuery( // foo
			ent.QueryUserSpouseFromQuery( // bar
				ent.QueryUserSpouseFromQuery( // foo
					ent.QueryUserSpouse(client.User, foo), // bar
				),
			),
		).
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		ent.QueryUserSpouseFromQuery( // bar
			ent.QueryUserSpouseFromQuery( // foo
				ent.QueryUserSpouseFromQuery( // bar
					ent.QueryUserSpouse(client.User, bar), // foo
				),
			),
		).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		bar.Name,
		ent.QueryUserSpouseFromQuery( // bar
			client.User.
				Query().
				Where(user.Name("foo")), // foo
		).
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		ent.QueryUserSpouseFromQuery( // bar
			ent.QueryUserSpouseFromQuery( // foo
				client.User.
					Query().
					Where(user.Name("bar")), // bar
			),
		).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a O2M/M2O relation between two different types. A User and its Pets.
// The User type is the "owner" of the edge (assoc), and the Pet as an inverse edge to
// its owner. User can have one or more Pets, and Pet have only one owner (not required).
func O2MTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without pet")
	usr := client.User.Create().SetAge(30).SetName("a8m").SaveX(ctx)
	require.False(ent.QueryUserPets(client.User, usr).ExistX(ctx))

	t.Log("add pet to user on pet creation (inverse creation)")
	pedro := client.Pet.Create().SetName("pedro").SetOwnerID(usr.ID).SaveX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwner(client.Pet, pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, ent.QueryUserPets(client.User, usr).OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.Pet.DeleteOne(pedro).ExecX(ctx)
	require.Zero(client.Pet.Query().CountX(ctx))
	require.False(ent.QueryUserPets(client.User, usr).ExistX(ctx), "user should not have pet")

	t.Log("add pet to user by updating user (the owner of the edge)")
	pedro = client.Pet.Create().SetName("pedro").SaveX(ctx)
	client.User.UpdateOne(usr).AddPetIDs(pedro.ID).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwner(client.Pet, pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, ent.QueryUserPets(client.User, usr).OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.False(ent.QueryPetOwner(client.Pet, pedro).ExistX(ctx), "pet should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	usr = client.User.Create().SetAge(30).SetName("a8m").SaveX(ctx)
	client.Pet.UpdateOne(pedro).SetOwnerID(usr.ID).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwner(client.Pet, pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, ent.QueryUserPets(client.User, usr).OnlyX(ctx).Name)

	t.Log("add another pet to user")
	xabi := client.Pet.Create().SetName("xabi").SetOwnerID(usr.ID).SaveX(ctx)
	require.Equal(2, ent.QueryUserPets(client.User, usr).CountX(ctx))
	require.Equal(1, ent.QueryPetOwner(client.Pet, xabi).CountX(ctx))
	require.Equal(1, ent.QueryPetOwner(client.Pet, pedro).CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().SetAge(30).SetName("alex").AddPetIDs(pedro.ID).Save(ctx)
	require.Error(err, "pet already has an owner")

	t.Log("add multiple pets on creation")
	p1 := client.Pet.Create().SetName("p1").SaveX(ctx)
	p2 := client.Pet.Create().SetName("p2").SaveX(ctx)
	usr2 := client.User.Create().SetAge(30).SetName("alex").AddPetIDs(p1.ID, p2.ID).SaveX(ctx)
	require.True(ent.QueryPetOwner(client.Pet, p1).ExistX(ctx))
	require.True(ent.QueryPetOwner(client.Pet, p2).ExistX(ctx))
	require.Equal(2, ent.QueryUserPets(client.User, usr2).CountX(ctx))
	// delete p1, p2.
	client.Pet.Delete().Where(pet.IDIn(p1.ID, p2.ID)).ExecX(ctx)
	require.Zero(ent.QueryUserPets(client.User, usr2).CountX(ctx))

	t.Log("change the owner a pet")
	client.Pet.UpdateOne(xabi).ClearOwner().SetOwnerID(usr2.ID).ExecX(ctx)
	require.Equal(1, ent.QueryUserPets(client.User, usr).CountX(ctx))
	require.Equal(1, ent.QueryUserPets(client.User, usr2).CountX(ctx))
	require.Equal(usr2.Name, ent.QueryPetOwner(client.Pet, xabi).OnlyX(ctx).Name)

	t.Log("query with side lookup on inverse")
	opet := client.Pet.Create().SetName("orphan pet").SaveX(ctx)
	require.Equal(opet.Name, client.Pet.Query().Where(pet.Not(pet.HasOwner())).OnlyX(ctx).Name)
	require.Equal(2, client.Pet.Query().Where(pet.HasOwner()).CountX(ctx))

	t.Log("query with side lookup on assoc")
	require.Zero(client.User.Query().Where(user.Not(user.HasPets())).CountX(ctx))
	ousr := client.User.Create().SetAge(10).SetName("user without pet").SaveX(ctx)
	require.Equal(2, client.User.Query().Where(user.HasPets()).CountX(ctx))
	require.Equal(ousr.Name, client.User.Query().Where(user.Not(user.HasPets())).OnlyX(ctx).Name)

	t.Log("query with side lookup condition on inverse")
	require.Equal(pedro.Name, client.Pet.Query().Where(pet.HasOwnerWith(user.Name(usr.Name))).OnlyX(ctx).Name)
	// has owner, but with name != "a8m".
	require.Equal(xabi.Name, client.Pet.Query().Where(pet.HasOwnerWith(user.Not(user.Name(usr.Name)))).OnlyX(ctx).Name)
	// either has no owner, or has owner with name != "alex" and name != "a8m".
	require.Equal(
		opet.Name,
		client.Pet.Query().
			Where(
				pet.Or(
					// has no owner.
					pet.Not(pet.HasOwner()),
					// has owner with name != "a8m" and name != "alex".
					pet.HasOwnerWith(
						user.Not(user.Name(usr.Name)),
						user.Not(user.Name(usr2.Name)),
					),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(usr.Name, client.User.Query().Where(user.HasPetsWith(pet.Name(pedro.Name))).OnlyX(ctx).Name)
	require.Equal(usr2.Name, client.User.Query().Where(user.HasPetsWith(pet.Name(xabi.Name))).OnlyX(ctx).Name)
	require.Zero(
		client.User.Query().
			Where(
				user.HasPetsWith(
					pet.Not(pet.Name(xabi.Name)),
					pet.Not(pet.Name(pedro.Name)),
				),
			).CountX(ctx),
	)
	// either has no pet, or has pet with name != "pedro" and name != "xabi".
	require.Equal(
		ousr.Name,
		client.User.Query().
			Where(
				user.Or(
					// has no pet.
					user.Not(user.HasPets()),
					// has pet with name != "pedro" and name != "xabi".
					user.HasPetsWith(
						pet.Not(pet.Name(xabi.Name)),
						pet.Not(pet.Name(pedro.Name)),
					),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query long path from inverse")
	require.Equal(pedro.Name, ent.QueryUserPetsFromQuery(ent.QueryPetOwner(client.Pet, pedro)).OnlyX(ctx).Name, "should get itself")
	require.Equal(usr.Name, ent.QueryPetOwnerFromQuery(ent.QueryUserPetsFromQuery(ent.QueryPetOwner(client.Pet, pedro))).OnlyX(ctx).Name, "should get its owner")
	require.Equal(
		usr.Name,
		ent.QueryPetOwnerFromQuery(
			ent.QueryUserPetsFromQuery(
				ent.QueryPetOwner(client.Pet, pedro).
					Where(user.HasPets()),
			),
		).
			Where(user.HasPets()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(usr.Name, ent.QueryPetOwnerFromQuery(ent.QueryUserPets(client.User, usr)).OnlyX(ctx).Name, "should get itself")
	require.Equal(pedro.Name, ent.QueryUserPetsFromQuery(ent.QueryPetOwnerFromQuery(ent.QueryUserPets(client.User, usr))).OnlyX(ctx).Name, "should get its pet")
	require.Equal(
		pedro.Name,
		ent.QueryUserPetsFromQuery(
			ent.QueryPetOwnerFromQuery(
				ent.QueryUserPets(client.User, usr).
					Where(pet.HasOwner()), // pedro
			).
				Where(user.HasPets()), // a8m
		). // pedro
			OnlyX(ctx).Name,
		"should get its pet",
	)
	require.Equal(
		xabi.Name,
		ent.QueryUserPetsFromQuery( // xabi
			ent.QueryPetOwnerFromQuery( // alex
				ent.QueryUserPetsFromQuery( // xabi
					client.User.Query().
						// alex matches this query (not a8m, and have a pet).
						Where(
							user.Not(user.Name(usr.Name)),
							user.HasPets(),
						),
				),
			),
		).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a O2M/M2O relation between two instances of the same type. A "parent" and
// its children. User can have one or more children, but can have only one parent (unique inverse edge).
// Note that both edges are not required.
func O2MSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new parent without children")
	prt := client.User.Create().SetAge(30).SetName("a8m").SaveX(ctx)
	require.Zero(ent.QueryUserChildren(client.User, prt).CountX(ctx))

	t.Log("add child to parent on child creation (inverse creation)")
	chd := client.User.Create().SetAge(1).SetName("child").SetParentID(prt.ID).SaveX(ctx)
	require.Equal(prt.Name, ent.QueryUserParent(client.User, chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, ent.QueryUserChildren(client.User, prt).OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(chd).ExecX(ctx)
	require.False(ent.QueryUserChildren(client.User, prt).ExistX(ctx), "user should not have children")

	t.Log("add child to parent by updating user (the owner of the edge)")
	chd = client.User.Create().SetAge(1).SetName("child").SaveX(ctx)
	client.User.UpdateOne(prt).AddChildIDs(chd.ID).ExecX(ctx)
	require.Equal(prt.Name, ent.QueryUserParent(client.User, chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, ent.QueryUserChildren(client.User, prt).OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(prt).ExecX(ctx)
	require.Equal(1, client.User.Query().CountX(ctx))
	require.False(ent.QueryUserParent(client.User, chd).ExistX(ctx), "child should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	prt = client.User.Create().SetAge(30).SetName("a8m").SaveX(ctx)
	client.User.UpdateOne(chd).SetParentID(prt.ID).ExecX(ctx)
	require.Equal(prt.Name, ent.QueryUserParent(client.User, chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, ent.QueryUserChildren(client.User, prt).OnlyX(ctx).Name)
	require.Zero(ent.QueryUserParent(client.User, prt).CountX(ctx), "parent is orphan")
	require.Zero(ent.QueryUserChildren(client.User, chd).CountX(ctx), "child should not have children")

	t.Log("add another pet to user")
	chd2 := client.User.Create().SetAge(1).SetName("child2").SetParentID(prt.ID).SaveX(ctx)
	require.Equal(2, ent.QueryUserChildren(client.User, prt).CountX(ctx))
	require.Equal(1, ent.QueryUserParent(client.User, chd).CountX(ctx))
	require.Equal(1, ent.QueryUserParent(client.User, chd2).CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().SetAge(30).SetName("alex").AddChildIDs(chd.ID).Save(ctx)
	require.Error(err, "child already has parent")
	_, err = client.User.Create().SetAge(30).SetName("alex").AddChildIDs(chd2.ID).Save(ctx)
	require.Error(err, "child already has parent")

	t.Log("add multiple child on creation")
	chd3 := client.User.Create().SetAge(1).SetName("child3").SaveX(ctx)
	chd4 := client.User.Create().SetAge(1).SetName("child4").SaveX(ctx)
	prt2 := client.User.Create().SetAge(30).SetName("alex").AddChildIDs(chd3.ID, chd4.ID).SaveX(ctx)
	require.True(ent.QueryUserParent(client.User, chd3).ExistX(ctx))
	require.True(ent.QueryUserParent(client.User, chd3).ExistX(ctx))
	require.Equal(2, ent.QueryUserChildren(client.User, prt2).CountX(ctx))
	// delete chd3, chd4.
	client.User.Delete().Where(user.IDIn(chd3.ID, chd4.ID)).ExecX(ctx)
	require.Zero(ent.QueryUserChildren(client.User, prt2).CountX(ctx))

	t.Log("change the parent a child")
	client.User.UpdateOne(chd2).ClearParent().SetParentID(prt2.ID).ExecX(ctx)
	require.Equal(1, ent.QueryUserChildren(client.User, prt).CountX(ctx))
	require.Equal(1, ent.QueryUserChildren(client.User, prt2).CountX(ctx))
	require.Equal(chd2.Name, ent.QueryUserChildren(client.User, prt2).OnlyX(ctx).Name)

	t.Log("query with side lookup on inverse")
	ochd := client.User.Create().SetAge(1).SetName("orphan user").SaveX(ctx)
	require.Equal(3, client.User.Query().Where(user.Not(user.HasParent())).CountX(ctx))
	require.Equal(
		ochd.Name,
		client.User.Query().
			Where(
				user.Not(user.HasParent()),
				user.Not(user.HasChildren()),
			).
			OnlyX(ctx).Name,
		"3 orphan users, but only one does not have children",
	)
	require.Equal(2, client.User.Query().Where(user.HasParent()).CountX(ctx))

	t.Log("query with side lookup on assoc")
	require.Equal(2, client.User.Query().Where(user.HasChildren()).CountX(ctx))
	require.Equal(3, client.User.Query().Where(user.Not(user.HasChildren())).CountX(ctx))

	t.Log("query with side lookup condition on inverse")
	require.Equal(chd.Name, client.User.Query().Where(user.HasParentWith(user.Name(prt.Name))).OnlyX(ctx).Name)
	// has parent, but with name != "a8m".
	require.Equal(chd2.Name, client.User.Query().Where(user.HasParentWith(user.Not(user.Name(prt.Name)))).OnlyX(ctx).Name)
	// either has no parent, or has parent with name != "alex".
	require.Equal(
		4,
		client.User.Query().
			Where(
				user.Or(
					// has no parent.
					user.Not(user.HasParent()),
					// has parent with name != "alex".
					user.HasParentWith(
						user.Not(user.Name(prt2.Name)),
					),
				),
			).
			CountX(ctx),
		"should match chd, ochd, prt, prt2",
	)
	// either has no parent, or has parent with name != "a8m".
	require.Equal(
		4,
		client.User.Query().
			Where(
				user.Or(
					// has no parent.
					user.Not(user.HasParent()),
					// has parent with name != "a8m".
					user.HasParentWith(
						user.Not(user.Name(prt.Name)),
					),
				),
			).
			CountX(ctx),
		"should match chd2, ochd, prt, prt2",
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(prt.Name, client.User.Query().Where(user.HasChildrenWith(user.Name(chd.Name))).OnlyX(ctx).Name)
	require.Equal(prt2.Name, client.User.Query().Where(user.HasChildrenWith(user.Name(chd2.Name))).OnlyX(ctx).Name)
	// parent with 2 children named: child and child2.
	require.Zero(
		client.User.Query().
			Where(
				user.HasChildrenWith(
					user.Name(chd.Name),
					user.Name(chd2.Name),
				),
			).
			CountX(ctx),
	)
	// either has no children, or has 2 children: "child" and "child2".
	require.Equal(
		3,
		client.User.Query().
			Where(
				user.Or(
					// has no children.
					user.Not(user.HasChildren()),
					// has 2 children: "child" and "child2".
					user.HasChildrenWith(
						user.Name(chd.Name),
						user.Name(chd2.Name),
					),
				),
			).
			CountX(ctx),
		"should match chd, chd2 and ochd",
	)

	t.Log("query long path from inverse")
	require.Equal(chd.Name, ent.QueryUserChildrenFromQuery(ent.QueryUserParent(client.User, chd)).OnlyX(ctx).Name, "should get itself")
	require.Equal(prt.Name, ent.QueryUserParentFromQuery(ent.QueryUserChildrenFromQuery(ent.QueryUserParent(client.User, chd))).OnlyX(ctx).Name, "should get its parent")
	require.Equal(
		prt.Name,
		ent.QueryUserParentFromQuery(
			ent.QueryUserChildrenFromQuery(
				ent.QueryUserParent(client.User, chd).
					Where(user.HasChildren()),
			),
		).
			Where(user.HasChildren()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(prt.Name, ent.QueryUserParentFromQuery(ent.QueryUserChildren(client.User, prt)).OnlyX(ctx).Name, "should get itself")
	require.Equal(chd.Name, ent.QueryUserChildrenFromQuery(ent.QueryUserParentFromQuery(ent.QueryUserChildren(client.User, prt))).OnlyX(ctx).Name, "should get its child")
	require.Equal(
		chd.Name,
		ent.QueryUserChildrenFromQuery(
			ent.QueryUserParentFromQuery(
				ent.QueryUserChildren(client.User, prt).
					Where(user.HasParent()), // child
			).
				Where(user.HasChildren()), // parent
		). // child
			OnlyX(ctx).Name,
		"should get its child",
	)
	require.Equal(
		chd2.Name,
		ent.QueryUserChildrenFromQuery( // child
			ent.QueryUserParentFromQuery( // parent
				ent.QueryUserChildrenFromQuery( // child
					client.User.Query().
						// "alex" matches this query (not "a8m", and have a child).
						Where(
							user.Not(user.Name(prt.Name)),
							user.HasChildren(),
						),
				),
			),
		).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two instances of the same type, where the relation
// has the same name in both directions. A friendship between Users.
// User A has "friend" B (and vice versa). When setting B as a friend of A, this sets A
// as friend of B as well. In other words:
//
//	foo := client.User.Create().SetName("foo").SaveX(ctx)
//	bar := client.User.Create().SetName("bar").AddFriends(foo).SaveX(ctx)
//	count := client.User.Query.Where(user.HasFriends()).CountX(ctx)
//	// count will be 2, even though we've created only one relation above.
func M2MSelfRef(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without friends")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(ent.QueryUserFriends(client.User, foo).ExistX(ctx))

	t.Log("sets friendship on user creation (inverse creation)")
	bar := client.User.Create().SetAge(10).SetName("bar").AddFriendIDs(foo.ID).SaveX(ctx)
	require.True(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserFriends(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("add friendship to user by updating existing users")
	bar = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.User.UpdateOne(foo).AddFriendIDs(bar.ID).ExecX(ctx)
	require.True(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserFriends(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("remove friendship using update")
	client.User.UpdateOne(foo).RemoveFriendIDs(bar.ID).ExecX(ctx)
	require.False(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserFriends(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFriends()).CountX(ctx))
	// return back the friendship.
	client.User.UpdateOne(foo).AddFriendIDs(bar.ID).ExecX(ctx)

	t.Log("create a user without friends")
	baz := client.User.Create().SetAge(10).SetName("baz").SaveX(ctx)
	require.False(ent.QueryUserFriends(client.User, baz).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("both baz and bar are friends of foo")
	client.User.UpdateOne(baz).AddFriendIDs(foo.ID).ExecX(ctx)
	require.Equal(2, ent.QueryUserFriends(client.User, foo).CountX(ctx))
	require.Equal(foo.Name, ent.QueryUserFriends(client.User, bar).OnlyX(ctx).Name)
	require.Equal(foo.Name, ent.QueryUserFriends(client.User, baz).OnlyX(ctx).Name)
	require.Equal(3, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("query with side lookup")
	require.Equal(
		[]string{bar.Name, baz.Name},
		client.User.Query().
			Where(user.HasFriendsWith(user.Name(foo.Name))).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.HasFriendsWith(user.Name(bar.Name))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Not(user.HasFriendsWith(user.Name(foo.Name)))).
			OnlyX(ctx).Name,
		"foo does not have friendship with foo",
	)
	require.Equal(
		[]string{bar.Name, baz.Name},
		client.User.Query().
			Where(user.Not(user.HasFriendsWith(user.Name(baz.Name)))).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
		"bar and baz do not have friendship with baz",
	)

	t.Log("query path from a user")
	require.Equal(
		foo.Name,
		ent.QueryUserFriendsFromQuery( // foo
			ent.QueryUserFriendsFromQuery( // baz
				ent.QueryUserFriendsFromQuery( // foo
					ent.QueryUserFriends(client.User, foo).Where(user.Name(bar.Name)), // bar
				),
			).Where(user.Name(baz.Name)),
		).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		ent.QueryUserFriendsFromQuery( // foo
			ent.QueryUserFriendsFromQuery( // bar, baz
				ent.QueryUserFriendsFromQuery( // foo
					ent.QueryUserFriends(client.User, foo), // bar, baz
				),
			),
		).
			OnlyX(ctx).Name,
	)
	require.Equal(
		baz.Name,
		ent.QueryUserFriendsFromQuery( // baz
			ent.QueryUserFriendsFromQuery( // foo
				ent.QueryUserFriends(client.User, foo).Where(user.Name(bar.Name)), // bar
			),
		).Where(user.Not(user.Name(bar.Name))).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		[]string{bar.Name, baz.Name},
		ent.QueryUserFriendsFromQuery( // bar, baz
			client.User.
				Query().
				Where(user.Name(foo.Name)), // foo
		).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
	)
	require.Equal(
		bar.Name,
		ent.QueryUserFriendsFromQuery( // bar and baz
			client.User.
				Query().
				// foo has a friend (bar) that does not have a friend named baz.
				Where(
					user.HasFriendsWith(
						user.Not(
							user.HasFriendsWith(user.Name(baz.Name)),
						),
					),
				),
		).
			// filter baz out.
			Where(user.Not(user.Name(baz.Name))).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two instances of the same type.
// Following and followers.
func M2MSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without followers")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))

	t.Log("adds followers on user creation (inverse creation)")
	bar := client.User.Create().SetAge(10).SetName("bar").AddFollowingIDs(foo.ID).SaveX(ctx)
	require.Equal(foo.Name, ent.QueryUserFollowing(client.User, bar).OnlyX(ctx).Name)
	require.Equal(bar.Name, ent.QueryUserFollowers(client.User, foo).OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.HasFollowing()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowing()).CountX(ctx))

	t.Log("add followers to user by updating existing users")
	bar = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.User.UpdateOne(foo).AddFollowerIDs(bar.ID).ExecX(ctx)
	require.Equal(foo.Name, ent.QueryUserFollowing(client.User, bar).OnlyX(ctx).Name)
	require.Equal(bar.Name, ent.QueryUserFollowers(client.User, foo).OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.HasFollowing()).CountX(ctx))

	t.Log("remove following using update")
	client.User.UpdateOne(bar).RemoveFollowingIDs(foo.ID).ExecX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserFollowing(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowing()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	// follow back.
	client.User.UpdateOne(bar).AddFollowingIDs(foo.ID).ExecX(ctx)

	t.Log("remove followers using update (inverse)")
	client.User.UpdateOne(foo).RemoveFollowerIDs(bar.ID).ExecX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserFollowing(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowing()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	// follow back.
	client.User.UpdateOne(bar).AddFollowingIDs(foo.ID).ExecX(ctx)

	users := make([]*ent.User, 5)
	for i := range users {
		u := client.User.Create().SetAge(10).SetName(fmt.Sprintf("user-%d", i)).SaveX(ctx)
		users[i] = client.User.UpdateOne(u).AddFollowingIDs(foo.ID, bar.ID).SaveX(ctx)
		require.Equal(
			[]string{bar.Name, foo.Name},
			ent.QueryUserFollowing(client.User, u).
				Order(ent.Asc(user.FieldName)).
				GroupBy(user.FieldName).
				StringsX(ctx),
		)
	}
	require.Equal(5, ent.QueryUserFollowers(client.User, bar).CountX(ctx), "users1..5")
	require.Equal(6, ent.QueryUserFollowers(client.User, foo).CountX(ctx), "users1..5 and bar")
	require.Equal(2, client.User.Query().Where(user.HasFollowers()).CountX(ctx), "foo and bar")
	require.Equal(6, client.User.Query().Where(user.HasFollowing()).CountX(ctx), "users1..5 and bar")
	// compare followers.
	require.Equal(
		ent.QueryUserFollowers(client.User, bar).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
		ent.QueryUserFollowers(client.User, foo).
			Where(user.Not(user.Name(bar.Name))).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
		"bar.followers = (foo.followers - bar)",
	)

	// delete users 1..5.
	client.User.Delete().Where(user.NameHasPrefix("user")).ExecX(ctx)
	require.Equal(2, client.User.Query().CountX(ctx))

	t.Log("query with side lookup from inverse")
	require.Equal(foo.Name, ent.QueryUserFollowingFromQuery(ent.QueryUserFollowers(client.User, foo)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, ent.QueryUserFollowersFromQuery(ent.QueryUserFollowingFromQuery(ent.QueryUserFollowers(client.User, foo))).OnlyX(ctx).Name, "should get its follower (bar)")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, ent.QueryUserFollowersFromQuery(ent.QueryUserFollowing(client.User, bar)).OnlyX(ctx).Name, "should get itself")
	require.Equal(foo.Name, ent.QueryUserFollowingFromQuery(ent.QueryUserFollowersFromQuery(ent.QueryUserFollowing(client.User, bar))).OnlyX(ctx).Name, "should get foo")

	// generate additional users and make sure we don't get them in the queries below.
	client.User.Create().SetAge(10).SetName("baz").SaveX(ctx)
	client.User.Create().SetAge(10).SetName("qux").SaveX(ctx)

	t.Log("query path from a user")
	require.Equal(
		bar.Name,
		ent.QueryUserFollowersFromQuery( // bar
			ent.QueryUserFollowingFromQuery( // foo
				ent.QueryUserFollowers(client.User, foo).Where(user.Name(bar.Name)), // bar
			).Where(user.HasFollowers()),
		).
			Where(
				user.HasFollowingWith(
					user.Name(foo.Name),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		foo.Name,
		ent.QueryUserFollowingFromQuery( // has followers named bar (foo)
			ent.QueryUserFollowersFromQuery( // bar
				ent.QueryUserFollowingFromQuery( // foo
					ent.QueryUserFollowersFromQuery( // bar
						client.User.
							Query().Where(user.Name(foo.Name)), // foo
					).Where(user.Name(bar.Name)),
				).Where(user.HasFollowers()),
			).
				Where(
					user.HasFollowingWith(
						user.Name(foo.Name),
					),
				),
		).
			Where(
				user.HasFollowersWith(
					user.Name(bar.Name),
				),
			).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two different types. User and groups.
func M2MTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without groups")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.Zero(client.Group.Query().CountX(ctx))

	t.Log("adds users to group on group creation (inverse creation)")
	// group-info is required edge.
	inf := client.GroupInfo.Create().SetDesc("desc").SaveX(ctx)
	hub := client.Group.Create().SetName("Github").SetExpire(time.Now()).AddUserIDs(foo.ID).SetInfoID(inf.ID).SaveX(ctx)
	require.Equal(foo.Name, ent.QueryGroupUsers(client.Group, hub).OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, ent.QueryUserGroups(client.User, foo).OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.HasUsers()).CountX(ctx))

	t.Log("add an existing M2M edge should not throw an error")
	client.User.UpdateOne(foo).AddGroupIDs(hub.ID).ExecX(ctx)
	require.Equal(1, ent.QueryUserGroups(client.User, foo).CountX(ctx))
	client.Group.UpdateOne(hub).AddUserIDs(foo.ID).ExecX(ctx)
	require.Equal(1, ent.QueryGroupUsers(client.Group, hub).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.Group.DeleteOne(hub).ExecX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))

	t.Log("add user to groups updating existing users")
	hub = client.Group.Create().SetName("Github").SetExpire(time.Now()).SetInfoID(inf.ID).SaveX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	client.User.UpdateOne(foo).AddGroupIDs(hub.ID).ExecX(ctx)
	require.Equal(foo.Name, ent.QueryGroupUsers(client.Group, hub).OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, ent.QueryUserGroups(client.User, foo).OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.HasUsers()).CountX(ctx))

	t.Log("delete assoc should delete inverse as well")
	client.User.DeleteOne(foo).ExecX(ctx)
	require.False(ent.QueryGroupUsers(client.Group, hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	// add back the user.
	foo = client.User.Create().SetAge(10).SetName("foo").AddGroupIDs(hub.ID).SaveX(ctx)

	t.Log("remove following using update (assoc)")
	client.User.UpdateOne(foo).RemoveGroupIDs(hub.ID).ExecX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.False(ent.QueryGroupUsers(client.Group, hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	// join back to group.
	client.User.UpdateOne(foo).AddGroupIDs(hub.ID).ExecX(ctx)

	t.Log("remove following using update (inverse)")
	client.Group.UpdateOne(hub).RemoveUserIDs(foo.ID).ExecX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.False(ent.QueryGroupUsers(client.Group, hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	// add back the user.
	client.Group.UpdateOne(hub).AddUserIDs(foo.ID).ExecX(ctx)

	t.Log("multiple groups and users")
	lab := client.Group.Create().SetName("Gitlab").SetExpire(time.Now()).SetInfoID(inf.ID).SaveX(ctx)
	bar := client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	require.Equal(1, client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	client.User.UpdateOne(bar).AddGroupIDs(lab.ID).ExecX(ctx)
	require.Equal(2, client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Equal(2, client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	// validate relations.
	require.Equal(foo.Name, ent.QueryGroupUsers(client.Group, hub).OnlyX(ctx).Name, "hub has only one user")
	require.Equal(hub.Name, ent.QueryUserGroups(client.User, foo).OnlyX(ctx).Name, "foo is connected only to hub")
	require.Equal(bar.Name, ent.QueryGroupUsers(client.Group, lab).OnlyX(ctx).Name, "lab has only one user")
	require.Equal(lab.Name, ent.QueryUserGroups(client.User, bar).OnlyX(ctx).Name, "bar is connected only to lab")
	// add bar to hub.
	client.User.UpdateOne(bar).AddGroupIDs(hub.ID).ExecX(ctx)
	require.Equal(2, ent.QueryGroupUsers(client.Group, hub).CountX(ctx))
	require.Equal(1, ent.QueryGroupUsers(client.Group, lab).CountX(ctx))
	require.Equal([]string{bar.Name, foo.Name}, ent.QueryGroupUsers(client.Group, hub).Order(ent.Asc(user.FieldName)).GroupBy(user.FieldName).StringsX(ctx))
	require.Equal([]string{hub.Name, lab.Name}, ent.QueryUserGroups(client.User, bar).Order(ent.Asc(user.FieldName)).GroupBy(user.FieldName).StringsX(ctx))

	t.Log("query with side lookup from inverse")
	require.Equal(hub.Name, ent.QueryUserGroupsFromQuery(ent.QueryGroupUsers(client.Group, hub)).Where(group.Name(hub.Name)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, ent.QueryGroupUsersFromQuery(ent.QueryUserGroupsFromQuery(ent.QueryGroupUsers(client.Group, lab)).Where(group.Not(group.Name(hub.Name)))).OnlyX(ctx).Name, "should get its user")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, bar).Where(group.Name(lab.Name))).OnlyX(ctx).Name, "should get itself")
	require.Equal(lab.Name, ent.QueryUserGroupsFromQuery(ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, bar).Where(group.Name(lab.Name)))).Where(group.Name(lab.Name)).OnlyX(ctx).Name, "should get its group")

	t.Log("query path from a user")
	require.Equal(
		hub.Name,
		ent.QueryUserGroupsFromQuery( // hub
			ent.QueryGroupUsersFromQuery( // foo (not having group with name "lab")
				ent.QueryUserGroups(client.User, bar).
					// hub.
					Where(
						group.HasUsersWith(user.Name(foo.Name)),
					),
			).
				Where(
					user.Not(
						user.HasGroupsWith(group.Name(lab.Name)),
					),
				),
		).
			OnlyX(ctx).Name,
	)

	t.Log("query path from a client")
	require.Equal(
		bar.Name,
		ent.QueryGroupUsersFromQuery( // bar, foo
			ent.QueryUserGroupsFromQuery( // hub
				ent.QueryGroupUsersFromQuery( // foo (not having group with name "lab")
					client.Group.
						// hub.
						Query().
						Where(
							group.HasUsersWith(user.Name(foo.Name)),
						),
				).
					Where(
						user.Not(
							user.HasGroupsWith(group.Name(lab.Name)),
						),
					),
			),
		).
			Order(ent.Asc(user.FieldName)).
			// bar
			FirstX(ctx).Name,
	)
}
