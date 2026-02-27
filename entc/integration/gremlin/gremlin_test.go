// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gremlin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/entc/integration/gremlin/ent"
	"entgo.io/ent/entc/integration/gremlin/ent/card"
	"entgo.io/ent/entc/integration/gremlin/ent/file"
	"entgo.io/ent/entc/integration/gremlin/ent/group"
	"entgo.io/ent/entc/integration/gremlin/ent/groupinfo"
	"entgo.io/ent/entc/integration/gremlin/ent/node"
	"entgo.io/ent/entc/integration/gremlin/ent/pet"
	"entgo.io/ent/entc/integration/gremlin/ent/user"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestGremlin runs the sanity tests for the gremlin dialect.
//
// Note: every change for these tests should be applied also
// on the tests under the `integration` directory (The code
// is the same, the import path is different).
func TestGremlin(t *testing.T) {
	client, err := ent.Open("gremlin", "http://localhost:8182")
	require.NoError(t, err)
	defer client.Close()
	// run all tests except transaction and index tests.
	for _, tt := range tests[2:] {
		name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
		t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
			drop(t, client)
			tt(t, client)
		})
	}
}

var tests = []func(*testing.T, *ent.Client){
	Tx,
	Types,
	Clone,
	Sanity,
	Paging,
	Select,
	Delete,
	Relation,
	Predicate,
	AddValues,
	ClearFields,
	UniqueConstraint,
	O2OTwoTypes,
	O2OSameType,
	O2OSelfRef,
	O2MTwoTypes,
	O2MSameType,
	M2MSelfRef,
	M2MSameType,
	M2MTwoTypes,
	DefaultValue,
	ImmutableValue,
	Sensitive,
}

func Sanity(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	usr := client.User.Create().SetName("foo").SetAge(20).SaveX(ctx)
	require.Equal("foo", usr.Name)
	require.Equal(20, usr.Age)
	require.NotEmpty(usr.ID)
	client.User.Query().OnlyX(ctx)
	client.User.Delete().ExecX(ctx)
	require.Empty(client.User.Query().AllX(ctx))
	pt := client.Pet.Create().SetName("pedro").SaveX(ctx)
	usr = client.User.Create().SetName("foo").SetAge(20).AddPetIDs(pt.ID).SaveX(ctx)
	child := client.User.Create().SetName("bar").SetAge(20).AddChildIDs(usr.ID).SaveX(ctx)
	inf := client.GroupInfo.Create().SetDesc("desc").SaveX(ctx)
	grp := client.Group.Create().SetName("Github").SetExpire(time.Now()).AddUserIDs(usr.ID, child.ID).SetInfoID(inf.ID).SaveX(ctx)
	require.Equal(1, client.Group.Query().CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Active(false)).CountX(ctx))
	require.Len(client.Group.QueryUsers(grp).AllX(ctx), 2)
	client.User.QueryGroups(usr).OnlyX(ctx)
	client.User.QueryGroups(child).OnlyX(ctx)
	usr2 := client.User.Create().SetName("qux").SetAge(20).SetSpouseID(usr.ID).SaveX(ctx)
	client.User.QuerySpouse(usr2).OnlyX(ctx)
	client.User.QuerySpouse(usr).OnlyX(ctx)
	require.Equal(usr.Name, client.Pet.QueryOwner(client.User.QueryPets(usr).OnlyX(ctx)).OnlyX(ctx).Name)
	require.Equal(pt.Name, client.User.QueryPets(client.Pet.QueryOwner(client.User.QueryPets(usr).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name)
	require.Empty(client.User.QueryPets(client.User.QuerySpouse(usr).OnlyX(ctx)).AllX(ctx))
	require.Equal(pt.Name, client.User.QueryPets(client.User.QuerySpouse(usr2).OnlyX(ctx)).OnlyX(ctx).Name)
	require.Len(client.Group.QueryUsers(client.User.QueryGroups(usr).OnlyX(ctx)).AllX(ctx), 2)
	require.Len(client.User.QueryGroups(client.Group.QueryUsers(client.User.QueryGroups(usr).OnlyX(ctx)).OnlyX(ctx)).AllX(ctx), 1, "should be unique by default")
	require.Len(client.User.QueryGroups(usr).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.HasPets()).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.HasSpouse()).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Not(user.HasSpouse())).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.HasGroups()).AllX(ctx), 2)
	require.Len(client.Group.Query().Where(group.HasUsers()).AllX(ctx), 1)
	require.Len(client.Group.Query().Where(group.HasUsersWith(user.Name("foo"))).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.HasGroupsWith(group.NameHasPrefix("G"))).AllX(ctx), 2)
	require.Equal(3, client.User.Query().CountX(ctx))
	require.Equal(client.Group.Query().Where(group.HasUsersWith(user.Name("foo"))).CountX(ctx), 1)
	require.True(client.User.Query().ExistX(ctx))
	require.True(client.User.Query().Where(user.HasPetsWith(pet.NameHasPrefix("ped"))).ExistX(ctx))
	require.False(client.User.Query().Where(user.HasPetsWith(pet.NameHasPrefix("pan"))).ExistX(ctx))
	require.Equal(child.Name, client.User.Query().Order(ent.Asc("name")).FirstX(ctx).Name)
	require.Equal(usr2.Name, client.User.Query().Order(ent.Desc("name")).FirstX(ctx).Name)
	// update fields.
	client.User.Update().Where(user.ID(child.ID)).SetName("Ariel").SaveX(ctx)
	client.User.Query().Where(user.Name("Ariel")).OnlyX(ctx)
	// update edges.
	require.Empty(client.User.QueryPets(child).AllX(ctx))
	require.NoError(client.Pet.UpdateOne(pt).ClearOwner().Exec(ctx))
	client.User.Update().Where(user.ID(child.ID)).AddPetIDs(pt.ID).SaveX(ctx)
	require.NotEmpty(client.User.QueryPets(child).AllX(ctx))
	client.User.Update().Where(user.ID(child.ID)).RemovePetIDs(pt.ID).SaveX(ctx)
	require.Empty(client.User.QueryPets(child).AllX(ctx))
	// remove edges.
	client.User.Update().ClearSpouse().SaveX(ctx)
	require.Empty(client.User.Query().Where(user.HasSpouse()).AllX(ctx))
	client.User.Update().AddFriendIDs(child.ID).RemoveGroupIDs(grp.ID).Where(user.ID(usr.ID)).SaveX(ctx)
	require.NotEmpty(client.User.QueryGroups(child).AllX(ctx))
	require.Empty(client.User.QueryGroups(usr).AllX(ctx))
	require.Len(client.User.QueryFriends(child).AllX(ctx), 1)
	require.Len(client.User.QueryFriends(usr).AllX(ctx), 1)
	// update one vertex.
	usr = client.User.UpdateOne(usr).SetName("baz").AddGroupIDs(grp.ID).SaveX(ctx)
	require.Equal("baz", usr.Name)
	require.NotEmpty(client.User.QueryGroups(usr).AllX(ctx))
	id := uuid.NewString()
	it := client.Item.Create().SetID(id).SaveX(ctx)
	require.Equal(id, it.ID)

	// grouping.
	var v []struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Sum   int    `json:"sum"`
		Count int    `json:"count"`
	}
	client.User.Query().
		GroupBy(user.FieldName, user.FieldAge).
		Aggregate(ent.Count(), ent.Sum(user.FieldAge)).
		ScanX(ctx, &v)
	require.NotEmpty(v)
	// IN predicates.
	ids := client.User.Query().IDsX(ctx)
	require.Len(ids, 3)
	client.User.Delete().Where(user.IDIn(ids...)).ExecX(ctx)
	ids = client.User.Query().IDsX(ctx)
	require.Empty(ids)
	// nop.
	client.User.Delete().Where(user.IDIn(ids...)).ExecX(ctx)
}

func Clone(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	f1 := client.File.Create().SetName("foo").SetSize(10).SaveX(ctx)
	f2 := client.File.Create().SetName("foo").SetSize(20).SaveX(ctx)
	base := client.File.Query().Where(file.Name("foo"))
	require.Equal(t, f1.Size, base.Clone().Where(file.Size(f1.Size)).OnlyX(ctx).Size)
	require.Equal(t, f2.Size, base.Clone().Where(file.Size(f2.Size)).OnlyX(ctx).Size)
	// ensure clone emits valid code.
	query := client.User.Query().Where(user.Name("unknown"))
	for i := 0; i < 10; i++ {
		_, err := query.Clone().Where(user.Name("unknown")).First(ctx)
		require.True(t, ent.IsNotFound(err), "should not return syntax error")
	}
}

func Paging(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	for i := 1; i <= 10; i++ {
		client.User.Create().SetName(fmt.Sprintf("name-%d", i)).SetAge(i).SaveX(ctx)
	}

	require.Equal(10, client.User.Query().CountX(ctx))
	require.Len(client.User.Query().Offset(5).AllX(ctx), 5)
	require.Len(client.User.Query().Offset(6).AllX(ctx), 4)
	require.Equal(
		[]int{7, 8},
		client.User.Query().
			Offset(6).
			Limit(2).
			Order(ent.Asc(user.FieldAge)).
			GroupBy(user.FieldAge).
			IntsX(ctx),
	)
	for i := 0; i < 10; i++ {
		require.Equal(i+1, client.User.Query().Order(ent.Asc(user.FieldAge)).Offset(i).Limit(1).AllX(ctx)[0].Age)
	}
}

func Select(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	t.Log("select one field")
	client.User.Create().SetName("foo").SetAge(30).SaveX(ctx)
	names := client.User.
		Query().
		Select(user.FieldName).
		StringsX(ctx)
	require.Equal([]string{"foo"}, names)
	client.User.Create().SetName("bar").SetAge(30).SaveX(ctx)
	t.Log("select one field with ordering")
	names = client.User.
		Query().
		Order(ent.Asc(user.FieldName)).
		Select(user.FieldName).
		StringsX(ctx)
	require.Equal([]string{"bar", "foo"}, names)
	names = client.User.
		Query().
		Order(ent.Desc(user.FieldName)).
		Select(user.FieldName).
		StringsX(ctx)
	require.Equal([]string{"foo", "bar"}, names)
	client.User.Create().SetName("baz").SetAge(30).SaveX(ctx)
	names = client.User.
		Query().
		Order(ent.Asc(user.FieldName)).
		Select(user.FieldName).
		StringsX(ctx)
	require.Equal([]string{"bar", "baz", "foo"}, names)

	t.Log("select 2 fields")
	var v []struct {
		Age  int    `json:"age"`
		Name string `json:"name"`
	}
	client.User.
		Query().
		Order(ent.Asc(user.FieldName)).
		Select(user.FieldAge, user.FieldName).
		ScanX(ctx, &v)
	require.Equal([]int{30, 30, 30}, []int{v[0].Age, v[1].Age, v[2].Age})
	require.Equal([]string{"bar", "baz", "foo"}, []string{v[0].Name, v[1].Name, v[2].Name})

	a8m := client.User.Create().SetName("Ariel").SetNickname("a8m").SetAge(30).SaveX(ctx)
	require.NotEmpty(a8m.ID)
	require.NotEmpty(a8m.Age)
	require.NotEmpty(a8m.Name)
	require.NotEmpty(a8m.Nickname)
	a8m = client.User.UpdateOne(a8m).SetAge(32).Select(user.FieldAge).SaveX(ctx)
	require.NotEmpty(a8m.ID)
	require.NotEmpty(a8m.Age)
	require.Empty(a8m.Name)
	require.Empty(a8m.Nickname)
}

func Predicate(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	f1 := client.File.Create().SetName("1").SetSize(10).SaveX(ctx)
	f2 := client.File.Create().SetName("2").SetSize(20).SaveX(ctx)
	f3 := client.File.Create().SetName("3").SetSize(30).SaveX(ctx)
	f4 := client.File.Create().SetName("4").SetSize(40).SaveX(ctx)
	files := client.File.Query().
		Where(
			file.Or(
				file.Name(f1.Name),
				file.And(file.Name(f2.Name), file.Size(f2.Size)),
			),
		).
		Order(ent.Asc(file.FieldName)).
		AllX(ctx)
	require.Equal(f1.Name, files[0].Name)
	require.Equal(f2.Name, files[1].Name)

	match := client.File.Query().
		Where(file.Or(file.Name(f1.Name), file.Name(f2.Name))).
		Where(file.Size(f1.Size)).
		OnlyX(ctx)
	require.Equal(f1.Name, match.Name)

	match = client.File.Query().
		Where(file.Size(f2.Size)).
		Where(file.Or(file.Name(f1.Name), file.Name(f2.Name))).
		OnlyX(ctx)
	require.Equal(f2.Name, match.Name)

	files = client.File.Query().
		Where(file.Or(file.Size(f3.Size), file.Size(f4.Size))).
		Where(file.Or(file.Name(f3.Name), file.Name(f4.Name))).
		Where(file.Not(file.Or(file.Name(f1.Name), file.Size(f1.Size)))).
		Order(ent.Asc(file.FieldName)).
		AllX(ctx)
	require.Equal(f3.Name, files[0].Name)
	require.Equal(f4.Name, files[1].Name)

	files = client.File.Query().
		Where(
			file.Or(
				file.Name(f4.Name),
				file.And(file.Name(f3.Name), file.Size(f3.Size)),
			),
		).
		Order(ent.Asc(file.FieldName)).
		AllX(ctx)
	require.Equal(f3.Name, files[0].Name)
	require.Equal(f4.Name, files[1].Name)

	require.Zero(client.File.Query().Where(file.UserNotNil()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.UserIsNil()).CountX(ctx))
	require.Zero(client.File.Query().Where(file.GroupNotNil()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.GroupIsNil()).CountX(ctx))

	f1 = client.File.UpdateOne(f1).SetUser("a8m").SaveX(ctx)
	require.NotNil(f1.User)
	require.Equal("a8m", *f1.User)
	require.Equal(3, client.File.Query().Where(file.UserIsNil()).CountX(ctx))
	require.Equal(f1.Name, client.File.Query().Where(file.UserNotNil()).OnlyX(ctx).Name)
	f5 := client.File.Create().SetName("5").SetSize(40).SetUser("mashraki").SaveX(ctx)
	require.NotNil(f5.User)
	require.Equal("mashraki", *f5.User)
	require.Equal(3, client.File.Query().Where(file.UserIsNil()).CountX(ctx))
	require.Equal(2, client.File.Query().Where(file.UserNotNil()).CountX(ctx))

	require.Equal(5, client.File.Query().Where(file.GroupIsNil()).CountX(ctx))
	f4 = client.File.UpdateOne(f4).SetGroup("fbc").SaveX(ctx)
	require.Equal(1, client.File.Query().Where(file.GroupNotNil()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.GroupIsNil()).CountX(ctx))
	require.Equal(
		5,
		client.File.Query().
			Where(
				file.Or(
					file.GroupIsNil(),
					file.And(
						file.GroupNotNil(),
						file.Name(f4.Name),
					),
				),
			).
			CountX(ctx),
	)
}

func AddValues(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	t.Log("add values to fields")
	cmt := client.Comment.Create().SetUniqueInt(1).SetUniqueFloat(1).SaveX(ctx)
	cmt = client.Comment.UpdateOne(cmt).AddUniqueInt(10).SaveX(ctx)
	require.Equal(11, cmt.UniqueInt)
	require.Equal(11, client.Comment.Query().OnlyX(ctx).UniqueInt, "should be updated in the database")
	t.Log("add values to null fields")
	cmt = client.Comment.UpdateOne(cmt).AddNillableInt(10).SaveX(ctx)
	require.Equal(10, *cmt.NillableInt)

	cmt1 := client.Comment.Create().SetUniqueInt(1).SetUniqueFloat(10).SaveX(ctx)
	err := client.Comment.UpdateOne(cmt1).AddUniqueInt(10).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	cmt1 = client.Comment.UpdateOne(cmt1).AddUniqueInt(20).AddNillableInt(20).SaveX(ctx)
	require.Equal(21, cmt1.UniqueInt)
	require.Equal(20, *cmt1.NillableInt)

	cmt1 = client.Comment.UpdateOne(cmt1).AddUniqueInt(10).AddUniqueInt(-1).SaveX(ctx)
	require.Equal(30, cmt1.UniqueInt)
	require.Equal(30, client.Comment.GetX(ctx, cmt1.ID).UniqueInt)
}

func Delete(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	nd := client.Node.Create().SetValue(1e3).SaveX(ctx)
	err := client.Node.DeleteOneID(nd.ID).Exec(ctx)
	require.NoError(err)
	err = client.Node.DeleteOneID(nd.ID).Exec(ctx)
	require.True(ent.IsNotFound(err))

	for i := 0; i < 5; i++ {
		client.Node.Create().SetValue(i).SaveX(ctx)
	}
	affected, err := client.Node.Delete().Where(node.ValueGT(2)).Exec(ctx)
	require.NoError(err)
	require.Equal(2, affected)

	affected, err = client.Node.Delete().Exec(ctx)
	require.NoError(err)
	require.Equal(3, affected)
}

func Relation(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	t.Log("querying group info")
	info, err := client.GroupInfo.
		Query().
		First(ctx)
	require.Nil(info)
	require.True(ent.IsNotFound(err))

	t.Log("creating group info")
	info = client.GroupInfo.
		Create().
		SetDesc("group info").
		SaveX(ctx)
	t.Logf("group info created: %v", info)

	t.Log("creating group")
	grp := client.Group.
		Create().
		SetInfoID(info.ID).
		SetName("Github").
		SetExpire(time.Now().Add(time.Hour)).
		SaveX(ctx)
	require.NotZero(grp.ID)
	require.Equal(grp.MaxUsers, 10)
	require.Equal(grp.Name, "Github")
	t.Logf("group created: %v", grp)

	t.Log("creating user")
	usr := client.User.
		Create().
		SetAge(20).
		SetName("a8m").
		AddGroupIDs(grp.ID).
		SaveX(ctx)
	require.NotZero(usr.ID)
	require.Equal(usr.Age, 20)
	require.Equal(usr.Name, "a8m")
	require.Equal(usr.Last, "unknown")
	t.Logf("user created: %v", usr)

	t.Log("querying assoc edges")
	groups := client.User.QueryGroups(usr).IDsX(ctx)
	require.NotEmpty(groups)
	require.Equal(grp.ID, groups[0])
	t.Log("querying inverse edge")
	users := client.Group.QueryUsers(grp).IDsX(ctx)
	require.NotEmpty(users)
	require.Equal(usr.ID, users[0])

	t.Log("remove group edge")
	client.User.UpdateOne(usr).RemoveGroupIDs(grp.ID).ExecX(ctx)
	require.Empty(client.Group.QueryUsers(grp).AllX(ctx))
	require.Empty(client.User.QueryGroups(usr).AllX(ctx))
	t.Logf("add group edge")
	client.User.UpdateOne(usr).AddGroupIDs(grp.ID).ExecX(ctx)
	require.NotEmpty(client.Group.QueryUsers(grp).AllX(ctx))
	require.NotEmpty(client.User.QueryGroups(usr).AllX(ctx))
	t.Log("remove users inverse edge")
	client.Group.UpdateOne(grp).RemoveUserIDs(usr.ID).ExecX(ctx)
	require.Empty(client.Group.QueryUsers(grp).AllX(ctx))
	require.Empty(client.User.QueryGroups(usr).AllX(ctx))
	t.Logf("add group inverse edge")
	client.Group.UpdateOne(grp).AddUserIDs(usr.ID).ExecX(ctx)
	require.NotEmpty(client.Group.QueryUsers(grp).AllX(ctx))
	require.NotEmpty(client.User.QueryGroups(usr).AllX(ctx))

	t.Log("count vertices")
	require.Equal(1, client.User.Query().CountX(ctx))
	require.Equal(1, client.Group.Query().CountX(ctx))

	t.Log("get only vertices")
	require.NotNil(client.User.Query().OnlyX(ctx))
	require.NotNil(client.Group.Query().OnlyX(ctx))

	t.Log("get only ids")
	require.NotEmpty(client.User.Query().OnlyIDX(ctx))
	require.NotEmpty(client.Group.Query().OnlyIDX(ctx))

	t.Log("query spouse edge")
	require.Zero(client.User.Query().Where(user.HasSpouse()).CountX(ctx))
	neta := client.User.Create().SetName("neta").SetAge(18).SetSpouseID(usr.ID).SaveX(ctx)
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("check for singular error")
	_, err = client.User.Query().Only(ctx)
	require.True(ent.IsNotSingular(err))

	t.Log("query parent/children edges")
	require.False(client.User.QueryParent(usr).ExistX(ctx))
	require.Empty(client.User.QueryChildren(usr).AllX(ctx))
	child := client.User.Create().SetName("pedro").SetAge(7).SetParentID(usr.ID).SaveX(ctx)
	require.Equal(usr.Name, client.User.QueryParent(child).OnlyX(ctx).Name)
	require.Equal(child.Name, client.User.QueryChildren(usr).OnlyX(ctx).Name)
	require.False(client.User.QueryParent(usr).ExistX(ctx))

	t.Log("clear parent edge")
	brat := client.User.Create().SetName("brat").SetAge(19).SetParentID(usr.ID).SaveX(ctx)
	require.Equal(2, client.User.QueryChildren(usr).CountX(ctx))
	brat = client.User.UpdateOne(brat).ClearParent().SaveX(ctx)
	_, err = client.User.UpdateOne(brat).ClearParent().Save(ctx)
	require.NoError(err)
	require.False(client.User.QueryParent(brat).ExistX(ctx))
	require.Equal(1, client.User.QueryChildren(usr).CountX(ctx))

	t.Log("delete child clears edge")
	brat = client.User.UpdateOne(brat).SetParentID(usr.ID).SaveX(ctx)
	require.Equal(2, client.User.QueryChildren(usr).CountX(ctx))
	client.User.DeleteOne(brat).ExecX(ctx)
	require.Equal(1, client.User.QueryChildren(usr).CountX(ctx))

	client.Group.UpdateOne(grp).AddBlockedIDs(neta.ID).SaveX(ctx)
	blocked := client.Group.QueryBlocked(client.User.QueryGroups(usr).OnlyX(ctx)).OnlyX(ctx)
	t.Log("blocked:", blocked)

	t.Log("query users with or condition")
	require.Len(client.User.Query().Where(user.Or(user.Name("a8m"), user.Name("neta"))).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Or(user.Name("a8m"), user.Name("noam"))).AllX(ctx), 1)
	require.Zero(client.User.Query().Where(user.Or(user.Name("alex"), user.Name("noam"))).AllX(ctx))

	t.Log("query using the in predicate")
	require.Len(client.User.Query().Where(user.NameIn("a8m", "neta")).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.NameIn("a8m", "alex")).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.IDIn(neta.ID)).AllX(ctx), 1)

	t.Log("query existence")
	require.True(client.User.Query().Where(user.Name("a8m")).Exist(ctx))
	require.False(client.User.Query().Where(user.Name("alex")).Exist(ctx))

	t.Log("query using get")
	require.Equal(usr.Name, client.User.GetX(ctx, usr.ID).Name)
	uid, err := client.User.Query().Where(user.ID(usr.ID), user.Not(user.Name(usr.Name))).Only(ctx)
	require.Error(err)
	require.Nil(uid)

	t.Log("test validators")
	_, err = client.Group.Create().SetInfoID(info.ID).SetType("a").SetName("Gituhb").SetExpire(time.Now().Add(time.Hour)).Save(ctx)
	require.Error(err, "type validator failed")
	_, err = client.Group.Create().SetInfoID(info.ID).SetType("pass").SetName("failed").SetExpire(time.Now().Add(time.Hour)).Save(ctx)
	require.Error(err, "name validator failed")
	_, err = client.Group.Create().SetInfoID(info.ID).SetType("pass").SetName("Github20").SetExpire(time.Now().Add(time.Hour)).Save(ctx)
	require.Error(err, "name validator failed")
	_, err = client.Group.Create().SetInfoID(info.ID).SetType("pass").SetName("Github").SetMaxUsers(-1).SetExpire(time.Now().Add(time.Hour)).Save(ctx)
	require.Error(err, "max_users validator failed")
	_, err = client.Group.Update().SetMaxUsers(-10).Save(ctx)
	require.Error(err, "max_users validator failed")
	_, err = client.Group.UpdateOne(grp).SetMaxUsers(-10).Save(ctx)
	require.Error(err, "max_users validator failed")

	t.Log("query using edge-with predicate")
	require.Len(client.User.QueryGroups(usr).Where(group.HasInfoWith(groupinfo.Desc("group info"))).AllX(ctx), 1)
	require.Empty(client.User.QueryGroups(usr).Where(group.HasInfoWith(groupinfo.Desc("missing info"))).AllX(ctx))
	t.Log("query using edge-with predicate on inverse edges")
	require.Len(client.Group.Query().Where(group.Name("Github"), group.HasUsersWith(user.Name("a8m"))).AllX(ctx), 1)
	require.Empty(client.Group.Query().Where(group.Name("Github"), group.HasUsersWith(user.Name("alex"))).AllX(ctx))
	t.Logf("query path using edge-with predicate")
	require.Len(client.GroupInfo.Query().Where(groupinfo.HasGroupsWith(group.HasUsersWith(user.Name("a8m")))).AllX(ctx), 1)
	require.Empty(client.GroupInfo.Query().Where(groupinfo.HasGroupsWith(group.HasUsersWith(user.Name("alex")))).AllX(ctx))
	require.Len(client.GroupInfo.Query().Where(groupinfo.Or(groupinfo.Desc("group info"), groupinfo.HasGroupsWith(group.HasUsersWith(user.Name("alex"))))).AllX(ctx), 1)

	t.Log("query with ordering")
	u1 := client.User.Query().Order(ent.Asc(user.FieldName)).FirstIDX(ctx)
	u2 := client.User.Query().Order(ent.Desc(user.FieldName)).FirstIDX(ctx)
	require.NotEqual(u1, u2)
	u1 = client.User.Query().Order(ent.Asc(user.FieldLast), ent.Asc(user.FieldAge)).FirstIDX(ctx)
	u2 = client.User.Query().Order(ent.Asc(user.FieldLast), ent.Desc(user.FieldAge)).FirstIDX(ctx)
	require.NotEqual(u1, u2)
	u1 = client.User.Query().Order(ent.Asc(user.FieldName, user.FieldAge)).FirstIDX(ctx)
	u2 = client.User.Query().Order(ent.Asc(user.FieldName, user.FieldAge)).FirstIDX(ctx)
	require.Equal(u1, u2)

	t.Log("query path")
	require.Len(client.User.Query().Where(user.HasGroups()).AllX(ctx), 1)
	require.Empty(client.User.Query().Where(user.HasGroupsWith(group.Name("boring"))).AllX(ctx))
	require.Equal(neta.Name, client.User.QuerySpouse(client.Group.QueryUsers(client.User.QueryGroups(usr).Where(group.Name("Github")).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name)
	require.Empty(client.Group.Query().Where(group.HasInfoWith(groupinfo.Desc("group info")), group.Name("boring")).AllX(ctx))
	require.Equal(child.Name, client.User.QueryChildren(client.Group.QueryUsers(client.GroupInfo.QueryGroups(client.GroupInfo.Query().Where(groupinfo.Desc("group info")).OnlyX(ctx)).Where(group.Name("Github")).OnlyX(ctx)).OnlyX(ctx)).FirstX(ctx).Name)

	t.Log("query using string predicate")
	require.Len(client.User.Query().Where(user.NameIn("a8m", "neta", "pedro")).AllX(ctx), 3)
	require.Empty(client.User.Query().Where(user.NameNotIn("a8m", "neta", "pedro")).AllX(ctx))
	require.Empty(client.User.Query().Where(user.NameIn("alex", "rocket")).AllX(ctx))
	require.NotNil(client.User.Query().Where(user.HasParentWith(user.NameIn("a8m", "neta"))).OnlyX(ctx))
	require.Len(client.User.Query().Where(user.NameContains("a8")).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.NameHasPrefix("a8")).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Or(user.NameHasPrefix("a8"), user.NameHasSuffix("eta"))).AllX(ctx), 2)

	t.Log("group-by one field")
	names, err := client.User.Query().GroupBy(user.FieldName).Strings(ctx)
	require.NoError(err)
	sort.Strings(names)
	require.Equal([]string{"a8m", "neta", "pedro"}, names)
	ages, err := client.User.Query().GroupBy(user.FieldAge).Ints(ctx)
	require.NoError(err)
	require.Len(ages, 3)

	t.Log("group-by two fields with aggregation")
	client.User.Create().SetName(usr.Name).SetAge(usr.Age).SaveX(ctx)
	client.User.Create().SetName(neta.Name).SetAge(neta.Age).SaveX(ctx)
	child2 := client.User.Create().SetName(child.Name).SetAge(child.Age + 1).SaveX(ctx)
	var v []struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Sum   int    `json:"sum"`
		Count int    `json:"count"`
	}
	client.User.Query().
		GroupBy(user.FieldName, user.FieldAge).
		Aggregate(ent.Count(), ent.Sum(user.FieldAge)).
		ScanX(ctx, &v)
	require.Len(v, 4)
	sort.Slice(v, func(i, j int) bool {
		if v[i].Name != v[j].Name {
			return v[i].Name < v[j].Name
		}
		return v[i].Age < v[j].Age
	})
	for i, usr := range []*ent.User{usr, neta} {
		require.Equal(usr.Name, v[i].Name)
		require.Equal(usr.Age, v[i].Age)
		require.Equal(usr.Age*2, v[i].Sum)
		require.Equal(2, v[i].Count, "should have 2 vertices")
	}
	v = v[2:]
	for i, usr := range []*ent.User{child, child2} {
		require.Equal(usr.Name, v[i].Name)
		require.Equal(usr.Age, v[i].Age)
		require.Equal(usr.Age, v[i].Sum)
		require.Equal(1, v[i].Count)
	}

	t.Log("group by with .as modulator")
	var v2 []struct {
		Name  string `json:"name"`
		Total int    `json:"total"`
	}
	client.User.Query().GroupBy(user.FieldName).Aggregate(ent.As(ent.Count(), "total")).ScanX(ctx, &v2)
	require.Len(v2, 3)
	for i := range v2 {
		require.Equal(2, v2[i].Total)
	}
}

func ClearFields(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	img := client.File.Create().SetName("foo").SetSize(100).SetUser("a8m").SetGroup("Github").SaveX(ctx)

	t.Log("clear one field")
	img = client.File.UpdateOne(img).ClearUser().SaveX(ctx)
	require.Nil(t, img.User)
	img = client.File.Query().OnlyX(ctx)
	require.Nil(t, img.User)
	require.Equal(t, "Github", img.Group)

	t.Log("clear many fields")
	img = client.File.UpdateOne(img).ClearUser().ClearGroup().SaveX(ctx)
	require.Nil(t, img.User)
	img = client.File.Query().OnlyX(ctx)
	require.Nil(t, img.User)
	require.Empty(t, img.Group)

	t.Log("revert previous set")
	img = client.File.UpdateOne(img).SetUser("a8m").ClearUser().SaveX(ctx)
	require.Nil(t, img.User)
}

func UniqueConstraint(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("unique constraint violation on 1 field")
	foo := client.User.Create().SetAge(1).SetName("foo").SetNickname("baz").SaveX(ctx)
	_, err := client.User.Create().SetAge(1).SetName("bar").SetNickname("baz").Save(ctx)
	require.True(ent.IsConstraintError(err))
	bar := client.User.Create().SetAge(1).SetName("bar").SetNickname("bar").SetPhone("1").SaveX(ctx)

	t.Log("unique constraint violation on 2 fields")
	_, err = client.User.Create().SetAge(1).SetName("baz").SetNickname("bar").SetPhone("1").Save(ctx)
	require.True(ent.IsConstraintError(err))
	_, err = client.User.Create().SetAge(1).SetName("baz").SetNickname("qux").SetPhone("1").Save(ctx)
	require.True(ent.IsConstraintError(err))
	_, err = client.User.Create().SetAge(1).SetName("baz").SetNickname("bar").SetPhone("2").Save(ctx)
	require.True(ent.IsConstraintError(err))
	client.User.Create().SetAge(1).SetName("baz").SetNickname("qux").SetPhone("2").SaveX(ctx)
	_, err = client.User.UpdateOne(foo).SetNickname("bar").SetPhone("1").Save(ctx)
	require.True(ent.IsConstraintError(err))
	_, err = client.User.UpdateOne(foo).SetNickname("bar").SetPhone("2").Save(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2o unique constraint on creation")
	dan := client.User.Create().SetAge(1).SetName("dan").SetNickname("dan").SetSpouseID(foo.ID).SaveX(ctx)
	require.Equal(dan.Name, client.User.QuerySpouse(foo).OnlyX(ctx).Name)
	_, err = client.User.Create().SetAge(1).SetName("b").SetSpouseID(foo.ID).Save(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2m/m2o unique constraint on creation")
	c1 := client.User.Create().SetAge(1).SetName("c1").SetNickname("c1").SetParentID(foo.ID).SaveX(ctx)
	c2 := client.User.Create().SetAge(1).SetName("c2").SetNickname("c2").SetParentID(foo.ID).SaveX(ctx)
	_, err = client.User.Create().SetAge(10).SetName("z").SetNickname("z").AddChildIDs(c1.ID).Save(ctx)
	require.True(ent.IsConstraintError(err), "c1 already has a parent")
	_, err = client.User.Create().SetAge(10).SetName("z").SetNickname("z").AddChildIDs(c2.ID).Save(ctx)
	require.True(ent.IsConstraintError(err), "c2 already has a parent")
	_, err = client.User.Create().SetAge(10).SetName("z").SetNickname("z").AddChildIDs(c1.ID, c2.ID).Save(ctx)
	require.True(ent.IsConstraintError(err))

	inf := client.GroupInfo.Create().SetDesc("desc").SaveX(ctx)
	grp := client.Group.Create().SetName("Github").SetExpire(time.Now()).SetInfoID(inf.ID).SaveX(ctx)
	_, err = client.GroupInfo.Create().SetDesc("desc").AddGroupIDs(grp.ID).Save(ctx)
	require.True(ent.IsConstraintError(err))

	p1 := client.Pet.Create().SetName("p1").SetOwnerID(foo.ID).SaveX(ctx)
	p2 := client.Pet.Create().SetName("p2").SetOwnerID(foo.ID).SaveX(ctx)
	_, err = client.User.Create().SetAge(10).SetName("new-owner").AddPetIDs(p1.ID, p2.ID).Save(ctx)
	require.True(ent.IsConstraintError(err))

	err = client.User.UpdateOne(c2).SetNickname(c1.Nickname).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2o unique constraint on update")
	err = client.User.UpdateOne(bar).SetSpouseID(foo.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(foo).SetSpouseID(bar.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	client.User.UpdateOne(bar).ClearSpouse().ExecX(ctx)
	client.User.UpdateOne(foo).ClearSpouse().SetSpouseID(bar.ID).ExecX(ctx)
	require.False(client.User.QuerySpouse(dan).ExistX(ctx))
	require.Equal(bar.Name, client.User.QuerySpouse(foo).OnlyX(ctx).Name)
	require.Equal(foo.Name, client.User.QuerySpouse(bar).OnlyX(ctx).Name)

	t.Log("o2m unique constraint on update")
	_, err = client.User.UpdateOne(bar).SetAge(1).SetName("new-owner").AddPetIDs(p1.ID).Save(ctx)
	require.True(ent.IsConstraintError(err))
	_, err = client.User.UpdateOne(bar).SetAge(1).SetName("new-owner").AddPetIDs(p1.ID, p2.ID).Save(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("unique constraint violation when updating more than 1 vertex")
	err = client.User.Update().SetNickname("yada").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	require.False(client.User.Query().Where(user.Nickname("yada")).ExistX(ctx))
	client.User.Update().Where(user.Nickname("dan")).SetNickname("yada").ExecX(ctx)
	require.False(client.User.Query().Where(user.Nickname("dan")).ExistX(ctx))
	require.True(client.User.Query().Where(user.Nickname("yada")).ExistX(ctx))

	t.Log("unique constraint on numeric fields")
	cm1 := client.Comment.Create().SetUniqueInt(42).SetUniqueFloat(math.Pi).SaveX(ctx)
	_, err = client.Comment.Create().SetUniqueInt(42).SetUniqueFloat(math.E).Save(ctx)
	require.Error(err)
	_, err = client.Comment.Create().SetUniqueInt(7).SetUniqueFloat(math.Pi).Save(ctx)
	require.Error(err)
	_ = client.Comment.Create().SetUniqueInt(7).SetUniqueFloat(math.E).SaveX(ctx)
	err = client.Comment.UpdateOne(cm1).SetUniqueInt(7).Exec(ctx)
	require.Error(err)
	err = client.Comment.UpdateOne(cm1).SetUniqueFloat(math.E).Exec(ctx)
	require.Error(err)
}

func Tx(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	tx, err := client.Tx(ctx)
	require.NoError(err)
	tx.Node.Create().SaveX(ctx)

	require.NoError(tx.Rollback())
	require.Zero(client.Node.Query().CountX(ctx), "rollback should discard all changes")

	tx, err = client.Tx(ctx)
	require.NoError(err)

	nde := tx.Node.Create().SaveX(ctx)

	require.NoError(tx.Commit())
	require.Error(tx.Commit(), "should return an error on the second call")
	require.NotZero(client.Node.Query().CountX(ctx), "commit should save all changes")
	_, err = client.Node.QueryNext(nde).Count(ctx)
	require.Error(err, "should not be able to query after tx was closed")
	require.Zero(client.Node.QueryNext(client.Node.GetX(ctx, nde.ID)).CountX(ctx), "should be able to query the entity after wrap")

	tx, err = client.Tx(ctx)
	require.NoError(err)
	_, err = tx.Client().Tx(ctx)
	require.Error(err, "cannot start a transaction within a transaction")
	require.NoError(tx.Rollback())
}

func DefaultValue(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	c1 := client.Card.Create().SetNumber("102030").SetName("Firstname Lastname").SaveX(ctx)
	ctime, mtime := c1.CreateTime, c1.UpdateTime
	require.False(t, ctime.IsZero())
	require.False(t, mtime.IsZero())
	c1 = client.Card.UpdateOne(c1).SetName("F Lastname").SaveX(ctx)
	require.False(t, c1.CreateTime.IsZero())
	require.False(t, c1.UpdateTime.IsZero())
	require.False(t, mtime.Equal(c1.UpdateTime))
}

func ImmutableValue(t *testing.T, client *ent.Client) {
	tests := []struct {
		name    string
		updater func() any
	}{
		{
			name: "Update",
			updater: func() any {
				return client.Card.Update()
			},
		},
		{
			name: "UpdateOne",
			updater: func() any {
				return client.Card.UpdateOne(client.Card.Create().SetNumber("42").SaveX(context.Background()))
			},
		},
	}
	for _, tc := range tests {
		v := reflect.ValueOf(tc.updater())
		require.False(t, v.MethodByName("SetCreatedAt").IsValid())
		require.False(t, v.MethodByName("SetNillableCreatedAt").IsValid())
		require.False(t, v.MethodByName("SetNumber").IsValid())
		require.True(t, v.MethodByName("SetName").IsValid())
	}
}

func Sensitive(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	usr := client.User.Create().SetName("foo").SetAge(20).SetPassword("secret-password").SaveX(ctx)
	require.Equal("secret-password", usr.Password)
	require.Contains(usr.String(), "password=<sensitive>")
	b, err := json.Marshal(usr)
	require.NoError(err)
	require.NotContains(string(b), "secret-password")
}

// Demonstrate a O2O relation between two different types. A User and a CreditCard.
// The user is the owner of the edge, named "owner", and the card has an inverse edge
// named "owner" that points to the User.
func O2OTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without card")
	usr := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.Zero(client.User.QueryCard(usr).CountX(ctx))

	t.Log("add card to user on card creation (inverse creation)")
	crd := client.Card.Create().SetNumber("1").SetOwnerID(usr.ID).SaveX(ctx)
	require.Equal(client.User.QueryCard(usr).CountX(ctx), 1)
	require.Equal(client.Card.QueryOwner(crd).CountX(ctx), 1)

	t.Log("delete inverse should delete association")
	client.Card.DeleteOne(crd).ExecX(ctx)
	require.Zero(client.Card.Query().CountX(ctx))
	require.Zero(client.User.QueryCard(usr).CountX(ctx), "user should not have card")

	t.Log("add card to user by updating user (the owner of the edge)")
	crd = client.Card.Create().SetNumber("10").SaveX(ctx)
	client.User.UpdateOne(usr).SetCardID(crd.ID).ExecX(ctx)
	require.Equal(usr.Name, client.Card.QueryOwner(crd).OnlyX(ctx).Name)
	require.Equal(crd.Number, client.User.QueryCard(usr).OnlyX(ctx).Number)

	t.Log("delete assoc should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.Zero(client.Card.QueryOwner(crd).CountX(ctx), "card should not have an owner")

	t.Log("add card to user by updating card (the inverse edge)")
	usr = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.Card.UpdateOne(crd).SetOwnerID(usr.ID).ExecX(ctx)
	require.Equal(usr.Name, client.Card.QueryOwner(crd).OnlyX(ctx).Name)
	require.Equal(crd.Number, client.User.QueryCard(usr).OnlyX(ctx).Number)

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
	require.Equal(crd.Number, client.User.QueryCard(client.Card.QueryOwner(crd).OnlyX(ctx)).OnlyX(ctx).Number, "should get itself")
	require.Equal(usr.Name, client.Card.QueryOwner(client.User.QueryCard(client.Card.QueryOwner(crd).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get its owner")
	require.Equal(
		usr.Name,
		client.Card.QueryOwner(
			client.User.QueryCard(
				client.Card.QueryOwner(crd).
					Where(user.HasCard()).
					OnlyX(ctx)).
				OnlyX(ctx)).
			Where(user.HasCard()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(usr.Name, client.Card.QueryOwner(client.User.QueryCard(usr).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(crd.Number, client.User.QueryCard(client.Card.QueryOwner(client.User.QueryCard(usr).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Number, "should get its card")
	require.Equal(
		crd.Number,
		client.User.QueryCard(
			client.Card.QueryOwner(
				client.User.QueryCard(usr).
					Where(card.HasOwner()).
					OnlyX(ctx)).
				Where(user.HasCard()).
				OnlyX(ctx)).
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
	require.Zero(client.Node.QueryPrev(head).CountX(ctx))
	require.Zero(client.Node.QueryNext(head).CountX(ctx))

	t.Log("add node to the linked-list and connect it to the head (inverse creation)")
	sec := client.Node.Create().SetValue(2).SetPrevID(head.ID).SaveX(ctx)
	require.Zero(client.Node.QueryNext(sec).CountX(ctx), "should not have next")
	require.Equal(head.ID, client.Node.QueryPrev(sec).OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, client.Node.QueryNext(head).OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete inverse should delete association")
	client.Node.DeleteOne(sec).ExecX(ctx)
	require.Zero(client.Node.QueryNext(head).CountX(ctx))
	require.Equal(1, client.Node.Query().CountX(ctx), "linked-list should have 1 node")

	t.Log("add node to the linked-list by updating the head (the owner of the edge)")
	sec = client.Node.Create().SetValue(2).SaveX(ctx)
	client.Node.UpdateOne(head).SetNextID(sec.ID).ExecX(ctx)
	require.Zero(client.Node.QueryNext(sec).CountX(ctx), "should not have next")
	require.Equal(head.ID, client.Node.QueryPrev(sec).OnlyX(ctx).ID, "head should point to the second node")
	require.Equal(sec.ID, client.Node.QueryNext(head).OnlyX(ctx).ID)
	require.Equal(2, client.Node.Query().CountX(ctx), "linked-list should have 2 nodes")

	t.Log("delete assoc should delete inverse edge")
	client.Node.DeleteOne(head).ExecX(ctx)
	require.Zero(client.Node.QueryPrev(sec).CountX(ctx), "second node should be the head now")
	require.Zero(client.Node.QueryNext(sec).CountX(ctx), "second node should be the head now")

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
		require.Equal(nodes[i+1].Value, client.Node.QueryNext(n).OnlyX(ctx).Value)
	}
	require.Zero(client.Node.QueryNext(nodes[len(nodes)-1]).CountX(ctx), "last node should point to nil")

	t.Log("query with side lookup on inverse/assoc")
	require.Equal(4, client.Node.Query().Where(node.HasNext()).CountX(ctx))
	require.Equal(4, client.Node.Query().Where(node.HasPrev()).CountX(ctx))

	t.Log("make the linked-list to be circular")
	client.Node.UpdateOne(nodes[len(nodes)-1]).SetNextID(head.ID).SaveX(ctx)
	require.Equal(nodes[0].Value, client.Node.QueryNext(nodes[len(nodes)-1]).OnlyX(ctx).Value, "last node should point to head")
	require.Equal(nodes[len(nodes)-1].Value, client.Node.QueryPrev(nodes[0]).OnlyX(ctx).Value, "head should have a reference to the tail")

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
		client.Node.QueryPrev( // 1 (head)
			client.Node.QueryPrev( // 2
				client.Node.QueryPrev( // 3
					client.Node.QueryPrev( // 4
						client.Node.QueryPrev(head). // 5 (tail)
							OnlyX(ctx)).
						OnlyX(ctx)).
					OnlyX(ctx)).
				OnlyX(ctx)).
			OnlyX(ctx).Value,
	)
	// disrupt the query in the middle.
	require.Zero(client.Node.QueryPrev(client.Node.QueryPrev(client.Node.QueryPrev(client.Node.QueryPrev(client.Node.QueryPrev(head).OnlyX(ctx)).OnlyX(ctx)).Where(node.ValueGT(10)).OnlyX(ctx)).OnlyX(ctx)).CountX(ctx))

	t.Log("query long path from assoc")
	// going forward from head to next until we reach the head.
	require.Equal(
		head.Value,
		client.Node.QueryNext( // 1 (head)
			client.Node.QueryNext( // 5 (tail)
				client.Node.QueryNext( // 4
					client.Node.QueryNext( // 3
						client.Node.QueryNext(head). // 2
							OnlyX(ctx)).
						OnlyX(ctx)).
					OnlyX(ctx)).
				OnlyX(ctx)).
			OnlyX(ctx).Value,
	)
	// disrupt the query in the middle.
	require.Zero(client.Node.QueryNext(client.Node.QueryNext(client.Node.QueryNext(client.Node.QueryNext(client.Node.QueryNext(head).OnlyX(ctx)).OnlyX(ctx)).Where(node.ValueGT(10)).OnlyX(ctx)).OnlyX(ctx)).CountX(ctx))

	t.Log("delete all nodes except the head")
	client.Node.Delete().Where(node.ValueGT(1)).ExecX(ctx)
	head = client.Node.Query().OnlyX(ctx)

	t.Log("node points to itself (circular linked-list with 1 node)")
	client.Node.UpdateOne(head).SetNextID(head.ID).SaveX(ctx)
	require.Equal(head.ID, client.Node.QueryPrev(head).OnlyIDX(ctx))
	require.Equal(head.ID, client.Node.QueryNext(head).OnlyIDX(ctx))
	client.Node.UpdateOne(head).ClearNext().SaveX(ctx)
	require.Zero(client.Node.QueryPrev(head).CountX(ctx))
	require.Zero(client.Node.QueryNext(head).CountX(ctx))
}

// Demonstrate a O2O relation between two instances of the same type, where the relation
// has the same name in both directions. A couple. User A has "spouse" B (and vice versa).
// When setting B as a spouse of A, this sets A as spouse of B as well. In other words:
//
//		foo := client.User.Create().SetName("foo").SaveX(ctx)
//		bar := client.User.Create().SetName("bar").SetSpouse(foo).SaveX(ctx)
// 		count := client.User.Query.Where(user.HasSpouse()).CountX(ctx)
// 		// count will be 2, even though we've created only one relation above.
//
func O2OSelfRef(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without spouse")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(client.User.QuerySpouse(foo).ExistX(ctx))

	t.Log("sets spouse on user creation (inverse creation)")
	bar := client.User.Create().SetAge(10).SetName("bar").SetSpouseID(foo.ID).SaveX(ctx)
	require.True(client.User.QuerySpouse(foo).ExistX(ctx))
	require.True(client.User.QuerySpouse(bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(client.User.QuerySpouse(foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("add spouse to user by updating a user")
	bar = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.User.UpdateOne(foo).SetSpouseID(bar.ID).ExecX(ctx)
	require.True(client.User.QuerySpouse(foo).ExistX(ctx))
	require.True(client.User.QuerySpouse(bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("remove a spouse using update")
	client.User.UpdateOne(foo).ClearSpouse().ExecX(ctx)
	require.False(client.User.QuerySpouse(foo).ExistX(ctx))
	require.False(client.User.QuerySpouse(bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasSpouse()).CountX(ctx))
	// return back the spouse.
	client.User.UpdateOne(foo).SetSpouseID(bar.ID).ExecX(ctx)

	t.Log("create a user without spouse")
	baz := client.User.Create().SetAge(10).SetName("baz").SaveX(ctx)
	require.False(client.User.QuerySpouse(baz).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasSpouse()).CountX(ctx))

	t.Log("set a new spouse")
	client.User.UpdateOne(foo).ClearSpouse().SetSpouseID(baz.ID).ExecX(ctx)
	require.True(client.User.QuerySpouse(foo).ExistX(ctx))
	require.True(client.User.QuerySpouse(baz).ExistX(ctx))
	require.False(client.User.QuerySpouse(bar).ExistX(ctx))
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
		client.User.QuerySpouse( // foo
			client.User.QuerySpouse( // bar
				client.User.QuerySpouse( // foo
					client.User.QuerySpouse(foo). // bar
						OnlyX(ctx)).
					OnlyX(ctx)).
				OnlyX(ctx)).
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		client.User.QuerySpouse( // bar
			client.User.QuerySpouse( // foo
				client.User.QuerySpouse( // bar
					client.User.QuerySpouse(bar). // foo
						OnlyX(ctx)).
					OnlyX(ctx)).
				OnlyX(ctx)).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		bar.Name,
		client.User.
			Query().
			Where(user.Name("foo")). // foo
			QuerySpouse().           // bar
			OnlyX(ctx).Name,
	)
	require.Equal(
		bar.Name,
		client.User.
			Query().
			Where(user.Name("bar")). // bar
			QuerySpouse().           // foo
			QuerySpouse().           // bar
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
	require.False(client.User.QueryPets(usr).ExistX(ctx))

	t.Log("add pet to user on pet creation (inverse creation)")
	pedro := client.Pet.Create().SetName("pedro").SetOwnerID(usr.ID).SaveX(ctx)
	require.Equal(usr.Name, client.Pet.QueryOwner(pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, client.User.QueryPets(usr).OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.Pet.DeleteOne(pedro).ExecX(ctx)
	require.Zero(client.Pet.Query().CountX(ctx))
	require.False(client.User.QueryPets(usr).ExistX(ctx), "user should not have pet")

	t.Log("add pet to user by updating user (the owner of the edge)")
	pedro = client.Pet.Create().SetName("pedro").SaveX(ctx)
	client.User.UpdateOne(usr).AddPetIDs(pedro.ID).ExecX(ctx)
	require.Equal(usr.Name, client.Pet.QueryOwner(pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, client.User.QueryPets(usr).OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(usr).ExecX(ctx)
	require.Zero(client.User.Query().CountX(ctx))
	require.False(client.Pet.QueryOwner(pedro).ExistX(ctx), "pet should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	usr = client.User.Create().SetAge(30).SetName("a8m").SaveX(ctx)
	client.Pet.UpdateOne(pedro).SetOwnerID(usr.ID).ExecX(ctx)
	require.Equal(usr.Name, client.Pet.QueryOwner(pedro).OnlyX(ctx).Name)
	require.Equal(pedro.Name, client.User.QueryPets(usr).OnlyX(ctx).Name)

	t.Log("add another pet to user")
	xabi := client.Pet.Create().SetName("xabi").SetOwnerID(usr.ID).SaveX(ctx)
	require.Equal(2, client.User.QueryPets(usr).CountX(ctx))
	require.Equal(1, client.Pet.QueryOwner(xabi).CountX(ctx))
	require.Equal(1, client.Pet.QueryOwner(pedro).CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().SetAge(30).SetName("alex").AddPetIDs(pedro.ID).Save(ctx)
	require.Error(err, "pet already has an owner")

	t.Log("add multiple pets on creation")
	p1 := client.Pet.Create().SetName("p1").SaveX(ctx)
	p2 := client.Pet.Create().SetName("p2").SaveX(ctx)
	usr2 := client.User.Create().SetAge(30).SetName("alex").AddPetIDs(p1.ID, p2.ID).SaveX(ctx)
	require.True(client.Pet.QueryOwner(p1).ExistX(ctx))
	require.True(client.Pet.QueryOwner(p2).ExistX(ctx))
	require.Equal(2, client.User.QueryPets(usr2).CountX(ctx))
	// delete p1, p2.
	client.Pet.Delete().Where(pet.IDIn(p1.ID, p2.ID)).ExecX(ctx)
	require.Zero(client.User.QueryPets(usr2).CountX(ctx))

	t.Log("change the owner a pet")
	client.Pet.UpdateOne(xabi).ClearOwner().SetOwnerID(usr2.ID).ExecX(ctx)
	require.Equal(1, client.User.QueryPets(usr).CountX(ctx))
	require.Equal(1, client.User.QueryPets(usr2).CountX(ctx))
	require.Equal(usr2.Name, client.Pet.QueryOwner(xabi).OnlyX(ctx).Name)

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
	require.Equal(pedro.Name, client.User.QueryPets(client.Pet.QueryOwner(pedro).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(usr.Name, client.Pet.QueryOwner(client.User.QueryPets(client.Pet.QueryOwner(pedro).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get its owner")
	require.Equal(
		usr.Name,
		client.Pet.QueryOwner(
			client.User.QueryPets(
				client.Pet.QueryOwner(pedro).
					Where(user.HasPets()).
					OnlyX(ctx)).
				OnlyX(ctx)).
			Where(user.HasPets()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(usr.Name, client.Pet.QueryOwner(client.User.QueryPets(usr).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(pedro.Name, client.User.QueryPets(client.Pet.QueryOwner(client.User.QueryPets(usr).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get its pet")
	require.Equal(
		pedro.Name,
		client.User.QueryPets(
			client.Pet.QueryOwner(
				client.User.QueryPets(usr).
					Where(pet.HasOwner()). // pedro
					OnlyX(ctx)).           //
				Where(user.HasPets()). // a8m
				OnlyX(ctx)). // pedro
			OnlyX(ctx).Name,
		"should get its pet",
	)
	require.Equal(
		xabi.Name,
		client.User.Query().
			// alex matches this query (not a8m, and have a pet).
			Where(
				user.Not(user.Name(usr.Name)),
				user.HasPets(),
			).
			QueryPets().  // xabi
			QueryOwner(). // alex
			QueryPets().  // xabi
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
	require.Zero(client.User.QueryChildren(prt).CountX(ctx))

	t.Log("add child to parent on child creation (inverse creation)")
	chd := client.User.Create().SetAge(1).SetName("child").SetParentID(prt.ID).SaveX(ctx)
	require.Equal(prt.Name, client.User.QueryParent(chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, client.User.QueryChildren(prt).OnlyX(ctx).Name)

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(chd).ExecX(ctx)
	require.False(client.User.QueryChildren(prt).ExistX(ctx), "user should not have children")

	t.Log("add child to parent by updating user (the owner of the edge)")
	chd = client.User.Create().SetAge(1).SetName("child").SaveX(ctx)
	client.User.UpdateOne(prt).AddChildIDs(chd.ID).ExecX(ctx)
	require.Equal(prt.Name, client.User.QueryParent(chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, client.User.QueryChildren(prt).OnlyX(ctx).Name)

	t.Log("delete assoc (owner of the edge) should delete inverse edge")
	client.User.DeleteOne(prt).ExecX(ctx)
	require.Equal(1, client.User.Query().CountX(ctx))
	require.False(client.User.QueryParent(chd).ExistX(ctx), "child should not have an owner")

	t.Log("add pet to user by updating pet (the inverse edge)")
	prt = client.User.Create().SetAge(30).SetName("a8m").SaveX(ctx)
	client.User.UpdateOne(chd).SetParentID(prt.ID).ExecX(ctx)
	require.Equal(prt.Name, client.User.QueryParent(chd).OnlyX(ctx).Name)
	require.Equal(chd.Name, client.User.QueryChildren(prt).OnlyX(ctx).Name)
	require.Zero(client.User.QueryParent(prt).CountX(ctx), "parent is orphan")
	require.Zero(client.User.QueryChildren(chd).CountX(ctx), "child should not have children")

	t.Log("add another pet to user")
	chd2 := client.User.Create().SetAge(1).SetName("child2").SetParentID(prt.ID).SaveX(ctx)
	require.Equal(2, client.User.QueryChildren(prt).CountX(ctx))
	require.Equal(1, client.User.QueryParent(chd).CountX(ctx))
	require.Equal(1, client.User.QueryParent(chd2).CountX(ctx))

	t.Log("edge is unique on the inverse side")
	_, err := client.User.Create().SetAge(30).SetName("alex").AddChildIDs(chd.ID).Save(ctx)
	require.Error(err, "child already has parent")
	_, err = client.User.Create().SetAge(30).SetName("alex").AddChildIDs(chd2.ID).Save(ctx)
	require.Error(err, "child already has parent")

	t.Log("add multiple child on creation")
	chd3 := client.User.Create().SetAge(1).SetName("child3").SaveX(ctx)
	chd4 := client.User.Create().SetAge(1).SetName("child4").SaveX(ctx)
	prt2 := client.User.Create().SetAge(30).SetName("alex").AddChildIDs(chd3.ID, chd4.ID).SaveX(ctx)
	require.True(client.User.QueryParent(chd3).ExistX(ctx))
	require.True(client.User.QueryParent(chd3).ExistX(ctx))
	require.Equal(2, client.User.QueryChildren(prt2).CountX(ctx))
	// delete chd3, chd4.
	client.User.Delete().Where(user.IDIn(chd3.ID, chd4.ID)).ExecX(ctx)
	require.Zero(client.User.QueryChildren(prt2).CountX(ctx))

	t.Log("change the parent a child")
	client.User.UpdateOne(chd2).ClearParent().SetParentID(prt2.ID).ExecX(ctx)
	require.Equal(1, client.User.QueryChildren(prt).CountX(ctx))
	require.Equal(1, client.User.QueryChildren(prt2).CountX(ctx))
	require.Equal(chd2.Name, client.User.QueryChildren(prt2).OnlyX(ctx).Name)

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
	require.Equal(chd.Name, client.User.QueryChildren(client.User.QueryParent(chd).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(prt.Name, client.User.QueryParent(client.User.QueryChildren(client.User.QueryParent(chd).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get its parent")
	require.Equal(
		prt.Name,
		client.User.QueryParent(
			client.User.QueryChildren(
				client.User.QueryParent(chd).
					Where(user.HasChildren()).
					OnlyX(ctx)).
				OnlyX(ctx)).
			Where(user.HasChildren()).
			OnlyX(ctx).Name,
		"should get its owner",
	)

	t.Log("query long path from assoc")
	require.Equal(prt.Name, client.User.QueryParent(client.User.QueryChildren(prt).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(chd.Name, client.User.QueryChildren(client.User.QueryParent(client.User.QueryChildren(prt).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get its child")
	require.Equal(
		chd.Name,
		client.User.QueryChildren(
			client.User.QueryParent(
				client.User.QueryChildren(prt).
					Where(user.HasParent()).   // child
					OnlyX(ctx)).               //
				Where(user.HasChildren()). // parent
				OnlyX(ctx)). // child
			OnlyX(ctx).Name,
		"should get its child",
	)
	require.Equal(
		chd2.Name,
		client.User.Query().
			// "alex" matches this query (not "a8m", and have a child).
			Where(
				user.Not(user.Name(prt.Name)),
				user.HasChildren(),
			).
			QueryChildren(). // child
			QueryParent().   // parent
			QueryChildren(). // child
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two instances of the same type, where the relation
// has the same name in both directions. A friendship between Users.
// User A has "friend" B (and vice versa). When setting B as a friend of A, this sets A
// as friend of B as well. In other words:
//
//		foo := client.User.Create().SetName("foo").SaveX(ctx)
//		bar := client.User.Create().SetName("bar").AddFriends(foo).SaveX(ctx)
// 		count := client.User.Query.Where(user.HasFriends()).CountX(ctx)
// 		// count will be 2, even though we've created only one relation above.
//
func M2MSelfRef(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without friends")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(client.User.QueryFriends(foo).ExistX(ctx))

	t.Log("sets friendship on user creation (inverse creation)")
	bar := client.User.Create().SetAge(10).SetName("bar").AddFriendIDs(foo.ID).SaveX(ctx)
	require.True(client.User.QueryFriends(foo).ExistX(ctx))
	require.True(client.User.QueryFriends(bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(client.User.QueryFriends(foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("add friendship to user by updating existing users")
	bar = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.User.UpdateOne(foo).AddFriendIDs(bar.ID).ExecX(ctx)
	require.True(client.User.QueryFriends(foo).ExistX(ctx))
	require.True(client.User.QueryFriends(bar).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("remove friendship using update")
	client.User.UpdateOne(foo).RemoveFriendIDs(bar.ID).ExecX(ctx)
	require.False(client.User.QueryFriends(foo).ExistX(ctx))
	require.False(client.User.QueryFriends(bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFriends()).CountX(ctx))
	// return back the friendship.
	client.User.UpdateOne(foo).AddFriendIDs(bar.ID).ExecX(ctx)

	t.Log("create a user without friends")
	baz := client.User.Create().SetAge(10).SetName("baz").SaveX(ctx)
	require.False(client.User.QueryFriends(baz).ExistX(ctx))
	require.Equal(2, client.User.Query().Where(user.HasFriends()).CountX(ctx))

	t.Log("both baz and bar are friends of foo")
	client.User.UpdateOne(baz).AddFriendIDs(foo.ID).ExecX(ctx)
	require.Equal(2, client.User.QueryFriends(foo).CountX(ctx))
	require.Equal(foo.Name, client.User.QueryFriends(bar).OnlyX(ctx).Name)
	require.Equal(foo.Name, client.User.QueryFriends(baz).OnlyX(ctx).Name)
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
		client.User.QueryFriends( // foo
			client.User.QueryFriends( // baz
				client.User.QueryFriends( // foo
					client.User.QueryFriends(foo).Where(user.Name(bar.Name)). // bar
						OnlyX(ctx)).
					OnlyX(ctx)).Where(user.Name(baz.Name)).
				OnlyX(ctx)).
			OnlyX(ctx).Name,
	)
	require.Equal(
		foo.Name,
		client.User.QueryFriends( // foo
			client.User.QueryFriends( // bar, baz
				client.User.QueryFriends( // foo
					client.User.QueryFriends(foo). // bar, baz
						OnlyX(ctx)).
					OnlyX(ctx)).
				OnlyX(ctx)).
			OnlyX(ctx).Name,
	)
	require.Equal(
		baz.Name,
		client.User.QueryFriends( // baz
			client.User.QueryFriends( // foo
				client.User.QueryFriends(foo).Where(user.Name(bar.Name)). // bar
					OnlyX(ctx)).
				OnlyX(ctx)).Where(user.Not(user.Name(bar.Name))).
			OnlyX(ctx).Name,
	)

	t.Log("query path from client")
	require.Equal(
		[]string{bar.Name, baz.Name},
		client.User.
			Query().
			Where(user.Name(foo.Name)). // foo
			QueryFriends().             // bar, baz
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
	)
	require.Equal(
		bar.Name,
		client.User.
			Query().
			// foo has a friend (bar) that does not have a friend named baz.
			Where(
				user.HasFriendsWith(
					user.Not(
						user.HasFriendsWith(user.Name(baz.Name)),
					),
				),
			).
			// bar and baz.
			QueryFriends().
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
	require.False(client.User.QueryFollowers(foo).ExistX(ctx))

	t.Log("adds followers on user creation (inverse creation)")
	bar := client.User.Create().SetAge(10).SetName("bar").AddFollowingIDs(foo.ID).SaveX(ctx)
	require.Equal(foo.Name, client.User.QueryFollowing(bar).OnlyX(ctx).Name)
	require.Equal(bar.Name, client.User.QueryFollowers(foo).OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.HasFollowing()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.User.DeleteOne(bar).ExecX(ctx)
	require.False(client.User.QueryFollowers(foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowing()).CountX(ctx))

	t.Log("add followers to user by updating existing users")
	bar = client.User.Create().SetAge(10).SetName("bar").SaveX(ctx)
	client.User.UpdateOne(foo).AddFollowerIDs(bar.ID).ExecX(ctx)
	require.Equal(foo.Name, client.User.QueryFollowing(bar).OnlyX(ctx).Name)
	require.Equal(bar.Name, client.User.QueryFollowers(foo).OnlyX(ctx).Name)
	require.Equal(1, client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	require.Equal(1, client.User.Query().Where(user.HasFollowing()).CountX(ctx))

	t.Log("remove following using update")
	client.User.UpdateOne(bar).RemoveFollowingIDs(foo.ID).ExecX(ctx)
	require.False(client.User.QueryFollowers(foo).ExistX(ctx))
	require.False(client.User.QueryFollowing(bar).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowing()).CountX(ctx))
	require.Zero(client.User.Query().Where(user.HasFollowers()).CountX(ctx))
	// follow back.
	client.User.UpdateOne(bar).AddFollowingIDs(foo.ID).ExecX(ctx)

	t.Log("remove followers using update (inverse)")
	client.User.UpdateOne(foo).RemoveFollowerIDs(bar.ID).ExecX(ctx)
	require.False(client.User.QueryFollowers(foo).ExistX(ctx))
	require.False(client.User.QueryFollowing(bar).ExistX(ctx))
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
			client.User.QueryFollowing(u).
				Order(ent.Asc(user.FieldName)).
				GroupBy(user.FieldName).
				StringsX(ctx),
		)
	}
	require.Equal(5, client.User.QueryFollowers(bar).CountX(ctx), "users1..5")
	require.Equal(6, client.User.QueryFollowers(foo).CountX(ctx), "users1..5 and bar")
	require.Equal(2, client.User.Query().Where(user.HasFollowers()).CountX(ctx), "foo and bar")
	require.Equal(6, client.User.Query().Where(user.HasFollowing()).CountX(ctx), "users1..5 and bar")
	// compare followers.
	require.Equal(
		client.User.QueryFollowers(bar).
			Order(ent.Asc(user.FieldName)).
			GroupBy(user.FieldName).
			StringsX(ctx),
		client.User.QueryFollowers(foo).
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
	require.Equal(foo.Name, client.User.QueryFollowing(client.User.QueryFollowers(foo).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, client.User.QueryFollowers(client.User.QueryFollowing(client.User.QueryFollowers(foo).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get its follower (bar)")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, client.User.QueryFollowers(client.User.QueryFollowing(bar).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(foo.Name, client.User.QueryFollowing(client.User.QueryFollowers(client.User.QueryFollowing(bar).OnlyX(ctx)).OnlyX(ctx)).OnlyX(ctx).Name, "should get foo")

	// generate additional users and make sure we don't get them in the queries below.
	client.User.Create().SetAge(10).SetName("baz").SaveX(ctx)
	client.User.Create().SetAge(10).SetName("qux").SaveX(ctx)

	t.Log("query path from a user")
	require.Equal(
		bar.Name,
		client.User.QueryFollowers( // bar
			client.User.QueryFollowing( // foo
				client.User.QueryFollowers(foo).Where(user.Name(bar.Name)). // bar
					OnlyX(ctx)).Where(user.HasFollowers()).
				OnlyX(ctx)).
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
		client.User.Query().
			Where(
				user.HasFollowersWith(
					user.Name(bar.Name),
				),
			). // has followers named bar (foo).
			Where(user.Name(foo.Name)). // foo
			OnlyX(ctx).Name,
	)
}

// Demonstrate a M2M relation between two different types. User and groups.
func M2MTwoTypes(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("new user without groups")
	foo := client.User.Create().SetAge(10).SetName("foo").SaveX(ctx)
	require.False(client.User.QueryGroups(foo).ExistX(ctx))
	require.Zero(client.Group.Query().CountX(ctx))

	t.Log("adds users to group on group creation (inverse creation)")
	// group-info is required edge.
	inf := client.GroupInfo.Create().SetDesc("desc").SaveX(ctx)
	hub := client.Group.Create().SetName("Github").SetExpire(time.Now()).AddUserIDs(foo.ID).SetInfoID(inf.ID).SaveX(ctx)
	require.Equal(foo.Name, client.Group.QueryUsers(hub).OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, client.User.QueryGroups(foo).OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.HasUsers()).CountX(ctx))

	t.Log("delete inverse should delete association")
	client.Group.DeleteOne(hub).ExecX(ctx)
	require.False(client.User.QueryGroups(foo).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))

	t.Log("add user to groups updating existing users")
	hub = client.Group.Create().SetName("Github").SetExpire(time.Now()).SetInfoID(inf.ID).SaveX(ctx)
	require.False(client.User.QueryGroups(foo).ExistX(ctx))
	client.User.UpdateOne(foo).AddGroupIDs(hub.ID).ExecX(ctx)
	require.Equal(foo.Name, client.Group.QueryUsers(hub).OnlyX(ctx).Name, "group has only one user")
	require.Equal(hub.Name, client.User.QueryGroups(foo).OnlyX(ctx).Name, "user is connected to one group")
	require.Equal(1, client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Equal(1, client.Group.Query().Where(group.HasUsers()).CountX(ctx))

	t.Log("delete assoc should delete inverse as well")
	client.User.DeleteOne(foo).ExecX(ctx)
	require.False(client.Group.QueryUsers(hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	// add back the user.
	foo = client.User.Create().SetAge(10).SetName("foo").AddGroupIDs(hub.ID).SaveX(ctx)

	t.Log("remove following using update (assoc)")
	client.User.UpdateOne(foo).RemoveGroupIDs(hub.ID).ExecX(ctx)
	require.False(client.User.QueryGroups(foo).ExistX(ctx))
	require.False(client.Group.QueryUsers(hub).ExistX(ctx))
	require.Zero(client.User.Query().Where(user.HasGroups()).CountX(ctx))
	require.Zero(client.Group.Query().Where(group.HasUsers()).CountX(ctx))
	// join back to group.
	client.User.UpdateOne(foo).AddGroupIDs(hub.ID).ExecX(ctx)

	t.Log("remove following using update (inverse)")
	client.Group.UpdateOne(hub).RemoveUserIDs(foo.ID).ExecX(ctx)
	require.False(client.User.QueryGroups(foo).ExistX(ctx))
	require.False(client.Group.QueryUsers(hub).ExistX(ctx))
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
	require.Equal(foo.Name, client.Group.QueryUsers(hub).OnlyX(ctx).Name, "hub has only one user")
	require.Equal(hub.Name, client.User.QueryGroups(foo).OnlyX(ctx).Name, "foo is connected only to hub")
	require.Equal(bar.Name, client.Group.QueryUsers(lab).OnlyX(ctx).Name, "lab has only one user")
	require.Equal(lab.Name, client.User.QueryGroups(bar).OnlyX(ctx).Name, "bar is connected only to lab")
	// add bar to hub.
	client.User.UpdateOne(bar).AddGroupIDs(hub.ID).ExecX(ctx)
	require.Equal(2, client.Group.QueryUsers(hub).CountX(ctx))
	require.Equal(1, client.Group.QueryUsers(lab).CountX(ctx))
	require.Equal([]string{bar.Name, foo.Name}, client.Group.QueryUsers(hub).Order(ent.Asc(user.FieldName)).GroupBy(user.FieldName).StringsX(ctx))
	require.Equal([]string{hub.Name, lab.Name}, client.User.QueryGroups(bar).Order(ent.Asc(user.FieldName)).GroupBy(user.FieldName).StringsX(ctx))

	t.Log("query with side lookup from inverse")
	require.Equal(hub.Name, client.Group.QueryUsers(client.User.QueryGroups(client.Group.QueryUsers(hub).OnlyX(ctx)).Where(group.Name(hub.Name)).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(bar.Name, client.Group.QueryUsers(client.User.QueryGroups(client.Group.QueryUsers(lab).OnlyX(ctx)).Where(group.Not(group.Name(hub.Name))).OnlyX(ctx)).OnlyX(ctx).Name, "should get its user")

	t.Log("query with side lookup from assoc")
	require.Equal(bar.Name, client.Group.QueryUsers(client.User.QueryGroups(bar).Where(group.Name(lab.Name)).OnlyX(ctx)).OnlyX(ctx).Name, "should get itself")
	require.Equal(lab.Name, client.User.QueryGroups(client.Group.QueryUsers(client.User.QueryGroups(bar).Where(group.Name(lab.Name)).OnlyX(ctx)).OnlyX(ctx)).Where(group.Name(lab.Name)).OnlyX(ctx).Name, "should get its group")

	t.Log("query path from a user")
	require.Equal(
		hub.Name,
		client.User.QueryGroups(bar).
			Where(
				group.HasUsersWith(user.Name(foo.Name)),
			).
			// foo (not having group with name "lab").
			QueryUsers().
			Where(
				user.Not(
					user.HasGroupsWith(group.Name(lab.Name)),
				),
			).
			// hub.
			QueryGroups().
			OnlyX(ctx).Name,
	)

	t.Log("query path from a client")
	require.Equal(
		bar.Name,
		client.Group.
			// hub.
			Query().
			Where(
				group.HasUsersWith(user.Name(foo.Name)),
			).
			// foo (not having group with name "lab").
			QueryUsers().
			Where(
				user.Not(
					user.HasGroupsWith(group.Name(lab.Name)),
				),
			).
			// hub.
			QueryGroups().
			// bar, foo.
			QueryUsers().
			Order(ent.Asc(user.FieldName)).
			// bar
			FirstX(ctx).Name,
	)
}

func Types(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	ft := client.FieldType.Create().
		SetInt(1).
		SetInt8(8).
		SetInt16(16).
		SetInt32(32).
		SetInt64(64).
		SaveX(ctx)

	require.Equal(1, ft.Int)
	require.Equal(int8(8), ft.Int8)
	require.Equal(int16(16), ft.Int16)
	require.Equal(int32(32), ft.Int32)
	require.Equal(int64(64), ft.Int64)

	ft = client.FieldType.Create().
		SetInt(1).
		SetInt8(math.MaxInt8).
		SetInt16(math.MaxInt16).
		SetInt32(math.MaxInt16).
		SetInt64(math.MaxInt16).
		SetOptionalInt8(math.MaxInt8).
		SetOptionalInt16(math.MaxInt16).
		SetOptionalInt32(math.MaxInt32).
		SetOptionalInt64(math.MaxInt64).
		SetNillableInt8(math.MaxInt8).
		SetNillableInt16(math.MaxInt16).
		SetNillableInt32(math.MaxInt32).
		SetNillableInt64(math.MaxInt64).
		SaveX(ctx)

	require.Equal(int8(math.MaxInt8), ft.OptionalInt8)
	require.Equal(int16(math.MaxInt16), ft.OptionalInt16)
	require.Equal(int32(math.MaxInt32), ft.OptionalInt32)
	require.Equal(int64(math.MaxInt64), ft.OptionalInt64)
	require.Equal(int8(math.MaxInt8), *ft.NillableInt8)
	require.Equal(int16(math.MaxInt16), *ft.NillableInt16)
	require.Equal(int32(math.MaxInt32), *ft.NillableInt32)
	require.Equal(int64(math.MaxInt64), *ft.NillableInt64)

	ft = client.FieldType.UpdateOne(ft).
		SetInt(1).
		SetInt8(math.MaxInt8).
		SetInt16(math.MaxInt16).
		SetInt32(math.MaxInt16).
		SetInt64(math.MaxInt16).
		SetOptionalInt8(math.MaxInt8).
		SetOptionalInt16(math.MaxInt16).
		SetOptionalInt32(math.MaxInt32).
		SetOptionalInt64(math.MaxInt64).
		SetNillableInt8(math.MaxInt8).
		SetNillableInt16(math.MaxInt16).
		SetNillableInt32(math.MaxInt32).
		SetNillableInt64(math.MaxInt64).
		SaveX(ctx)

	require.Equal(int8(math.MaxInt8), ft.OptionalInt8)
	require.Equal(int16(math.MaxInt16), ft.OptionalInt16)
	require.Equal(int32(math.MaxInt32), ft.OptionalInt32)
	require.Equal(int64(math.MaxInt64), ft.OptionalInt64)
	require.Equal(int8(math.MaxInt8), *ft.NillableInt8)
	require.Equal(int16(math.MaxInt16), *ft.NillableInt16)
	require.Equal(int32(math.MaxInt32), *ft.NillableInt32)
	require.Equal(int64(math.MaxInt64), *ft.NillableInt64)
}

func drop(t *testing.T, client *ent.Client) {
	t.Log("drop data from database")
	ctx := context.Background()
	client.Pet.Delete().ExecX(ctx)
	client.File.Delete().ExecX(ctx)
	client.Card.Delete().ExecX(ctx)
	client.Node.Delete().ExecX(ctx)
	client.User.Delete().ExecX(ctx)
	client.Group.Delete().ExecX(ctx)
	client.Comment.Delete().ExecX(ctx)
	client.GroupInfo.Delete().ExecX(ctx)
	client.FieldType.Delete().ExecX(ctx)
	client.FileType.Delete().ExecX(ctx)
}
