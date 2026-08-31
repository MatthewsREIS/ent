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
	"entgo.io/ent/entc/integration/ent/groupinfo"
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
	usr := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("foo")).SaveX(ctx)
	require.Zero(ent.QueryUserCard(client.User, usr).CountX(ctx))

	t.Log("add card to user on card creation (inverse creation)")
	crd := client.Card.Create().With(card.Field.Number.Set("1"), card.Edge.Owner.SetID(usr.ID)).SaveX(ctx)
	require.Equal(ent.QueryUserCard(client.User, usr).CountX(ctx), 1)
	require.Equal(ent.QueryCardOwner(client.Card, crd).CountX(ctx), 1)

	t.Log("delete inverse should delete association")
	client.Card.DeleteOne(crd).ExecX(ctx)
	require.Zero(client.Card.Query().CountX(ctx))
	require.Zero(ent.QueryUserCard(client.User, usr).CountX(ctx), "user should not have card")

	t.Log("add card to user by updating user (the owner of the edge)")
	crd = client.Card.Create().With(card.Field.Number.Set("10")).SaveX(ctx)
	client.User.UpdateOne(usr).With(user.Edge.Card.SetID(crd.ID)).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryCardOwner(client.Card, crd).OnlyX(ctx).Name)
	require.Equal(crd.Number, ent.QueryUserCard(client.User, usr).OnlyX(ctx).Number)

	t.Log("delete assoc should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.Zero(ent.QueryCardOwner(client.Card, crd).CountX(ctx), "card should not have an owner")

	t.Log("add card to user by updating card (the inverse edge)")
	usr = client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar")).SaveX(ctx)
	client.Card.UpdateOne(crd).With(card.Edge.Owner.SetID(usr.ID)).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryCardOwner(client.Card, crd).OnlyX(ctx).Name)
	require.Equal(crd.Number, ent.QueryUserCard(client.User, usr).OnlyX(ctx).Number)

	t.Log("query with side lookup on inverse")
	ocrd := client.Card.Create().With(card.Field.Number.Set("orphan card")).SaveX(ctx)
	require.Equal(crd.Number, client.Card.Query().Where(card.Edge.Owner.Has()).OnlyX(ctx).Number)
	require.Equal(ocrd.Number, client.Card.Query().Where(card.Not(card.Edge.Owner.Has())).OnlyX(ctx).Number)

	t.Log("query with side lookup on assoc")
	ousr := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("user without card")).SaveX(ctx)
	require.Equal(usr.Name, client.User.Query().Where(user.Edge.Card.Has()).OnlyX(ctx).Name)
	require.Equal(ousr.Name, client.User.Query().Where(user.Not(user.Edge.Card.Has())).OnlyX(ctx).Name)

	t.Log("query with side lookup condition on inverse")
	require.Equal(crd.Number, client.Card.Query().Where(card.Edge.Owner.HasWith(user.Field.Name.EQ(usr.Name))).OnlyX(ctx).Number)
	// has owner, but with name != "bar".
	require.Zero(client.Card.Query().Where(card.Edge.Owner.HasWith(user.Not(user.Field.Name.EQ(usr.Name)))).CountX(ctx))
	// either has no owner, or has owner with name != "bar".
	require.Equal(
		ocrd.Number,
		client.Card.Query().
			Where(
				card.Or(
					// has no owner.
					card.Not(card.Edge.Owner.Has()), card.Edge.
						// has owner with name != "bar".
						Owner.HasWith(user.Not(user.Field.Name.EQ(usr.Name))),
				),
			).
			OnlyX(ctx).Number,
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(usr.Name, client.User.Query().Where(user.Edge.Card.HasWith(card.Field.Number.EQ(crd.Number))).OnlyX(ctx).Name)
	require.Zero(client.User.Query().Where(user.Edge.Card.HasWith(card.Not(card.Field.Number.EQ(crd.Number)))).CountX(ctx))
	// either has no card, or has card with number != "10".
	require.Equal(
		ousr.Name,
		client.User.Query().
			Where(
				user.Or(
					// has no card.
					user.Not(user.Edge.Card.Has()), user.Edge.
						// has card with number != "10".
						Card.HasWith(card.Not(card.Field.Number.EQ(crd.Number))),
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
					Where(user.Edge.Card.Has()),
			),
		).
			Where(user.Edge.Card.Has()).
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
					Where(card.Edge.Owner.Has()),
			).
				Where(user.Edge.Card.Has()),
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
	head := client.Node.Create().With(node.Field.Value.Set(1)).SaveX(ctx)
	require.Zero(ent.QueryNodePrev(client.Node, head).CountX(ctx))
	require.Zero(ent.QueryNodeNext(client.Node, head).CountX(ctx))

	t.Log("add node to the linked-list and connect it to the head (inverse creation)")
	sec := client.Node.Create().With(node.Field.Value.Set(2), node.Edge.Prev.SetID(head.ID)).SaveX(ctx)
	require.Zero(ent.QueryNodeNext(client.Node, sec).CountX(ctx), "should not have next")
	require.Equal(head.ID, ent.QueryNodePrev(client.Node, sec).OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, ent.QueryNodeNext(client.Node, head).OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete inverse should delete association")
	client.Node.DeleteOne(sec).ExecX(ctx)
	require.Zero(ent.QueryNodeNext(client.Node, head).CountX(ctx))
	require.Equal(1, client.Node.Query().CountX(ctx), "linked-list should have 1 node")

	t.Log("add node to the linked-list by updating the head (the owner of the edge)")
	sec = client.Node.Create().With(node.Field.Value.Set(2)).SaveX(ctx)
	client.Node.UpdateOne(head).With(node.Edge.Next.SetID(sec.ID)).ExecX(ctx)
	require.Zero(ent.QueryNodeNext(client.Node, sec).CountX(ctx), "should not have next")
	require.Equal(head.ID, ent.QueryNodePrev(client.Node, sec).OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, ent.QueryNodeNext(client.Node, head).OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete assoc should delete inverse edge")
	client.Node.DeleteOne(head).ExecX(ctx)
	require.Zero(ent.QueryNodePrev(client.Node, sec).CountX(ctx), "second node should be the head now")
	require.Zero(ent.QueryNodeNext(client.Node, sec).CountX(ctx), "second node should be the head now")

	t.Log("update second node value to be 1")
	head = client.Node.UpdateOne(sec).With(node.Field.Value.Set(1)).SaveX(ctx)
	require.Equal(1, head.Value)

	t.Log("create a linked-list 1->2->3->4->5")
	nodes := []*ent.Node{head}
	for i := 0; i < 4; i++ {
		next := client.Node.Create().With(node.Field.Value.Set(nodes[i].Value+1), node.Edge.Prev.SetID(nodes[i].ID)).SaveX(ctx)
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
	require.Equal(4, client.Node.Query().Where(node.Edge.Next.Has()).CountX(ctx))
	require.Equal(4, client.Node.Query().Where(node.Edge.Prev.Has()).CountX(ctx))

	t.Log("make the linked-list to be circular")
	client.Node.UpdateOne(nodes[len(nodes)-1]).With(node.Edge.Next.SetID(head.ID)).SaveX(ctx)
	require.Equal(nodes[0].Value, ent.QueryNodeNext(client.Node, nodes[len(nodes)-1]).OnlyX(ctx).Value, "last node should point to head")
	require.Equal(nodes[len(nodes)-1].Value, ent.QueryNodePrev(client.Node, nodes[0]).OnlyX(ctx).Value, "head should have a reference to the tail")

	t.Log("query with side lookup on inverse/assoc")
	require.Equal(5, client.Node.Query().Where(node.Edge.Next.Has()).CountX(ctx))
	require.Equal(5, client.Node.Query().Where(node.Edge.Prev.Has()).CountX(ctx))
	// node that points (with "next") to other node with value 2 (the head).
	require.Equal(nodes[0].Value, client.Node.Query().Where(node.Edge.Next.HasWith(node.Field.Value.EQ(2))).OnlyX(ctx).Value)
	// node that points (with "next") to other node with value 1 (the tail).
	require.Equal(nodes[len(nodes)-1].Value, client.Node.Query().Where(node.Edge.Next.HasWith(node.Field.Value.EQ(1))).OnlyX(ctx).Value)
	// nodes that points to nodes with value greater than 2 (X->2->3->4->X).
	values, err := client.Node.Query().
		Where(node.Edge.Next.HasWith(node.Field.Value.GT(2))).
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
	require.Zero(ent.QueryNodePrevFromQuery(ent.QueryNodePrevFromQuery(ent.QueryNodePrevFromQuery(ent.QueryNodePrevFromQuery(ent.QueryNodePrev(client.Node, head)).Where(node.Field.Value.GT(10))))).CountX(ctx))

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
	require.Zero(ent.QueryNodeNextFromQuery(ent.QueryNodeNextFromQuery(ent.QueryNodeNextFromQuery(ent.QueryNodeNextFromQuery(ent.QueryNodeNext(client.Node, head)).Where(node.Field.Value.GT(10))))).CountX(ctx))

	t.Log("delete all nodes except the head")
	client.Node.Delete().Where(node.Field.Value.GT(1)).ExecX(ctx)
	head = client.Node.Query().OnlyX(ctx)

	t.Log("node points to itself (circular linked-list with 1 node)")
	head = client.Node.UpdateOne(head).With(node.Edge.Next.SetID(head.ID)).SaveX(ctx)
	require.Equal(head.ID, ent.QueryNodePrev(client.Node, head).OnlyIDX(ctx))
	require.Equal(head.ID, ent.QueryNodeNext(client.Node, head).OnlyIDX(ctx))
	head = client.Node.UpdateOne(head).With(node.Edge.Next.Clear()).SaveX(ctx)
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
	foo := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("foo")).SaveX(ctx)
	require.False(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))

	t.Log("sets spouse on user creation (inverse creation)")
	bar := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar"), user.Edge.Spouse.SetID(foo.ID)).SaveX(ctx)
	require.True(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Edge.Spouse.Has()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Spouse.Has()).CountX(ctx))

	t.Log("add spouse to user by updating a user")
	bar = client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar")).SaveX(ctx)
	client.User.UpdateOne(foo).With(user.Edge.Spouse.SetID(bar.ID)).ExecX(ctx)
	require.True(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Edge.Spouse.Has()).CountX(ctx))

	t.Log("remove a spouse using update")
	client.User.UpdateOne(foo).With(user.Edge.Spouse.Clear()).ExecX(ctx)
	require.False(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Spouse.Has()).CountX(ctx))
	// return back the spouse.
	client.User.UpdateOne(foo).With(user.Edge.Spouse.SetID(bar.ID)).ExecX(ctx)

	t.Log("create a user without spouse")
	baz := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("baz")).SaveX(ctx)
	require.False(ent.QueryUserSpouse(client.User, baz).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Edge.Spouse.Has()).CountX(ctx))

	t.Log("set a new spouse")
	client.User.UpdateOne(foo).With(user.Edge.Spouse.Clear(), user.Edge.Spouse.SetID(baz.ID)).ExecX(ctx)
	require.True(ent.QueryUserSpouse(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserSpouse(client.User, baz).ExistX(ctx))
	require.False(ent.QueryUserSpouse(client.User, bar).ExistX(ctx))
	// return back the spouse.
	client.User.UpdateOne(foo).With(user.Edge.Spouse.Clear(), user.Edge.Spouse.SetID(bar.ID)).ExecX(ctx)

	t.Log("spouse is a unique edge")
	require.Error(client.User.UpdateOne(baz).With(user.Edge.Spouse.SetID(bar.ID)).Exec(ctx))
	require.Error(client.User.UpdateOne(baz).With(user.Edge.Spouse.SetID(foo.ID)).Exec(ctx))

	t.Log("query with side lookup")
	require.Equal(
		bar.Name,
		client.User.Query().
			Where(user.Edge.Spouse.HasWith(user.Field.Name.EQ("foo"))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Edge.Spouse.HasWith(user.Field.Name.EQ("bar"))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		baz.Name,
		client.User.Query().
			Where(user.Not(user.Edge.Spouse.Has())).
			OnlyX(ctx).Name,
	)
	// has spouse that has a spouse with name "foo" (which actually means itself).
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Edge.Spouse.HasWith(user.Edge.Spouse.HasWith(user.Field.Name.EQ("foo")))).
			OnlyX(ctx).Name,
	)
	// has spouse that has a spouse with name "bar" (which actually means itself).
	require.Equal(
		bar.Name,
		client.User.Query().
			Where(user.Edge.Spouse.HasWith(user.Edge.Spouse.HasWith(user.Field.Name.EQ("bar")))).
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
				Where(user.Field.Name.EQ("foo")), // foo
		).
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		ent.QueryUserSpouseFromQuery( // bar
			ent.QueryUserSpouseFromQuery( // foo
				client.User.
					Query().
					Where(user.Field.Name.EQ("bar")), // bar
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
	usr := client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("a8m")).SaveX(ctx)
	require.False(ent.QueryUserPets(client.User, usr).ExistX(ctx))

	t.Log("add pet to user on pet creation (inverse creation)")
	pedro := client.Pet.Create().With(pet.Field.Name.Set("pedro"), pet.Edge.Owner.SetID(usr.ID)).SaveX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwner(client.Pet, pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, ent.QueryUserPets(client.User, usr).OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.Pet.DeleteOne(pedro).ExecX(ctx)
	require.Zero(client.Pet.Query().CountX(ctx))
	require.False(ent.QueryUserPets(client.User, usr).ExistX(ctx), "user should not have pet")

	t.Log("add pet to user by updating user (the owner of the edge)")
	pedro = client.Pet.Create().With(pet.Field.Name.Set("pedro")).SaveX(ctx)
	client.User.UpdateOne(usr).With(user.Edge.Pets.AddIDs(pedro.ID)).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwner(client.Pet, pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, ent.QueryUserPets(client.User, usr).OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.False(ent.QueryPetOwner(client.Pet, pedro).ExistX(ctx), "pet should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	usr = client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("a8m")).SaveX(ctx)
	client.Pet.UpdateOne(pedro).With(pet.Edge.Owner.SetID(usr.ID)).ExecX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwner(client.Pet, pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, ent.QueryUserPets(client.User, usr).OnlyX(ctx).Name)

	t.Log("add another pet to user")
	xabi := client.Pet.Create().With(pet.Field.Name.Set("xabi"), pet.Edge.Owner.SetID(usr.ID)).SaveX(ctx)
	require.Equal(2, ent.QueryUserPets(client.User, usr).CountX(ctx))
	require.Equal(1, ent.QueryPetOwner(client.Pet, xabi).CountX(ctx))
	require.Equal(1, ent.QueryPetOwner(client.Pet, pedro).CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("alex"), user.Edge.Pets.AddIDs(pedro.ID)).Save(ctx)
	require.Error(err, "pet already has an owner")

	t.Log("add multiple pets on creation")
	p1 := client.Pet.Create().With(pet.Field.Name.Set("p1")).SaveX(ctx)
	p2 := client.Pet.Create().With(pet.Field.Name.Set("p2")).SaveX(ctx)
	usr2 := client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("alex"), user.Edge.Pets.AddIDs(p1.ID, p2.ID)).SaveX(ctx)
	require.True(ent.QueryPetOwner(client.Pet, p1).ExistX(ctx))
	require.True(ent.QueryPetOwner(client.Pet, p2).ExistX(ctx))
	require.Equal(2, ent.QueryUserPets(client.User, usr2).CountX(ctx))
	// delete p1, p2.
	client.Pet.Delete().Where(pet.Field.ID.In(p1.ID, p2.ID)).ExecX(ctx)
	require.Zero(ent.QueryUserPets(client.User, usr2).CountX(ctx))

	t.Log("change the owner a pet")
	client.Pet.UpdateOne(xabi).With(pet.Edge.Owner.Clear(), pet.Edge.Owner.SetID(usr2.ID)).ExecX(ctx)
	require.Equal(1, ent.QueryUserPets(client.User, usr).CountX(ctx))
	require.Equal(1, ent.QueryUserPets(client.User, usr2).CountX(ctx))
	require.Equal(usr2.Name, ent.QueryPetOwner(client.Pet, xabi).OnlyX(ctx).Name)

	t.Log("query with side lookup on inverse")
	opet := client.Pet.Create().With(pet.Field.Name.Set("orphan pet")).SaveX(ctx)
	require.Equal(opet.Name, client.Pet.Query().Where(pet.Not(pet.Edge.Owner.Has())).OnlyX(ctx).Name)
	require.Equal(2, client.Pet.Query().Where(pet.Edge.Owner.Has()).CountX(ctx))

	t.Log("query with side lookup on assoc")
	require.Zero(client.User.Query().Where(user.Not(user.Edge.Pets.Has())).CountX(ctx))
	ousr := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("user without pet")).SaveX(ctx)
	require.Equal(2, client.User.Query().Where(user.Edge.Pets.Has()).CountX(ctx))
	require.Equal(ousr.Name, client.User.Query().Where(user.Not(user.Edge.Pets.Has())).OnlyX(ctx).Name)

	t.Log("query with side lookup condition on inverse")
	require.Equal(pedro.Name, client.Pet.Query().Where(pet.Edge.Owner.HasWith(user.Field.Name.EQ(usr.Name))).OnlyX(ctx).Name)
	// has owner, but with name != "a8m".
	require.Equal(xabi.Name, client.Pet.Query().Where(pet.Edge.Owner.HasWith(user.Not(user.Field.Name.EQ(usr.Name)))).OnlyX(ctx).Name)
	// either has no owner, or has owner with name != "alex" and name != "a8m".
	require.Equal(
		opet.Name,
		client.Pet.Query().
			Where(
				pet.Or(
					// has no owner.
					pet.Not(pet.Edge.Owner.Has()), pet.Edge.Owner.
						// has owner with name != "a8m" and name != "alex".
						HasWith(
							user.Not(user.Field.Name.EQ(usr.Name)),
							user.Not(user.Field.Name.EQ(usr2.Name)),
						),
				),
			).
			OnlyX(ctx).Name,
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(usr.Name, client.User.Query().Where(user.Edge.Pets.HasWith(pet.Field.Name.EQ(pedro.Name))).OnlyX(ctx).Name)
	require.Equal(usr2.Name, client.User.Query().Where(user.Edge.Pets.HasWith(pet.Field.Name.EQ(xabi.Name))).OnlyX(ctx).Name)
	require.Zero(
		client.User.Query().
			Where(user.Edge.Pets.HasWith(
				pet.Not(pet.Field.Name.EQ(xabi.Name)),
				pet.Not(pet.Field.Name.EQ(pedro.Name)),
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
					user.Not(user.Edge.Pets.Has()), user.Edge.
						// has pet with name != "pedro" and name != "xabi".
						Pets.HasWith(
						pet.Not(pet.Field.Name.EQ(xabi.Name)),
						pet.Not(pet.Field.Name.EQ(pedro.Name)),
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
					Where(user.Edge.Pets.Has()),
			),
		).
			Where(user.Edge.Pets.Has()).
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
					Where(pet.Edge.Owner.Has()), // pedro
			).
				Where(user.Edge.Pets.Has()), // a8m
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
							user.Not(user.Field.Name.EQ(usr.Name)), user.Edge.Pets.Has(),
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
	prt := client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("a8m")).SaveX(ctx)
	require.Zero(ent.QueryUserChildren(client.User, prt).CountX(ctx))

	t.Log("add child to parent on child creation (inverse creation)")
	chd := client.User.Create().With(user.Field.Age.Set(1), user.Field.Name.Set("child"), user.Edge.Parent.SetID(prt.ID)).SaveX(ctx)
	require.Equal(prt.Name, ent.QueryUserParent(client.User, chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, ent.QueryUserChildren(client.User, prt).OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(chd).ExecX(ctx)
	require.False(ent.QueryUserChildren(client.User, prt).ExistX(ctx), "user should not have children")

	t.Log("add child to parent by updating user (the owner of the edge)")
	chd = client.User.Create().With(user.Field.Age.Set(1), user.Field.Name.Set("child")).SaveX(ctx)
	client.User.UpdateOne(prt).With(user.Edge.Children.AddIDs(chd.ID)).ExecX(ctx)
	require.Equal(prt.Name, ent.QueryUserParent(client.User, chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, ent.QueryUserChildren(client.User, prt).OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(prt).ExecX(ctx)
	require.Equal(1, client.User.Query().CountX(ctx))
	require.False(ent.QueryUserParent(client.User, chd).ExistX(ctx), "child should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	prt = client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("a8m")).SaveX(ctx)
	client.User.UpdateOne(chd).With(user.Edge.Parent.SetID(prt.ID)).ExecX(ctx)
	require.Equal(prt.Name, ent.QueryUserParent(client.User, chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, ent.QueryUserChildren(client.User, prt).OnlyX(ctx).Name)
	require.Zero(ent.QueryUserParent(client.User, prt).CountX(ctx), "parent is orphan")
	require.Zero(ent.QueryUserChildren(client.User, chd).CountX(ctx), "child should not have children")

	t.Log("add another pet to user")
	chd2 := client.User.Create().With(user.Field.Age.Set(1), user.Field.Name.Set("child2"), user.Edge.Parent.SetID(prt.ID)).SaveX(ctx)
	require.Equal(2, ent.QueryUserChildren(client.User, prt).CountX(ctx))
	require.Equal(1, ent.QueryUserParent(client.User, chd).CountX(ctx))
	require.Equal(1, ent.QueryUserParent(client.User, chd2).CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("alex"), user.Edge.Children.AddIDs(chd.ID)).Save(ctx)
	require.Error(err, "child already has parent")
	_, err = client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("alex"), user.Edge.Children.AddIDs(chd2.ID)).Save(ctx)
	require.Error(err, "child already has parent")

	t.Log("add multiple child on creation")
	chd3 := client.User.Create().With(user.Field.Age.Set(1), user.Field.Name.Set("child3")).SaveX(ctx)
	chd4 := client.User.Create().With(user.Field.Age.Set(1), user.Field.Name.Set("child4")).SaveX(ctx)
	prt2 := client.User.Create().With(user.Field.Age.Set(30), user.Field.Name.Set("alex"), user.Edge.Children.AddIDs(chd3.ID, chd4.ID)).SaveX(ctx)
	require.True(ent.QueryUserParent(client.User, chd3).ExistX(ctx))
	require.True(ent.QueryUserParent(client.User, chd3).ExistX(ctx))
	require.Equal(2, ent.QueryUserChildren(client.User, prt2).CountX(ctx))
	// delete chd3, chd4.
	client.User.Delete().Where(user.Field.ID.In(chd3.ID, chd4.ID)).ExecX(ctx)
	require.Zero(ent.QueryUserChildren(client.User, prt2).CountX(ctx))

	t.Log("change the parent a child")
	client.User.UpdateOne(chd2).With(user.Edge.Parent.Clear(), user.Edge.Parent.SetID(prt2.ID)).ExecX(ctx)
	require.Equal(1, ent.QueryUserChildren(client.User, prt).CountX(ctx))
	require.Equal(1, ent.QueryUserChildren(client.User, prt2).CountX(ctx))
	require.Equal(chd2.Name, ent.QueryUserChildren(client.User, prt2).OnlyX(ctx).Name)

	t.Log("query with side lookup on inverse")
	ochd := client.User.Create().With(user.Field.Age.Set(1), user.Field.Name.Set("orphan user")).SaveX(ctx)
	require.Equal(3, client.User.Query().Where(user.Not(user.Edge.Parent.Has())).CountX(ctx))
	require.Equal(
		ochd.Name,
		client.User.Query().
			Where(
				user.Not(user.Edge.Parent.Has()),
				user.Not(user.Edge.Children.Has()),
			).
			OnlyX(ctx).Name,
		"3 orphan users, but only one does not have children",
	)
	require.Equal(2, client.User.Query().Where(user.Edge.Parent.Has()).CountX(ctx))

	t.Log("query with side lookup on assoc")
	require.Equal(2, client.User.Query().Where(user.Edge.Children.Has()).CountX(ctx))
	require.Equal(3, client.User.Query().Where(user.Not(user.Edge.Children.Has())).CountX(ctx))

	t.Log("query with side lookup condition on inverse")
	require.Equal(chd.Name, client.User.Query().Where(user.Edge.Parent.HasWith(user.Field.Name.EQ(prt.Name))).OnlyX(ctx).Name)
	// has parent, but with name != "a8m".
	require.Equal(chd2.Name, client.User.Query().Where(user.Edge.Parent.HasWith(user.Not(user.Field.Name.EQ(prt.Name)))).OnlyX(ctx).Name)
	// either has no parent, or has parent with name != "alex".
	require.Equal(
		4,
		client.User.Query().
			Where(
				user.Or(
					// has no parent.
					user.Not(user.Edge.Parent.Has()), user.Edge.
						// has parent with name != "alex".
						Parent.HasWith(
						user.Not(user.Field.Name.EQ(prt2.Name)),
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
					user.Not(user.Edge.Parent.Has()), user.Edge.
						// has parent with name != "a8m".
						Parent.HasWith(
						user.Not(user.Field.Name.EQ(prt.Name)),
					),
				),
			).
			CountX(ctx),
		"should match chd2, ochd, prt, prt2",
	)

	t.Log("query with side lookup condition on assoc")
	require.Equal(prt.Name, client.User.Query().Where(user.Edge.Children.HasWith(user.Field.Name.EQ(chd.Name))).OnlyX(ctx).Name)
	require.Equal(prt2.Name, client.User.Query().Where(user.Edge.Children.HasWith(user.Field.Name.EQ(chd2.Name))).OnlyX(ctx).Name)
	// parent with 2 children named: child and child2.
	require.Zero(
		client.User.Query().
			Where(user.Edge.Children.HasWith(user.Field.Name.EQ(chd.Name), user.Field.Name.EQ(chd2.Name))).
			CountX(ctx),
	)
	// either has no children, or has 2 children: "child" and "child2".
	require.Equal(
		3,
		client.User.Query().
			Where(
				user.Or(
					// has no children.
					user.Not(user.Edge.Children.Has()), user.Edge.
						// has 2 children: "child" and "child2".
						Children.HasWith(user.Field.Name.EQ(chd.Name), user.Field.Name.EQ(chd2.Name)),
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
					Where(user.Edge.Children.Has()),
			),
		).
			Where(user.Edge.Children.Has()).
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
					Where(user.Edge.Parent.Has()), // child
			).
				Where(user.Edge.Children.Has()), // parent
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
							user.Not(user.Field.Name.EQ(prt.Name)), user.Edge.Children.Has(),
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
	foo := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("foo")).SaveX(ctx)
	require.False(ent.QueryUserFriends(client.User, foo).ExistX(ctx))

	t.Log("sets friendship on user creation (inverse creation)")
	bar := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar"), user.Edge.Friends.AddIDs(foo.ID)).SaveX(ctx)
	require.True(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserFriends(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Edge.Friends.Has()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Friends.Has()).CountX(ctx))

	t.Log("add friendship to user by updating existing users")
	bar = client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar")).SaveX(ctx)
	client.User.UpdateOne(foo).With(user.Edge.Friends.AddIDs(bar.ID)).ExecX(ctx)
	require.True(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.True(ent.QueryUserFriends(client.User, bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Edge.Friends.Has()).CountX(ctx))

	t.Log("remove friendship using update")
	client.User.UpdateOne(foo).With(user.Edge.Friends.RemoveIDs(bar.ID)).ExecX(ctx)
	require.False(ent.QueryUserFriends(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserFriends(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Friends.Has()).CountX(ctx))
	// return back the friendship.
	client.User.UpdateOne(foo).With(user.Edge.Friends.AddIDs(bar.ID)).ExecX(ctx)

	t.Log("create a user without friends")
	baz := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("baz")).SaveX(ctx)
	require.False(ent.QueryUserFriends(client.User, baz).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.Edge.Friends.Has()).CountX(ctx))

	t.Log("both baz and bar are friends of foo")
	client.User.UpdateOne(baz).With(user.Edge.Friends.AddIDs(foo.ID)).ExecX(ctx)
	require.Equal(2, ent.QueryUserFriends(client.User, foo).CountX(ctx))
	require.Equal(foo.Name, ent.QueryUserFriends(client.User, bar).OnlyX(ctx).Name)
	require.Equal(foo.Name, ent.QueryUserFriends(client.User, baz).OnlyX(ctx).Name)
	require.Equal(3, client.User.Query().Where(user.Edge.Friends.Has()).CountX(ctx))

	t.Log("query with side lookup")
	require.Equal(
		[]string{bar.Name, baz.Name},
		client.User.Query().
			Where(user.Edge.Friends.HasWith(user.Field.Name.EQ(foo.Name))).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Edge.Friends.HasWith(user.Field.Name.EQ(bar.Name))).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.Query().
			Where(user.Not(user.Edge.Friends.HasWith(user.Field.Name.EQ(foo.Name)))).
			OnlyX(ctx).Name,
		"foo does not have friendship with foo",
	)
	require.Equal(
		[]string{bar.Name, baz.Name},
		client.User.Query().
			Where(user.Not(user.Edge.Friends.HasWith(user.Field.Name.EQ(baz.Name)))).
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
					ent.QueryUserFriends(client.User, foo).Where(user.Field.Name.EQ(bar.Name)), // bar
				),
			).Where(user.Field.Name.EQ(baz.Name)),
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
				ent.QueryUserFriends(client.User, foo).Where(user.Field.Name.EQ(bar.Name)), // bar
			),
		).Where(user.Not(user.Field.Name.EQ(bar.Name))).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		[]string{bar.Name, baz.Name},
		ent.QueryUserFriendsFromQuery( // bar, baz
			client.User.
				Query().
				Where(user.Field.Name.EQ(foo.Name)), // foo
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
				Where(user.Edge.Friends.HasWith(
					user.Not(user.Edge.Friends.HasWith(user.Field.Name.EQ(baz.Name))),
				),
				),
		).
			// filter baz out.
			Where(user.Not(user.Field.Name.EQ(baz.Name))).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two instances of the same type.
// Following and followers.
func M2MSameType(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without followers")
	foo := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("foo")).SaveX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))

	t.Log("adds followers on user creation (inverse creation)")
	bar := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar"), user.Edge.Following.AddIDs(foo.ID)).SaveX(ctx)
	require.Equal(foo.Name, ent.QueryUserFollowing(client.User, bar).OnlyX(ctx).Name)
	require.Equal(bar.Name, ent.QueryUserFollowers(client.User, foo).OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.Edge.Followers.Has()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.Edge.Following.Has()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Followers.Has()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Following.Has()).CountX(ctx))

	t.Log("add followers to user by updating existing users")
	bar = client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar")).SaveX(ctx)
	client.User.UpdateOne(foo).With(user.Edge.Followers.AddIDs(bar.ID)).ExecX(ctx)
	require.Equal(foo.Name, ent.QueryUserFollowing(client.User, bar).OnlyX(ctx).Name)
	require.Equal(bar.Name, ent.QueryUserFollowers(client.User, foo).OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.Edge.Followers.Has()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.Edge.Following.Has()).CountX(ctx))

	t.Log("remove following using update")
	client.User.UpdateOne(bar).With(user.Edge.Following.RemoveIDs(foo.ID)).ExecX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserFollowing(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Following.Has()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Followers.Has()).CountX(ctx))
	// follow back.
	client.User.UpdateOne(bar).With(user.Edge.Following.AddIDs(foo.ID)).ExecX(ctx)

	t.Log("remove followers using update (inverse)")
	client.User.UpdateOne(foo).With(user.Edge.Followers.RemoveIDs(bar.ID)).ExecX(ctx)
	require.False(ent.QueryUserFollowers(client.User, foo).ExistX(ctx))
	require.False(ent.QueryUserFollowing(client.User, bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Following.Has()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Followers.Has()).CountX(ctx))
	// follow back.
	client.User.UpdateOne(bar).With(user.Edge.Following.AddIDs(foo.ID)).ExecX(ctx)

	users := make([]*ent.User, 5)
	for i := range users {
		u := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set(fmt.Sprintf("user-%d", i))).SaveX(ctx)
		users[i] = client.User.UpdateOne(u).With(user.Edge.Following.AddIDs(foo.ID, bar.ID)).SaveX(ctx)
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
	require.Equal(2, client.User.Query().Where(user.Edge.Followers.Has()).CountX(ctx), "foo and bar")
	require.Equal(6, client.User.Query().Where(user.Edge.Following.Has()).CountX(ctx), "users1..5 and bar")
	// compare followers.
	require.Equal(
		ent.QueryUserFollowers(client.User, bar).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
		ent.QueryUserFollowers(client.User, foo).
			Where(user.Not(user.Field.Name.EQ(bar.Name))).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
		"bar.followers = (foo.followers - bar)",
	)

	// delete users 1..5.
	client.User.Delete().Where(user.Field.Name.HasPrefix("user")).ExecX(ctx)
	require.Equal(2, client.User.Query().CountX(ctx))

	t.Log("query with side lookup from inverse")
	require.Equal(foo.Name, ent.QueryUserFollowingFromQuery(ent.QueryUserFollowers(client.User, foo)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, ent.QueryUserFollowersFromQuery(ent.QueryUserFollowingFromQuery(ent.QueryUserFollowers(client.User, foo))).OnlyX(ctx).Name, "should get its follower (bar)")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, ent.QueryUserFollowersFromQuery(ent.QueryUserFollowing(client.User, bar)).OnlyX(ctx).Name, "should get itself")
	require.Equal(foo.Name, ent.QueryUserFollowingFromQuery(ent.QueryUserFollowersFromQuery(ent.QueryUserFollowing(client.User, bar))).OnlyX(ctx).Name, "should get foo")

	// generate additional users and make sure we don't get them in the queries below.
	client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("baz")).SaveX(ctx)
	client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("qux")).SaveX(ctx)

	t.Log("query path from a user")
	require.Equal(
		bar.Name,
		ent.QueryUserFollowersFromQuery( // bar
			ent.QueryUserFollowingFromQuery( // foo
				ent.QueryUserFollowers(client.User, foo).Where(user.Field.Name.EQ(bar.Name)), // bar
			).Where(user.Edge.Followers.Has()),
		).
			Where(user.Edge.Following.HasWith(user.Field.Name.EQ(foo.Name))).
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
							Query().Where(user.Field.Name.EQ(foo.Name)), // foo
					).Where(user.Field.Name.EQ(bar.Name)),
				).Where(user.Edge.Followers.Has()),
			).
				Where(user.Edge.Following.HasWith(user.Field.Name.EQ(foo.Name))),
		).
			Where(user.Edge.Followers.HasWith(user.Field.Name.EQ(bar.Name))).
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two different types. User and groups.
func M2MTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without groups")
	foo := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("foo")).SaveX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.Zero(client.Group.Query().CountX(ctx))

	t.Log("adds users to group on group creation (inverse creation)")
	// group-info is required edge.
	inf := client.GroupInfo.Create().With(groupinfo.Field.Desc.Set("desc")).SaveX(ctx)
	hub := client.Group.Create().With(group.Field.Name.Set("Github"), group.Field.Expire.Set(time.Now()), group.Edge.Users.AddIDs(foo.ID), group.Edge.Info.SetID(inf.ID)).SaveX(ctx)
	require.Equal(foo.Name, ent.QueryGroupUsers(client.Group, hub).OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, ent.QueryUserGroups(client.User, foo).OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))

	t.Log("add an existing M2M edge should not throw an error")
	client.User.UpdateOne(foo).With(user.Edge.Groups.AddIDs(hub.ID)).ExecX(ctx)
	require.Equal(1, ent.QueryUserGroups(client.User, foo).CountX(ctx))
	client.Group.UpdateOne(hub).With(group.Edge.Users.AddIDs(foo.ID)).ExecX(ctx)
	require.Equal(1, ent.QueryGroupUsers(client.Group, hub).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.Group.DeleteOne(hub).ExecX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))

	t.Log("add user to groups updating existing users")
	hub = client.Group.Create().With(group.Field.Name.Set("Github"), group.Field.Expire.Set(time.Now()), group.Edge.Info.SetID(inf.ID)).SaveX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	client.User.UpdateOne(foo).With(user.Edge.Groups.AddIDs(hub.ID)).ExecX(ctx)
	require.Equal(foo.Name, ent.QueryGroupUsers(client.Group, hub).OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, ent.QueryUserGroups(client.User, foo).OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))

	t.Log("delete assoc should delete inverse as well")
	client.User.DeleteOne(foo).ExecX(ctx)
	require.False(ent.QueryGroupUsers(client.Group, hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))
	// add back the user.
	foo = client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("foo"), user.Edge.Groups.AddIDs(hub.ID)).SaveX(ctx)

	t.Log("remove following using update (assoc)")
	client.User.UpdateOne(foo).With(user.Edge.Groups.RemoveIDs(hub.ID)).ExecX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.False(ent.QueryGroupUsers(client.Group, hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))
	// join back to group.
	client.User.UpdateOne(foo).With(user.Edge.Groups.AddIDs(hub.ID)).ExecX(ctx)

	t.Log("remove following using update (inverse)")
	client.Group.UpdateOne(hub).With(group.Edge.Users.RemoveIDs(foo.ID)).ExecX(ctx)
	require.False(ent.QueryUserGroups(client.User, foo).ExistX(ctx))
	require.False(ent.QueryGroupUsers(client.Group, hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))
	// add back the user.
	client.Group.UpdateOne(hub).With(group.Edge.Users.AddIDs(foo.ID)).ExecX(ctx)

	t.Log("multiple groups and users")
	lab := client.Group.Create().With(group.Field.Name.Set("Gitlab"), group.Field.Expire.Set(time.Now()), group.Edge.Info.SetID(inf.ID)).SaveX(ctx)
	bar := client.User.Create().With(user.Field.Age.Set(10), user.Field.Name.Set("bar")).SaveX(ctx)
	require.Equal(1, client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))
	client.User.UpdateOne(bar).With(user.Edge.Groups.AddIDs(lab.ID)).ExecX(ctx)
	require.Equal(2, client.User.Query().Where(user.Edge.Groups.Has()).CountX(ctx))
	require.Equal(2, client.Group.Query().Where(group.Edge.Users.Has()).CountX(ctx))
	// validate relations.
	require.Equal(foo.Name, ent.QueryGroupUsers(client.Group, hub).OnlyX(ctx).Name, "hub has only one user")
	require.Equal(hub.Name, ent.QueryUserGroups(client.User, foo).OnlyX(ctx).Name, "foo is connected only to hub")
	require.Equal(bar.Name, ent.QueryGroupUsers(client.Group, lab).OnlyX(ctx).Name, "lab has only one user")
	require.Equal(lab.Name, ent.QueryUserGroups(client.User, bar).OnlyX(ctx).Name, "bar is connected only to lab")
	// add bar to hub.
	client.User.UpdateOne(bar).With(user.Edge.Groups.AddIDs(hub.ID)).ExecX(ctx)
	require.Equal(2, ent.QueryGroupUsers(client.Group, hub).CountX(ctx))
	require.Equal(1, ent.QueryGroupUsers(client.Group, lab).CountX(ctx))
	require.Equal([]string{bar.Name, foo.Name}, ent.QueryGroupUsers(client.Group, hub).Order(ent.Asc(user.FieldName)).GroupBy(user.FieldName).StringsX(ctx))
	require.Equal([]string{hub.Name, lab.Name}, ent.QueryUserGroups(client.User, bar).Order(ent.Asc(user.FieldName)).GroupBy(user.FieldName).StringsX(ctx))

	t.Log("query with side lookup from inverse")
	require.Equal(hub.Name, ent.QueryUserGroupsFromQuery(ent.QueryGroupUsers(client.Group, hub)).Where(group.Field.Name.EQ(hub.Name)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, ent.QueryGroupUsersFromQuery(ent.QueryUserGroupsFromQuery(ent.QueryGroupUsers(client.Group, lab)).Where(group.Not(group.Field.Name.EQ(hub.Name)))).OnlyX(ctx).Name, "should get its user")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, bar).Where(group.Field.Name.EQ(lab.Name))).OnlyX(ctx).Name, "should get itself")
	require.Equal(lab.Name, ent.QueryUserGroupsFromQuery(ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, bar).Where(group.Field.Name.EQ(lab.Name)))).Where(group.Field.Name.EQ(lab.Name)).OnlyX(ctx).Name, "should get its group")

	t.Log("query path from a user")
	require.Equal(
		hub.Name,
		ent.QueryUserGroupsFromQuery( // hub
			ent.QueryGroupUsersFromQuery( // foo (not having group with name "lab")
				ent.QueryUserGroups(client.User, bar).
					// hub.
					Where(group.Edge.Users.HasWith(user.Field.Name.EQ(foo.Name))),
			).
				Where(
					user.Not(user.Edge.Groups.HasWith(group.Field.Name.EQ(lab.Name))),
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
						Where(group.Edge.Users.HasWith(user.Field.Name.EQ(foo.Name))),
				).
					Where(
						user.Not(user.Edge.Groups.HasWith(group.Field.Name.EQ(lab.Name))),
					),
			),
		).
			Order(ent.Asc(user.FieldName)).
			// bar
			FirstX(ctx).Name,
	)
}
