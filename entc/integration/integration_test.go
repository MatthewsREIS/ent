// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	stdsql "database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	sqlschema "entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/entc/integration/ent"
	"entgo.io/ent/entc/integration/ent/card"
	"entgo.io/ent/entc/integration/ent/comment"
	"entgo.io/ent/entc/integration/ent/enttest"
	"entgo.io/ent/entc/integration/ent/exvaluescan"
	"entgo.io/ent/entc/integration/ent/file"
	"entgo.io/ent/entc/integration/ent/filetype"
	"entgo.io/ent/entc/integration/ent/group"
	"entgo.io/ent/entc/integration/ent/groupinfo"
	"entgo.io/ent/entc/integration/ent/hook"
	"entgo.io/ent/entc/integration/ent/item"
	"entgo.io/ent/entc/integration/ent/license"
	"entgo.io/ent/entc/integration/ent/migrate"
	"entgo.io/ent/entc/integration/ent/node"
	"entgo.io/ent/entc/integration/ent/pet"
	"entgo.io/ent/entc/integration/ent/schema"
	"entgo.io/ent/entc/integration/ent/schema/task"
	enttask "entgo.io/ent/entc/integration/ent/task"
	"entgo.io/ent/entc/integration/ent/user"
	"entgo.io/ent/runtime/entbuilder"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	t.Parallel()
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1", opts)
	defer client.Close()
	for _, tt := range tests {
		name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
		t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
			drop(t, client)
			tt(t, client)
		})
	}
}

func TestMySQL(t *testing.T) {
	for version, port := range map[string]int{"56": 3306, "57": 3307, "8": 3308} {
		addr := net.JoinHostPort("localhost", strconv.Itoa(port))
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			client := enttest.Open(t, dialect.MySQL, fmt.Sprintf("root:pass@tcp(%s)/test?parseTime=True", addr), opts)
			defer client.Close()
			for _, tt := range tests {
				name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
				t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
					drop(t, client)
					tt(t, client)
				})
			}
		})
	}
}

func TestMaria(t *testing.T) {
	for version, port := range map[string]int{"10.5": 4306, "10.2": 4307, "10.3": 4308} {
		addr := net.JoinHostPort("localhost", strconv.Itoa(port))
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			client := enttest.Open(t, dialect.MySQL, fmt.Sprintf("root:pass@tcp(%s)/test?parseTime=True", addr), opts)
			defer client.Close()
			for _, tt := range tests {
				name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
				t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
					drop(t, client)
					tt(t, client)
				})
			}
		})
	}
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{
		"10": 5430,
		"11": 5431,
		"12": 5432,
		"13": 5433,
		"14": 5434,
		"15": 5435,
		"16": 5436,
		"17": 5437,
	} {
		addr := fmt.Sprintf("host=localhost port=%d user=postgres dbname=test password=pass sslmode=disable", port)
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			client := enttest.Open(t, dialect.Postgres, addr, opts)
			defer client.Close()
			for _, tt := range tests {
				name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
				t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
					drop(t, client)
					tt(t, client)
				})
			}
		})
	}
}

var (
	opts = enttest.WithMigrateOptions(
		migrate.WithDropIndex(true),
		migrate.WithDropColumn(true),
	)
	tests = [...]func(*testing.T, *ent.Client){
		Sanity,
		NoSchemaChanges,
		Tx,
		Lock,
		Indexes,
		Types,
		Clone,
		EntQL,
		Paging,
		Select,
		Aggregate,
		Delete,
		Upsert,
		Relation,
		ExecQuery,
		Predicate,
		AddValues,
		ClearEdges,
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
		IDFieldImmutableOnUpdate,
		Sensitive,
		EagerLoading,
		NamedEagerLoading,
		Mutation,
		CreateBulk,
		ConstraintChecks,
		NillableRequired,
		ExtValueScan,
		OrderByEdgeCount,
		OrderByEdgeTerms,
		OrderByFluent,
	}
)

func Sanity(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	usr := client.User.Create().With(user.F.Name.Set("foo"), user.F.Age.Set(20)).SaveX(ctx)
	client.User.Update().ExecX(ctx)
	client.User.UpdateOne(usr).ExecX(ctx)
	require.Equal("foo", usr.Name)
	require.Equal(20, usr.Age)
	require.NotEmpty(usr.ID)
	client.User.Query().OnlyX(ctx)
	client.User.Delete().ExecX(ctx)
	require.Empty(client.User.Query().AllX(ctx))
	pt := client.Pet.Create().With(pet.F.Name.Set("pedro")).SaveX(ctx)
	usr = client.User.Create().With(user.F.Name.Set("foo"), user.F.Age.Set(20), user.E.Pets.AddIDs(pt.ID)).SaveX(ctx)
	child := client.User.Create().With(user.F.Name.Set("bar"), user.F.Age.Set(20), user.E.Children.AddIDs(usr.ID)).SaveX(ctx)
	inf := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx)
	grp := client.Group.Create().With(group.F.Name.Set("Github"), group.F.Expire.Set(time.Now()), group.E.Users.AddIDs(usr.ID, child.ID), group.E.Info.SetID(inf.ID)).SaveX(ctx)
	require.Equal(1, client.Group.Query().CountX(ctx))
	require.Zero(client.Group.Query().Where(group.F.Active.EQ(false)).CountX(ctx))
	require.Len(ent.QueryGroupUsers(client.Group, grp).AllX(ctx), 2)
	ent.QueryUserGroups(client.User, usr).OnlyX(ctx)
	ent.QueryUserGroups(client.User, child).OnlyX(ctx)
	usr2 := client.User.Create().With(user.F.Name.Set("qux"), user.F.Age.Set(20), user.E.Spouse.SetID(usr.ID)).SaveX(ctx)
	ent.QueryUserSpouse(client.User, usr2).OnlyX(ctx)
	ent.QueryUserSpouse(client.User, usr).OnlyX(ctx)
	require.Equal(usr.Name, ent.QueryPetOwnerFromQuery(ent.QueryUserPets(client.User, usr)).OnlyX(ctx).Name)
	require.Equal(pt.Name, ent.QueryUserPetsFromQuery(ent.QueryPetOwnerFromQuery(ent.QueryUserPets(client.User, usr))).OnlyX(ctx).Name)
	require.Empty(ent.QueryUserPetsFromQuery(ent.QueryUserSpouse(client.User, usr)).AllX(ctx))
	require.Equal(pt.Name, ent.QueryUserPetsFromQuery(ent.QueryUserSpouse(client.User, usr2)).OnlyX(ctx).Name)
	require.Len(ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, usr)).AllX(ctx), 2)
	require.Len(ent.QueryUserGroupsFromQuery(ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, usr))).AllX(ctx), 1, "should be unique by default")
	require.Len(ent.QueryUserGroups(client.User, usr).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.E.Pets.Has()).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.E.Spouse.Has()).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Not(user.E.Spouse.Has())).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.E.Groups.Has()).AllX(ctx), 2)
	require.Len(client.Group.Query().Where(group.E.Users.Has()).AllX(ctx), 1)
	require.Len(client.Group.Query().Where(group.E.Users.HasWith(user.F.Name.EQ("foo"))).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.E.Groups.HasWith(group.F.Name.HasPrefix("G"))).AllX(ctx), 2)
	require.Equal(3, client.User.Query().CountX(ctx))
	require.Equal(client.Group.Query().Where(group.E.Users.HasWith(user.F.Name.EQ("foo"))).CountX(ctx), 1)
	require.True(client.User.Query().ExistX(ctx))
	require.True(client.User.Query().Where(user.E.Pets.HasWith(pet.F.Name.HasPrefix("ped"))).ExistX(ctx))
	require.False(client.User.Query().Where(user.E.Pets.HasWith(pet.F.Name.HasPrefix("pan"))).ExistX(ctx))
	require.Equal(child.Name, client.User.Query().Order(ent.Asc("name")).FirstX(ctx).Name)
	require.Equal(usr2.Name, client.User.Query().Order(ent.Desc("name")).FirstX(ctx).Name)
	// Update fields.
	client.User.Update().Where(user.F.ID.EQ(child.ID)).With(user.F.Name.Set("Ariel")).SaveX(ctx)
	client.User.Query().Where(user.F.Name.EQ("Ariel")).OnlyX(ctx)
	// Update edges.
	require.Empty(ent.QueryUserPets(client.User, child).AllX(ctx))
	require.NoError(client.Pet.UpdateOne(pt).With(pet.E.Owner.Clear()).Exec(ctx))
	client.User.Update().Where(user.F.ID.EQ(child.ID)).With(user.E.Pets.AddIDs(pt.ID)).SaveX(ctx)
	require.NotEmpty(ent.QueryUserPets(client.User, child).AllX(ctx))
	client.User.Update().Where(user.F.ID.EQ(child.ID)).With(user.E.Pets.RemoveIDs(pt.ID)).SaveX(ctx)
	require.Empty(ent.QueryUserPets(client.User, child).AllX(ctx))
	// Remove edges.
	client.User.Update().With(user.E.Spouse.Clear()).SaveX(ctx)
	require.Empty(client.User.Query().Where(user.E.Spouse.Has()).AllX(ctx))
	client.User.Update().With(user.E.Friends.AddIDs(child.ID), user.E.Groups.RemoveIDs(grp.ID)).Where(user.F.ID.EQ(usr.ID)).SaveX(ctx)
	require.NotEmpty(ent.QueryUserGroups(client.User, child).AllX(ctx))
	require.Empty(ent.QueryUserGroups(client.User, usr).AllX(ctx))
	require.Len(ent.QueryUserFriends(client.User, child).AllX(ctx), 1)
	require.Len(ent.QueryUserFriends(client.User, usr).AllX(ctx), 1)
	// Update one node.
	usr = client.User.UpdateOne(usr).With(user.F.Name.Set("baz"), user.E.Groups.AddIDs(grp.ID)).SaveX(ctx)
	require.Equal("baz", usr.Name)
	require.NotEmpty(ent.QueryUserGroups(client.User, usr).AllX(ctx))
	// Update unknown node.
	err := client.User.UpdateOneID(usr.ID + math.MaxInt8).With(user.F.Name.Set("foo")).Exec(ctx)
	require.Error(err)
	require.True(ent.IsNotFound(err))
	// Update a vertex with filter.
	u := client.User.UpdateOneID(usr.ID).With(user.F.Name.Set("foo"))
	u.Mutation().WhereP(user.F.Name.EQ(usr.Name))
	require.NoError(u.Exec(ctx))
	u = client.User.UpdateOneID(usr.ID).With(user.F.Name.Set("bar"))
	u.Mutation().WhereP(user.F.Name.EQ("baz"))
	require.Error(u.Exec(ctx))
	require.True(ent.IsNotFound(err))

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
	client.User.Delete().Where(user.F.ID.In(ids...)).ExecX(ctx)
	ids = client.User.Query().IDsX(ctx)
	require.Empty(ids)
	// Nop.
	client.User.Delete().Where(user.F.ID.In(ids...)).ExecX(ctx)
	// Check the struct-tag annotation.
	fi, ok := reflect.TypeOf(ent.Card{}).FieldByName("Edges")
	require.True(ok)
	require.NotEmpty(fi.Tag.Get("mashraki"))
	fi, ok = reflect.TypeOf(ent.Card{}).FieldByName("ID")
	require.True(ok)
	require.Equal("-", fi.Tag.Get("json"))
	fi, ok = reflect.TypeOf(ent.Card{}).FieldByName("Number")
	require.True(ok)
	require.Equal("-", fi.Tag.Get("json"))
	client.User.Create().With(user.F.Name.Set("tarrence"), user.F.Age.Set(30)).ExecX(ctx)

	t.Run("StringPredicates", func(t *testing.T) {
		client.Pet.Delete().ExecX(ctx)
		a := client.Pet.Create().With(pet.F.Name.Set("a%")).SaveX(ctx)
		require.True(client.Pet.Query().Where(pet.F.Name.HasPrefix("a%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.HasPrefix("%a%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Or(pet.F.Name.HasPrefix("%a%"), pet.F.Name.HasPrefix("%a%"))).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.HasSuffix("%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.HasSuffix("a%%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.Contains("a")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.Contains("a%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.Contains("%a%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.ContainsFold("A%")).ExistX(ctx))

		client.Pet.UpdateOne(a).With(pet.F.Name.Set("a_\\")).ExecX(ctx)
		require.True(client.Pet.Query().Where(pet.F.Name.HasPrefix("a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.HasPrefix("%a")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.HasPrefix("a_")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.HasSuffix("a_\\")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.HasSuffix("%a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.HasSuffix("a%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.Contains("a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.Contains("%a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.Contains("a%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.ContainsFold("A")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.ContainsFold("%A")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.ContainsFold("A%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.F.Name.EqualFold("A_\\")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.EqualFold("%A_\\")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.EqualFold("A_\\%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.F.Name.EqualFold("A%")).ExistX(ctx))
	})
}

func Upsert(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	u := client.User.Create().With(user.F.Name.Set("Ariel"), user.F.Age.Set(30), user.F.Phone.Set("0000")).SaveX(ctx)
	require.Equal(t, "static", u.Address, "address was set by default func")
	err := client.User.Create().With(user.F.Name.Set("Mashraki"), user.F.Age.Set(30), user.F.Phone.Set("0000")).Exec(ctx)
	require.True(t, ent.IsConstraintError(err), "phone field is unique")
	err = client.User.Create().With(user.F.Name.Set("Mashraki"), user.F.Age.Set(30), user.F.Phone.Set("0000")).OnConflict().Exec(ctx)
	require.EqualError(t, err, "ent: missing options for UserCreate.OnConflict")

	client.User.Create().With(user.F.Name.Set("Mashraki"), user.F.Age.Set(30), user.F.Phone.Set("0000")).
		OnConflict(
			sql.ConflictColumns(user.FieldPhone),
		).
		// Update "name" to the value that was set on create ("Mashraki").
		UpdateFields(user.FieldName).
		ExecX(ctx)
	u = client.User.GetX(ctx, u.ID)
	require.Equal(t, "Mashraki", u.Name, "name was changed by the UPDATE clause")

	id := client.User.Create().With(user.F.Name.Set("Boring"), user.F.Age.Set(33), user.F.Phone.Set("0000")).
		OnConflictColumns(user.FieldPhone).
		// Override some fields with custom update.
		Update(func(u *user.UserUpsert) {
			// Age was set to the new value (33).
			u.Update(user.FieldAge)
			// Update an additional field that was defined in `VALUES`.
			u.Set(user.FieldAddress, "localhost")
		}).
		IDX(ctx)
	require.Equal(t, u.ID, id)
	u = client.User.GetX(ctx, u.ID)
	require.Equal(t, "Mashraki", u.Name)
	require.Equal(t, 33, u.Age, "age was modified by the UPDATE clause")
	require.Equal(t, "localhost", u.Address, "address was modified by the UPDATE clause")

	id = client.User.Create().With(user.F.Name.Set("Boring"), user.F.Age.Set(33), user.F.Phone.Set("0000")).
		OnConflictColumns(user.FieldPhone).
		// Override some fields with custom update.
		Add(user.FieldAge, -1).
		IDX(ctx)
	u = client.User.GetX(ctx, id)
	require.Equal(t, 32, u.Age, "age was modified by the UPDATE clause")

	builders := []*user.UserCreate{
		client.User.Create().With(user.F.Name.Set("A"), user.F.Age.Set(1), user.F.Phone.Set("0000")), // Duplicate
		client.User.Create().With(user.F.Name.Set("B"), user.F.Age.Set(1), user.F.Phone.Set("1111")), // New row.
	}
	client.User.CreateBulk(builders...).
		OnConflictColumns(user.FieldPhone).
		UpdateNewValues().
		ExecX(ctx)
	users := client.User.Query().Order(ent.Asc(user.FieldPhone)).AllX(ctx)
	require.Equal(t, "0000", users[0].Phone)
	require.Equal(t, "A", users[0].Name)
	require.Equal(t, "1111", users[1].Phone)
	require.Equal(t, "B", users[1].Name)

	// Setting primary key manually.
	a := client.Item.Create().With(item.F.ID.Set("A")).SaveX(ctx)
	require.Equal(t, "A", a.ID)
	if strings.Contains(t.Name(), "MySQL") || strings.Contains(t.Name(), "Maria") {
		// MySQL is skipped since it does not support the RETURNING clause. Maria is skipped
		// as well, because there's no way to distinguish between MySQL and Maria at runtime.
		client.Item.Create().With(item.F.ID.Set("A")).OnConflict().Ignore().ExecX(ctx)
		require.Equal(t, 1, client.Item.Query().CountX(ctx))
		client.Item.Delete().ExecX(ctx)

		// Primary key is set by a default function.
		b := client.Item.Create().With(item.F.Text.Set("hello")).SaveX(ctx)
		require.NotZero(t, b.ID)
		client.Item.Create().With(item.F.ID.Set(b.ID), item.F.Text.Set("world")).OnConflict().UpdateNewValues().ExecX(ctx)
		cb := client.Item.Query().OnlyX(ctx)
		require.Equal(t, cb.ID, b.ID)
		require.Equal(t, "world", cb.Text)
	} else {
		aid := client.Item.Create().With(item.F.ID.Set("A")).OnConflict(sql.ConflictColumns(item.FieldID)).Ignore().IDX(ctx)
		require.Equal(t, a.ID, aid)
		client.Item.Delete().ExecX(ctx)

		// Primary key is set by a default function.
		b := client.Item.Create().With(item.F.Text.Set("hello")).SaveX(ctx)
		require.NotZero(t, b.ID)
		bid := client.Item.Create().With(item.F.ID.Set(b.ID), item.F.Text.Set("hello")).OnConflictColumns(item.FieldText).Ignore().IDX(ctx)
		require.Equal(t, b.ID, bid)
		bid = client.Item.Create().With(item.F.Text.Set("hello")).OnConflictColumns(item.FieldText).UpdateNewValues().IDX(ctx)
		require.Equal(t, bid, b.ID)
		require.Equal(t, bid, client.Item.Query().OnlyIDX(ctx))
		bid = client.Item.Create().With(item.F.ID.Set(bid), item.F.Text.Set("world")).OnConflictColumns(item.FieldID).UpdateNewValues().IDX(ctx)
		require.Equal(t, bid, b.ID)
		b = client.Item.Query().OnlyX(ctx)
		require.Equal(t, bid, b.ID)
		require.Equal(t, "world", b.Text)

		client.Item.CreateBulk(client.Item.Create().With(item.F.ID.Set(bid), item.F.Text.Set("hello"))).
			OnConflictColumns(item.FieldID).
			Ignore().
			ExecX(ctx)
		require.Equal(t, bid, client.Item.Query().OnlyIDX(ctx))
	}

	ts := time.Unix(1623279251, 0)
	c1 := client.Card.Create().With(card.F.Number.Set("102030"), card.F.CreateTime.Set(ts), card.F.UpdateTime.Set(ts)).
		SaveX(ctx)

	// "DO UPDATE SET ... WHERE ..." does not support by MySQL.
	if strings.Contains(t.Name(), "Postgres") || strings.Contains(t.Name(), "SQLite") {
		err = client.Card.Create().With(card.F.Number.Set(c1.Number)).
			OnConflict(
				sql.ConflictColumns(card.FieldNumber),
				sql.UpdateWhere(sql.NEQ(card.FieldCreateTime, ts)),
			).
			UpdateNewValues().
			Exec(ctx)
		// Only rows for which the "UpdateWhere" expression
		// returns true will be updated. That is, none.
		require.True(t, errors.Is(err, stdsql.ErrNoRows))

		id = client.Card.Create().With(card.F.Number.Set(c1.Number)).
			OnConflict(
				sql.ConflictColumns(card.FieldNumber),
				sql.UpdateWhere(sql.EQ(card.FieldCreateTime, ts)),
			).
			UpdateNewValues().
			IDX(ctx)
	} else {
		id = client.Card.Create().With(card.F.Number.Set(c1.Number)).
			OnConflictColumns(card.FieldNumber).
			UpdateNewValues().
			IDX(ctx)
	}

	// Ensure immutable fields were not changed during upsert.
	c2 := client.Card.GetX(ctx, id)
	require.Equal(t, c1.CreateTime.Unix(), c2.CreateTime.Unix())
	require.NotEqual(t, c1.UpdateTime.Unix(), c2.UpdateTime.Unix())

	// Ensure immutable fields were not changed during bulk upsert.
	l1 := client.License.Create().With(license.F.CreateTime.Set(ts), license.F.UpdateTime.Set(ts)).SaveX(ctx)
	client.License.CreateBulk(client.License.Create().With(license.F.ID.Set(l1.ID))).
		OnConflictColumns(license.FieldID).
		UpdateNewValues().
		ExecX(ctx)
	l2 := client.License.GetX(ctx, l1.ID)
	require.Equal(t, l1.CreateTime.Unix(), l2.CreateTime.Unix())
	require.NotEqual(t, l1.UpdateTime.Unix(), l2.UpdateTime.Unix())

	c3 := client.Card.Create().With(card.F.Name.Set("a8m"), card.F.Number.Set("405060")).SaveX(ctx)
	client.Card.Create().With(card.F.Number.Set(c3.Number)).OnConflictColumns(card.FieldNumber).Clear(card.FieldName).UpdateNewValues().ExecX(ctx)
	require.Empty(t, client.Card.GetX(ctx, c3.ID).Name)
	client.Card.UpdateOne(c3).With(card.F.Name.Set("a8m")).ExecX(ctx)
	client.Card.CreateBulk(client.Card.Create().With(card.F.Number.Set(c3.Number)), client.Card.Create().With(card.F.Number.Set("708090"), card.F.Name.Set("m8a"))).
		OnConflictColumns(card.FieldNumber).
		UpdateNewValues().
		ExecX(ctx)
	require.Empty(t, client.Card.GetX(ctx, c3.ID).Name, "existing name fields should be cleared when not set (= set to nil)")
	require.NotEmpty(t, client.Card.Query().Where(card.F.Number.EQ("708090")).OnlyX(ctx).Name, "new record should set their name")

	// Conflict on a composite unique index.
	t1 := client.Task.Create().With(enttask.F.Name.Set("todo1"), enttask.F.Owner.Set("a8m"), enttask.F.Priority.Set(task.PriorityLow)).SaveX(ctx)
	tid := client.Task.Create().With(enttask.F.Name.Set("todo1"), enttask.F.Owner.Set("a8m"), enttask.F.Priority.Set(task.PriorityHigh)).
		OnConflictColumns(enttask.FieldName, enttask.FieldOwner).
		UpdateFields(enttask.FieldPriority).
		IDX(ctx)
	require.Equal(t, t1.ID, tid)
	require.Equal(t, task.PriorityHigh, client.Task.GetX(ctx, tid).Priority)
}

func Clone(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	f1 := client.File.Create().With(file.F.Name.Set("foo"), file.F.Size.Set(10)).SaveX(ctx)
	f2 := client.File.Create().With(file.F.Name.Set("foo"), file.F.Size.Set(20)).SaveX(ctx)
	base := client.File.Query().Where(file.F.Name.EQ("foo"))
	require.Equal(t, f1.Size, base.Clone().Where(file.F.Size.EQ(f1.Size)).OnlyX(ctx).Size)
	require.Equal(t, f2.Size, base.Clone().Where(file.F.Size.EQ(f2.Size)).OnlyX(ctx).Size)
	// ensure clone emits valid code.
	query := ent.QueryPetTeamFromQuery(client.Pet.Query().Where(pet.F.Name.EQ("unknown")))
	for i := 0; i < 10; i++ {
		_, err := query.Clone().Where(user.F.Name.EQ("unknown")).First(ctx)
		require.True(t, ent.IsNotFound(err), "should not return syntax error")
	}
}

func Paging(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	for i := 1; i <= 10; i++ {
		client.User.Create().With(user.F.Name.Set(fmt.Sprintf("name-%d", i)), user.F.Age.Set(i)).SaveX(ctx)
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
	u := client.User.Create().With(user.F.Name.Set("foo"), user.F.Age.Set(30)).SaveX(ctx)
	name := client.User.
		Query().
		Where(user.F.ID.EQ(u.ID)).
		Select(user.FieldName).
		StringX(ctx)
	require.Equal("foo", name)
	client.User.Create().With(user.F.Name.Set("bar"), user.F.Age.Set(30), user.E.Friends.AddIDs(u.ID)).SaveX(ctx)
	t.Log("select one field with ordering")
	names := client.User.
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
	client.User.Create().With(user.F.Name.Set("baz"), user.F.Age.Set(30)).SaveX(ctx)
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

	users := ent.WithUserFriends(client.User.
		Query().
		Select(user.FieldAge).
		Where(user.F.Name.EQ("foo")), func(q *ent.UserQuery) {
		q.Select(user.FieldName)
	}).
		AllX(ctx)
	for i := range users {
		require.Empty(users[i].Name)
		require.NotZero(users[i].ID)
		require.NotZero(users[i].Age)
		for _, f := range users[i].Edges.Friends {
			require.NotEmpty(f.Name)
			require.NotZero(f.ID)
			require.Zero(f.Age)
		}
	}
	a8m := client.User.Create().With(user.F.Name.Set("Ariel"), user.F.Nickname.Set("a8m"), user.F.Age.Set(30)).SaveX(ctx)
	require.NotEmpty(a8m.ID)
	require.NotEmpty(a8m.Age)
	require.NotEmpty(a8m.Name)
	require.NotEmpty(a8m.Nickname)
	a8m = client.User.UpdateOne(a8m).With(user.F.Age.Set(32)).Select(user.FieldAge).SaveX(ctx)
	require.NotEmpty(a8m.ID)
	require.NotEmpty(a8m.Age)
	require.Empty(a8m.Name)
	require.Empty(a8m.Nickname)

	client.Pet.CreateBulk(
		client.Pet.Create().With(pet.F.Name.Set("a")),
		client.Pet.Create().With(pet.F.Name.Set("a")),
	).ExecX(ctx)
	names = client.Pet.Query().Select(pet.FieldName).StringsX(ctx)
	require.Equal([]string{"a", "a"}, names)
	names = client.Pet.Query().Unique(true).Select(pet.FieldName).StringsX(ctx)
	require.Equal([]string{"a"}, names)
	client.Pet.Delete().ExecX(ctx)

	pets := client.Pet.CreateBulk(
		client.Pet.Create().With(pet.F.Name.Set("a")),
		client.Pet.Create().With(pet.F.Name.Set("b")),
		client.Pet.Create().With(pet.F.Name.Set("c")),
		client.Pet.Create().With(pet.F.Name.Set("b")),
	).SaveX(ctx)
	client.User.Create().With(user.F.Name.Set("foo"), user.F.Age.Set(20), user.E.Pets.AddIDs(pets[0].ID, pets[1].ID)).SaveX(ctx)
	client.User.Create().With(user.F.Name.Set("bar"), user.F.Age.Set(20), user.E.Pets.AddIDs(pets[2].ID, pets[3].ID)).SaveX(ctx)
	names = client.Pet.Query().Order(ent.Asc(pet.FieldID)).Select(pet.FieldName).StringsX(ctx)
	require.Equal([]string{"a", "b", "c", "b"}, names)
	names = client.Pet.Query().Order(ent.Asc(pet.FieldName)).Select(pet.FieldName).StringsX(ctx)
	require.Equal([]string{"a", "b", "b", "c"}, names)
	names = client.Pet.Query().
		Order(func(s *sql.Selector) {
			// Join with user table for ordering by owner-name
			// and pet-name (edge + field ordering).
			t := sql.Table(user.Table)
			s.Join(t).On(s.C(pet.OwnerColumn), t.C(user.FieldID))
			s.OrderBy(t.C(user.FieldName), s.C(pet.FieldName))
		}).
		Select(pet.FieldName).
		StringsX(ctx)
	require.Equal([]string{"b", "c", "a", "b"}, names)

	var ps []*ent.Pet
	client.Pet.Query().Select().ScanX(ctx, &ps)
	require.Len(ps, 4, "support scanning nodes manually")

	lens := client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.Select("LENGTH(name)")
		}).
		IntsX(ctx)
	require.Equal([]int{1, 1, 1, 1}, lens)

	dlen := client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.SelectExpr(sql.ExprFunc(func(b *sql.Builder) {
				b.WriteString("LENGTH(name)").WriteOp(sql.OpMul).Arg(2)
			}))
		}).
		IntsX(ctx)
	require.Equal([]int{2, 2, 2, 2}, dlen)

	for i := range pets {
		client.Pet.UpdateOne(pets[i]).With(pet.F.Name.Set(pets[i].Name + pets[i].Name)).ExecX(ctx)
	}
	n := client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.Select("SUM(LENGTH(name))")
		}).
		IntX(ctx)
	require.Equal(8, n)

	var (
		p1 []struct {
			ent.Pet
			NameLength int `sql:"length"`
		}
		p2 = client.Pet.Query().Order(ent.Asc(pet.FieldID)).AllX(ctx)
	)
	client.Pet.Query().
		Order(ent.Asc(pet.FieldID)).
		Modify(func(s *sql.Selector) {
			s.AppendSelect("LENGTH(name)")
		}).
		ScanX(ctx, &p1)
	for i := range p2 {
		require.Equal(p2[i].ID, p1[i].ID)
		require.Equal(p2[i].Age, p1[i].Age)
		require.Equal(p2[i].Name, p1[i].Name)
		require.Equal(len(p1[i].Name), p1[1].NameLength)
	}

	// Select count.
	names = client.Pet.Query().Order(ent.Asc(pet.FieldName)).Select(pet.FieldName).StringsX(ctx)
	require.Equal([]string{"aa", "bb", "bb", "cc"}, names)
	count := client.Pet.Query().Select(pet.FieldName).CountX(ctx)
	require.Equal(4, count)
	count = client.Pet.Query().Unique(true).Select(pet.FieldName).CountX(ctx)
	require.Equal(3, count)

	var (
		gs []struct {
			ent.Group
			UsersCount int `sql:"users_count"`
		}
		inf = client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx)
		hub = client.Group.Create().With(group.F.Name.Set("GitHub"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(a8m.ID)).SaveX(ctx)
		lab = client.Group.Create().With(group.F.Name.Set("GitLab"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(func() []int {
			ids := make([]int, len(users))
			for i := range users {
				ids[i] = users[i].ID
			}
			return ids
		}()...)).SaveX(ctx)
	)
	client.Group.Query().
		Order(ent.Asc(group.FieldID)).
		Modify(func(s *sql.Selector) {
			t := sql.Table(group.UsersTable)
			s.LeftJoin(t).
				On(
					s.C(group.FieldID),
					t.C(group.UsersPrimaryKey[1]),
				).
				// Append the "users_count" column to the selected columns.
				AppendSelect(
					sql.As(sql.Count(t.C(group.UsersPrimaryKey[1])), "users_count"),
				).
				GroupBy(s.C(group.FieldID))
		}).
		ScanX(ctx, &gs)
	require.Len(gs, 2)
	require.Equal(ent.QueryGroupUsers(client.Group, hub).CountX(ctx), gs[0].UsersCount)
	require.Equal(ent.QueryGroupUsers(client.Group, lab).CountX(ctx), gs[1].UsersCount)

	// Select Subquery.
	t.Log("select subquery")
	i, err := client.User.
		Query().
		Modify(func(s *sql.Selector) {
			subQuery := sql.SelectExpr(sql.Raw("1")).As("s")
			s.Select("*").From(subQuery)
		}).
		Int(ctx)
	require.NoError(err)
	require.Equal(1, i)

	// Select with join.
	u = client.User.Create().With(user.F.Name.Set("crossworth"), user.F.Age.Set(28)).SaveX(ctx)
	id := client.User.
		Query().
		Where(func(s *sql.Selector) {
			subQuery := sql.Select(user.FieldID).
				From(sql.Table(user.Table)).
				Where(sql.EQ(s.C(user.FieldName), "crossworth"))
			s.Join(subQuery).On(s.C(user.FieldID), subQuery.C(user.FieldID))
		}).
		OnlyIDX(ctx)
	require.Equal(u.ID, id)

	// Update modifiers.
	allUpper := func() bool {
		for _, name := range client.User.Query().Select(user.FieldName).StringsX(ctx) {
			if strings.ToUpper(name) != name {
				return false
			}
		}
		return true
	}
	require.False(allUpper(), "at least one name is not upper-cased")
	// Execute custom update modifier.
	client.User.Update().
		Modify(func(u *sql.UpdateBuilder) {
			u.Set(user.FieldName, sql.Expr(fmt.Sprintf("UPPER(%s)", user.FieldName)))
		}).
		ExecX(ctx)
	require.True(allUpper(), "at names must be upper-cased")

	// Select and scan dynamic values.
	const (
		as1 = "name_length"
		as2 = "another_name"
	)
	pets = client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.AppendSelectAs("LENGTH(name)", as1)
			s.AppendSelectAs("optional_time", as2)
		}).
		AllX(ctx)
	for _, p := range pets {
		n, err := p.Value(as1)
		require.NoError(err)
		require.EqualValues(len(p.Name), n)
		v, err := p.Value(as2)
		require.NoError(err)
		require.Nil(v)
	}

	// Update and scan.
	require.NoError(client.Pet.Update().With(pet.F.OptionalTime.Set(time.Now())).Exec(ctx))
	pets = client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.AppendSelectAs("optional_time", as2)
		}).
		AllX(ctx)
	for _, p := range pets {
		v, err := p.Value(as2)
		require.NoError(err)
		tv, ok := v.(time.Time)
		require.True(ok)
		require.True(!tv.IsZero())
	}

	// Order by random value should compile a valid query.
	_, err = client.User.Query().Order(sql.OrderByRand()).All(ctx)
	require.NoError(err)
}

func Aggregate(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	a8m := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("a8m")).SaveX(ctx)
	nat := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("nati"), user.E.Spouse.SetID(a8m.ID)).SaveX(ctx)
	owners := []*ent.User{a8m, nat}
	for i := 1; i <= 10; i++ {
		client.Pet.Create().With(pet.F.Name.Set(fmt.Sprintf("pet%d", i)), pet.F.Age.Set(float64(i)), pet.E.Owner.SetID(owners[i%2].ID)).SaveX(ctx)
	}
	s1 := client.Pet.Query().Aggregate(ent.Sum(pet.FieldAge)).IntX(ctx)
	require.Equal(t, 55, s1)
	s2 := client.Pet.Query().Where(pet.E.Owner.Has()).Aggregate(ent.Sum(pet.FieldAge)).IntX(ctx)
	require.Equal(t, s1, s2)

	// Aggregate traversals.
	require.Equal(t, 30, ent.QueryUserPets(client.User, a8m).Aggregate(ent.Sum(pet.FieldAge)).IntX(ctx))
	require.Equal(t, 25, ent.QueryUserPets(client.User, nat).Aggregate(ent.Sum(pet.FieldAge)).IntX(ctx))
	require.Equal(t, 25, ent.QueryUserPetsFromQuery(ent.QueryUserSpouse(client.User, a8m)).Aggregate(ent.Sum(pet.FieldAge)).IntX(ctx))
	require.Equal(t, 30, ent.QueryUserPetsFromQuery(ent.QueryUserSpouse(client.User, nat)).Aggregate(ent.Sum(pet.FieldAge)).IntX(ctx))

	// Aggregate 2 fields.
	var vs1 []struct{ Sum, Count int }
	client.Pet.Query().
		Aggregate(
			ent.Sum(pet.FieldAge),
			ent.Count(),
		).
		ScanX(ctx, &vs1)
	require.Len(t, vs1, 1)
	require.Equal(t, 55, vs1[0].Sum)
	require.Equal(t, 10, vs1[0].Count)

	// Aggregate 4 fields.
	var vs2 []struct {
		Sum, Min, Max, Count int
		Avg                  float64
	}
	client.Pet.Query().
		Aggregate(
			ent.Sum(pet.FieldAge),
			ent.Min(pet.FieldAge),
			ent.Max(pet.FieldAge),
			ent.Mean(pet.FieldAge),
			ent.Count(),
		).
		ScanX(ctx, &vs2)
	require.Len(t, vs2, 1)
	require.Equal(t, 55, vs2[0].Sum)
	require.Equal(t, 1, vs2[0].Min)
	require.Equal(t, 10, vs2[0].Max)
	require.Equal(t, 10, vs2[0].Count)
	require.Equal(t, 5.5, vs2[0].Avg)
}

func ExecQuery(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	var rows sql.Rows
	err := client.Driver().Query(ctx, "SELECT 1", []any{}, &rows)
	require.NoError(err)
	require.True(rows.Next())
	require.NoError(rows.Close())
	tx, err := client.Tx(ctx)
	require.NoError(err)
	tx.Task.Create().ExecX(ctx)
	require.Equal(1, tx.Task.Query().CountX(ctx))
	err = tx.Client().Driver().Query(ctx, "SELECT COUNT(*) FROM "+enttask.Table, []any{}, &rows)
	require.NoError(err)
	count, err := sql.ScanInt(rows)
	require.NoError(err)
	require.NoError(rows.Close())
	require.Equal(1, count)
	require.NoError(tx.Commit())
}

func NillableRequired(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	client.Task.Create().With(enttask.F.Name.Set("Name")).ExecX(ctx)
	tk := client.Task.Query().OnlyX(ctx)
	require.Empty(tk.Name, "Name is not selected by default")
	require.NotNil(tk.CreatedAt, "field value should be populated by default by the database")
	require.Error(client.Task.UpdateOne(tk).With(enttask.F.CreatedAt.Set(time.Now())).Exec(ctx), "immutable-nillable should reject a write on update")
	tk = client.Task.Query().Select(enttask.FieldID, enttask.FieldPriority, enttask.FieldName).OnlyX(ctx)
	require.Nil(tk.CreatedAt, "field should not be populated when it is not selected")
	require.Equal("Name", tk.Name, "Name should be populated when selected manually")
}

func Predicate(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	f1 := client.File.Create().With(file.F.Name.Set("1"), file.F.Size.Set(10)).SaveX(ctx)
	f2 := client.File.Create().With(file.F.Name.Set("2"), file.F.Size.Set(20)).SaveX(ctx)
	f3 := client.File.Create().With(file.F.Name.Set("3"), file.F.Size.Set(30)).SaveX(ctx)
	f4 := client.File.Create().With(file.F.Name.Set("4"), file.F.Size.Set(40)).SaveX(ctx)
	files := client.File.Query().
		Where(
			file.Or(file.F.Name.EQ(f1.Name), file.And(file.F.Name.EQ(f2.Name), file.F.Size.EQ(f2.Size))),
		).
		Order(ent.Asc(file.FieldName)).
		AllX(ctx)
	require.Equal(f1.Name, files[0].Name)
	require.Equal(f2.Name, files[1].Name)

	match := client.File.Query().
		Where(file.Or(file.F.Name.EQ(f1.Name), file.F.Name.EQ(f2.Name))).
		Where(file.F.Size.EQ(f1.Size)).
		OnlyX(ctx)
	require.Equal(f1.Name, match.Name)

	match = client.File.Query().
		Where(file.F.Size.EQ(f2.Size)).
		Where(file.Or(file.F.Name.EQ(f1.Name), file.F.Name.EQ(f2.Name))).
		OnlyX(ctx)
	require.Equal(f2.Name, match.Name)

	files = client.File.Query().
		Where(file.Or(file.F.Size.EQ(f3.Size), file.F.Size.EQ(f4.Size))).
		Where(file.Or(file.F.Name.EQ(f3.Name), file.F.Name.EQ(f4.Name))).
		Where(file.Not(file.Or(file.F.Name.EQ(f1.Name), file.F.Size.EQ(f1.Size)))).
		Order(ent.Asc(file.FieldName)).
		AllX(ctx)
	require.Equal(f3.Name, files[0].Name)
	require.Equal(f4.Name, files[1].Name)

	files = client.File.Query().
		Where(
			file.Or(file.F.Name.EQ(f4.Name), file.And(file.F.Name.EQ(f3.Name), file.F.Size.EQ(f3.Size))),
		).
		Order(ent.Asc(file.FieldName)).
		AllX(ctx)
	require.Equal(f3.Name, files[0].Name)
	require.Equal(f4.Name, files[1].Name)

	require.Zero(client.File.Query().Where(file.F.User.NotNil()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.F.User.IsNil()).CountX(ctx))
	require.Zero(client.File.Query().Where(file.F.Group.NotNil()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.F.Group.IsNil()).CountX(ctx))

	f1 = client.File.UpdateOne(f1).With(file.F.User.Set("a8m")).SaveX(ctx)
	require.NotNil(f1.User)
	require.Equal("a8m", *f1.User)
	require.Equal(3, client.File.Query().Where(file.F.User.IsNil()).CountX(ctx))
	require.Equal(f1.Name, client.File.Query().Where(file.F.User.NotNil()).OnlyX(ctx).Name)
	f5 := client.File.Create().With(file.F.Name.Set("5"), file.F.Size.Set(40), file.F.User.Set("mashraki")).SaveX(ctx)
	require.NotNil(f5.User)
	require.Equal("mashraki", *f5.User)
	require.Equal(3, client.File.Query().Where(file.F.User.IsNil()).CountX(ctx))
	require.Equal(2, client.File.Query().Where(file.F.User.NotNil()).CountX(ctx))

	require.Equal(5, client.File.Query().Where(file.F.Group.IsNil()).CountX(ctx))
	f4 = client.File.UpdateOne(f4).With(file.F.Group.Set("fbc")).SaveX(ctx)
	require.Equal(1, client.File.Query().Where(file.F.Group.NotNil()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.F.Group.IsNil()).CountX(ctx))
	require.Equal(
		5,
		client.File.Query().
			Where(
				file.Or(file.F.Group.IsNil(), file.And(file.F.Group.NotNil(), file.F.Name.EQ(f4.Name))),
			).
			CountX(ctx),
	)

	inf := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx)
	hub := client.Group.Create().With(group.F.Name.Set("GitHub"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID)).SaveX(ctx)
	lab := client.Group.Create().With(group.F.Name.Set("GitLab"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID), group.F.Active.Set(false)).SaveX(ctx)
	require.Equal(hub.ID, client.Group.Query().Where(group.F.Active.EQ(true)).OnlyIDX(ctx))
	require.Equal(lab.ID, client.Group.Query().Where(group.F.Active.EQ(false)).OnlyIDX(ctx))
	require.Equal(hub.ID, client.Group.Query().Where(group.F.Active.NEQ(false)).OnlyIDX(ctx))
	require.Equal(lab.ID, client.Group.Query().Where(group.F.Active.NEQ(true)).OnlyIDX(ctx))

	client.User.CreateBulk(
		client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("Ariel"), user.F.Nickname.Set("A")),
		client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("Ariel"), user.F.Nickname.Set("A%")),
	).ExecX(ctx)
	a1 := client.User.Query().Where(sql.FieldsHasPrefix(user.FieldName, user.FieldNickname)).OnlyX(ctx)
	require.Equal("A", a1.Nickname)
	a2 := client.User.Query().Where(user.Not(sql.FieldsHasPrefix(user.FieldName, user.FieldNickname))).OnlyX(ctx)
	require.Equal("A%", a2.Nickname)
}

func AddValues(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	t.Log("add values to fields")
	cmt := client.Comment.Create().With(comment.F.UniqueInt.Set(1), comment.F.UniqueFloat.Set(1)).SaveX(ctx)
	cmt = client.Comment.UpdateOne(cmt).With(comment.F.UniqueInt.Add(10)).SaveX(ctx)
	require.Equal(11, cmt.UniqueInt)
	require.Equal(11, client.Comment.Query().OnlyX(ctx).UniqueInt, "should be updated in the database")
	t.Log("add values to null fields")
	cmt = client.Comment.UpdateOne(cmt).With(comment.F.NillableInt.Add(10)).SaveX(ctx)
	require.Equal(10, *cmt.NillableInt)

	cmt1 := client.Comment.Create().With(comment.F.UniqueInt.Set(1), comment.F.UniqueFloat.Set(10)).SaveX(ctx)
	err := client.Comment.UpdateOne(cmt1).With(comment.F.UniqueInt.Add(10)).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	cmt1 = client.Comment.UpdateOne(cmt1).With(comment.F.UniqueInt.Add(20), comment.F.NillableInt.Add(20)).SaveX(ctx)
	require.Equal(21, cmt1.UniqueInt)
	require.Equal(20, *cmt1.NillableInt)

	cmt1 = client.Comment.UpdateOne(cmt1).With(comment.F.UniqueInt.Add(10), comment.F.UniqueInt.Add(-1)).SaveX(ctx)
	require.Equal(30, cmt1.UniqueInt)
	require.Equal(30, client.Comment.GetX(ctx, cmt1.ID).UniqueInt)
}

func Delete(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	nd := client.Node.Create().With(node.F.Value.Set(1e3)).SaveX(ctx)
	err := client.Node.DeleteOneID(nd.ID).Exec(ctx)
	require.NoError(err)
	err = client.Node.DeleteOneID(nd.ID).Exec(ctx)
	require.True(ent.IsNotFound(err))

	for i := 0; i < 5; i++ {
		client.Node.Create().With(node.F.Value.Set(i)).ExecX(ctx)
	}
	affected, err := client.Node.Delete().Where(node.F.Value.GT(2)).Exec(ctx)
	require.NoError(err)
	require.Equal(2, affected)

	affected, err = client.Node.Delete().Exec(ctx)
	require.NoError(err)
	require.Equal(3, affected)

	info := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("group info")).SaveX(ctx)
	hub := client.Group.Create().With(group.E.Info.SetID(info.ID), group.F.Name.Set("GitHub"), group.F.Expire.Set(time.Now().Add(time.Hour))).SaveX(ctx)
	err = client.GroupInfo.DeleteOne(info).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	// Group.DeleteOneID(id).Where(...), is identical to Group.Delete().Where(group.ID(id), ...),
	// but, in case the OpDelete is not an allowed operation, the DeleteOne can be used with Where.
	n, err := client.Group.Delete().
		Where(group.F.ID.EQ(hub.ID), group.F.Expire.LT(time.Now())). // Expired.
		Exec(ctx)
	require.Zero(n)
	require.NoError(err)

	err = client.Group.DeleteOne(hub).
		Where(group.F.Expire.LT(time.Now())).
		Exec(ctx)
	require.True(ent.IsNotFound(err))
	client.Group.UpdateOne(hub).With(group.F.Expire.Set(time.Now().Add(-time.Hour))).ExecX(ctx)
	client.Group.DeleteOne(hub).
		Where(group.F.Expire.LT(time.Now())).
		ExecX(ctx)

	// The behavior described above it also applied to UpdateOne.
	hub = client.Group.Create().With(group.E.Info.SetID(info.ID), group.F.Name.Set("GitHub"), group.F.Expire.Set(time.Now().Add(time.Hour))).SaveX(ctx)
	err = client.Group.UpdateOne(hub).With(group.F.Active.Set(false), group.F.Expire.Set(time.Time{})).
		Where(group.F.Expire.LT(time.Now())). // Expired.
		Exec(ctx)
	require.True(ent.IsNotFound(err))
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
		Create().With(groupinfo.F.Desc.Set("group info")).
		SaveX(ctx)
	t.Logf("group info created: %v", info)

	t.Log("creating group")
	grp := client.Group.
		Create().With(group.E.Info.SetID(info.ID), group.F.Name.Set("Github"), group.F.Expire.Set(time.Now().Add(time.Hour))).
		SaveX(ctx)
	require.NotZero(grp.ID)
	require.Equal(grp.MaxUsers, 10)
	require.Equal(grp.Name, "Github")
	t.Logf("group created: %v", grp)

	t.Log("creating user")
	usr := client.User.
		Create().With(user.F.Age.Set(20), user.F.Name.Set("a8m"), user.E.Groups.AddIDs(grp.ID)).
		SaveX(ctx)
	require.NotZero(usr.ID)
	require.Equal(usr.Age, 20)
	require.Equal(usr.Name, "a8m")
	require.Equal(usr.Last, "unknown")
	t.Logf("user created: %v", usr)

	t.Log("querying assoc edges")
	groups := ent.QueryUserGroups(client.User, usr).IDsX(ctx)
	require.NotEmpty(groups)
	require.Equal(grp.ID, groups[0])
	t.Log("querying inverse edge")
	users := ent.QueryGroupUsers(client.Group, grp).IDsX(ctx)
	require.NotEmpty(users)
	require.Equal(usr.ID, users[0])

	t.Log("remove group edge")
	client.User.UpdateOne(usr).With(user.E.Groups.RemoveIDs(grp.ID)).ExecX(ctx)
	require.Empty(ent.QueryGroupUsers(client.Group, grp).AllX(ctx))
	require.Empty(ent.QueryUserGroups(client.User, usr).AllX(ctx))
	t.Logf("add group edge")
	client.User.UpdateOne(usr).With(user.E.Groups.AddIDs(grp.ID)).ExecX(ctx)
	require.NotEmpty(ent.QueryGroupUsers(client.Group, grp).AllX(ctx))
	require.NotEmpty(ent.QueryUserGroups(client.User, usr).AllX(ctx))
	t.Log("remove users inverse edge")
	client.Group.UpdateOne(grp).With(group.E.Users.RemoveIDs(usr.ID)).ExecX(ctx)
	require.Empty(ent.QueryGroupUsers(client.Group, grp).AllX(ctx))
	require.Empty(ent.QueryUserGroups(client.User, usr).AllX(ctx))
	t.Logf("add group inverse edge")
	client.Group.UpdateOne(grp).With(group.E.Users.AddIDs(usr.ID)).ExecX(ctx)
	require.NotEmpty(ent.QueryGroupUsers(client.Group, grp).AllX(ctx))
	require.NotEmpty(ent.QueryUserGroups(client.User, usr).AllX(ctx))

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
	require.Zero(client.User.Query().Where(user.E.Spouse.Has()).CountX(ctx))
	neta := client.User.Create().With(user.F.Name.Set("neta"), user.F.Age.Set(18), user.E.Spouse.SetID(usr.ID)).SaveX(ctx)
	require.Equal(2, client.User.Query().Where(user.E.Spouse.Has()).CountX(ctx))

	t.Log("check for singular error")
	_, err = client.User.Query().Only(ctx)
	require.True(ent.IsNotSingular(err))

	t.Log("query parent/children edges")
	require.False(ent.QueryUserParent(client.User, usr).ExistX(ctx))
	require.Empty(ent.QueryUserChildren(client.User, usr).AllX(ctx))
	child := client.User.Create().With(user.F.Name.Set("pedro"), user.F.Age.Set(7), user.E.Parent.SetID(usr.ID)).SaveX(ctx)
	require.Equal(usr.Name, ent.QueryUserParent(client.User, child).OnlyX(ctx).Name)
	require.Equal(child.Name, ent.QueryUserChildren(client.User, usr).OnlyX(ctx).Name)
	require.False(ent.QueryUserParent(client.User, usr).ExistX(ctx))

	t.Log("clear parent edge")
	brat := client.User.Create().With(user.F.Name.Set("brat"), user.F.Age.Set(19), user.E.Parent.SetID(usr.ID)).SaveX(ctx)
	require.Equal(2, ent.QueryUserChildren(client.User, usr).CountX(ctx))
	brat = client.User.UpdateOne(brat).With(user.E.Parent.Clear()).SaveX(ctx)
	err = client.User.UpdateOne(brat).With(user.E.Parent.Clear()).Exec(ctx)
	require.NoError(err)
	require.False(ent.QueryUserParent(client.User, brat).ExistX(ctx))
	require.Equal(1, ent.QueryUserChildren(client.User, usr).CountX(ctx))

	t.Log("delete child clears edge")
	brat = client.User.UpdateOne(brat).With(user.E.Parent.SetID(usr.ID)).SaveX(ctx)
	require.Equal(2, ent.QueryUserChildren(client.User, usr).CountX(ctx))
	client.User.DeleteOne(brat).ExecX(ctx)
	require.Equal(1, ent.QueryUserChildren(client.User, usr).CountX(ctx))

	client.Group.UpdateOne(grp).With(group.E.Blocked.AddIDs(neta.ID)).ExecX(ctx)
	blocked := ent.QueryGroupBlocked(client.Group, ent.QueryUserGroups(client.User, usr).OnlyX(ctx)).OnlyX(ctx)
	t.Log("blocked:", blocked)

	t.Log("query users with or condition")
	require.Len(client.User.Query().Where(user.Or(user.F.Name.EQ("a8m"), user.F.Name.EQ("neta"))).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Or(user.F.Name.EQ("a8m"), user.F.Name.EQ("noam"))).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Or(user.F.Name.EQ("alex"), user.F.Name.EQ("noam"))).AllX(ctx), 0)

	t.Log("query using the in predicate")
	require.Len(client.User.Query().Where(user.F.Name.In("a8m", "neta")).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.F.Name.In("a8m", "alex")).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.F.ID.In(neta.ID)).AllX(ctx), 1)

	t.Log("query existence")
	require.True(client.User.Query().Where(user.F.Name.EQ("a8m")).Exist(ctx))
	require.False(client.User.Query().Where(user.F.Name.EQ("alex")).Exist(ctx))

	t.Log("query using get")
	require.Equal(usr.Name, client.User.GetX(ctx, usr.ID).Name)
	uid, err := client.User.Query().Where(user.F.ID.EQ(usr.ID), user.Not(user.F.Name.EQ(usr.Name))).Only(ctx)
	require.Error(err)
	require.Nil(uid)

	t.Log("test validators")
	err = client.Group.Create().With(group.E.Info.SetID(info.ID), group.F.Type.Set("a"), group.F.Name.Set("Gituhb"), group.F.Expire.Set(time.Now().Add(time.Hour))).Exec(ctx)
	require.Error(err, "type validator failed")
	err = client.Group.Create().With(group.E.Info.SetID(info.ID), group.F.Type.Set("pass"), group.F.Name.Set("failed"), group.F.Expire.Set(time.Now().Add(time.Hour))).Exec(ctx)
	require.Error(err, "name validator failed")
	var checkerr schema.CheckError
	require.True(errors.As(err, &checkerr))
	require.EqualError(err, `ent: validator failed for field "Group.name": last name must begin with uppercase`)
	require.EqualError(checkerr, "last name must begin with uppercase")
	err = client.Group.Create().With(group.E.Info.SetID(info.ID), group.F.Type.Set("pass"), group.F.Name.Set("Github20"), group.F.Expire.Set(time.Now().Add(time.Hour))).Exec(ctx)
	require.Error(err, "name validator failed")
	err = client.Group.Create().With(group.E.Info.SetID(info.ID), group.F.Type.Set("pass"), group.F.Name.Set("Github"), group.F.MaxUsers.Set(-1), group.F.Expire.Set(time.Now().Add(time.Hour))).Exec(ctx)
	require.Error(err, "max_users validator failed")
	err = client.Group.Update().With(group.F.MaxUsers.Set(-10)).Exec(ctx)
	require.Error(err, "max_users validator failed")
	err = client.Group.UpdateOne(grp).With(group.F.MaxUsers.Set(-10)).Exec(ctx)
	require.Error(err, "max_users validator failed")
	_, err = client.Group.Query().Select("unknown_field").String(ctx)
	require.EqualError(err, "ent: invalid field \"unknown_field\" for query")
	_, err = client.Group.Query().GroupBy("unknown_field").String(ctx)
	require.EqualError(err, "ent: invalid field \"unknown_field\" for query")
	_, err = client.User.Query().Order(ent.Asc("invalid")).Only(ctx)
	require.EqualError(err, "ent: unknown column \"invalid\" for table \"users\"")
	_, err = ent.QueryUserFollowingFromQuery(client.User.Query().Order(ent.Asc("invalid"))).Only(ctx)
	require.EqualError(err, "ent: unknown column \"invalid\" for table \"users\"")
	_, err = client.User.Query().GroupBy("name").Aggregate(ent.Sum("invalid")).String(ctx)
	require.EqualError(err, "ent: unknown column \"invalid\" for table \"users\"")

	t.Log("query using edge-with predicate")
	require.Len(ent.QueryUserGroups(client.User, usr).Where(group.E.Info.HasWith(groupinfo.F.Desc.EQ("group info"))).AllX(ctx), 1)
	require.Empty(ent.QueryUserGroups(client.User, usr).Where(group.E.Info.HasWith(groupinfo.F.Desc.EQ("missing info"))).AllX(ctx))
	t.Log("query using edge-with predicate on inverse edges")
	require.Len(client.Group.Query().Where(group.F.Name.EQ("Github"), group.E.Users.HasWith(user.F.Name.EQ("a8m"))).AllX(ctx), 1)
	require.Empty(client.Group.Query().Where(group.F.Name.EQ("Github"), group.E.Users.HasWith(user.F.Name.EQ("alex"))).AllX(ctx))
	t.Logf("query path using edge-with predicate")
	require.Len(client.GroupInfo.Query().Where(groupinfo.E.Groups.HasWith(group.E.Users.HasWith(user.F.Name.EQ("a8m")))).AllX(ctx), 1)
	require.Empty(client.GroupInfo.Query().Where(groupinfo.E.Groups.HasWith(group.E.Users.HasWith(user.F.Name.EQ("alex")))).AllX(ctx))
	require.Len(client.GroupInfo.Query().Where(groupinfo.Or(groupinfo.F.Desc.EQ("group info"), groupinfo.E.Groups.HasWith(group.E.Users.HasWith(user.F.Name.EQ("alex"))))).AllX(ctx), 1)

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
	require.Len(ent.QueryGroupUsersFromQuery(client.Group.Query()).AllX(ctx), 1)
	require.Empty(ent.QueryGroupUsersFromQuery(client.Group.Query().Where(group.F.Name.EQ("boring"))).AllX(ctx))
	require.Equal(neta.Name, ent.QueryUserSpouseFromQuery(ent.QueryGroupUsersFromQuery(ent.QueryUserGroups(client.User, usr).Where(group.F.Name.EQ("Github")))).OnlyX(ctx).Name)
	require.Empty(ent.QueryGroupInfoGroupsFromQuery(client.GroupInfo.Query().Where(groupinfo.F.Desc.EQ("group info"))).Where(group.F.Name.EQ("boring")).AllX(ctx))
	require.Equal(child.Name, ent.QueryUserChildrenFromQuery(ent.QueryGroupUsersFromQuery(ent.QueryGroupInfoGroupsFromQuery(client.GroupInfo.Query().Where(groupinfo.F.Desc.EQ("group info"))).Where(group.F.Name.EQ("Github")))).FirstX(ctx).Name)
	client.User.UpdateOne(neta).With(user.E.Groups.AddIDs(grp.ID)).ExecX(ctx)
	require.Equal(grp.ID, ent.QueryUserGroupsFromQuery(client.User.Query()).OnlyIDX(ctx))

	t.Log("query using string predicate")
	require.Len(client.User.Query().Where(user.F.Name.In("a8m", "neta", "pedro")).AllX(ctx), 3)
	require.Empty(client.User.Query().Where(user.F.Name.NotIn("a8m", "neta", "pedro")).AllX(ctx))
	require.Empty(client.User.Query().Where(user.F.Name.In("alex", "rocket")).AllX(ctx))
	require.NotNil(client.User.Query().Where(user.E.Parent.HasWith(user.F.Name.In("a8m", "neta"))).OnlyX(ctx))
	require.Len(client.User.Query().Where(user.F.Name.Contains("a8")).AllX(ctx), 1)
	require.Equal(1, client.User.Query().Where(user.F.Name.HasPrefix("a8")).CountX(ctx))
	require.Zero(client.User.Query().Where(user.F.Name.HasPrefix("%a8%")).CountX(ctx))
	require.Equal(2, client.User.Query().Where(user.Or(user.F.Name.HasPrefix("a8"), user.F.Name.HasSuffix("eta"))).CountX(ctx))

	t.Log("group-by one field")
	names, err := client.User.Query().GroupBy(user.FieldName).Strings(ctx)
	require.NoError(err)
	sort.Strings(names)
	require.Equal([]string{"a8m", "neta", "pedro"}, names)
	ages, err := client.User.Query().GroupBy(user.FieldAge).Ints(ctx)
	require.NoError(err)
	require.Len(ages, 3)
	age, err := client.User.Query().Where(user.F.Name.EQ("alexsn")).GroupBy(user.FieldAge).Int(ctx)
	require.True(ent.IsNotFound(err))
	require.Zero(age)

	t.Log("group-by two fields with aggregation")
	client.User.Create().With(user.F.Name.Set(usr.Name), user.F.Age.Set(usr.Age)).ExecX(ctx)
	client.User.Create().With(user.F.Name.Set(neta.Name), user.F.Age.Set(neta.Age)).ExecX(ctx)
	child2 := client.User.Create().With(user.F.Name.Set(child.Name), user.F.Age.Set(child.Age+1)).SaveX(ctx)
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

	t.Log("group by a relation")
	p1 := client.Pet.Create().With(pet.F.Name.Set("a"), pet.F.Age.Set(10)).SaveX(ctx)
	p2 := client.Pet.Create().With(pet.F.Name.Set("b"), pet.F.Age.Set(7)).SaveX(ctx)
	foo := client.User.Create().With(user.F.Name.Set("foo"), user.F.Age.Set(10), user.E.Pets.AddIDs(p1.ID, p2.ID)).SaveX(ctx)
	p3 := client.Pet.Create().With(pet.F.Name.Set("c"), pet.F.Age.Set(14)).SaveX(ctx)
	p4 := client.Pet.Create().With(pet.F.Name.Set("d"), pet.F.Age.Set(1)).SaveX(ctx)
	bar := client.User.Create().With(user.F.Name.Set("bar"), user.F.Age.Set(10), user.E.Pets.AddIDs(p3.ID, p4.ID)).SaveX(ctx)

	var v3 []struct {
		ID      int
		Name    string
		Average float64
	}
	client.User.Query().
		Where(user.F.ID.In(foo.ID, bar.ID)).
		Order(ent.Asc(user.FieldID)).
		GroupBy(user.FieldID, user.FieldName).
		Aggregate(func(s *sql.Selector) string {
			// Join with pet table and calculate the
			// average age of the pets of each user.
			t := sql.Table(pet.Table)
			s.Join(t).On(s.C(user.FieldID), t.C(pet.OwnerColumn))
			return sql.As(sql.Avg(t.C(pet.FieldAge)), "average")
		}).
		ScanX(ctx, &v3)
	require.Len(v3, 2)
	require.Equal(foo.ID, v3[0].ID)
	require.Equal(foo.Name, v3[0].Name)
	require.Equal(8.5, v3[0].Average)
	require.Equal(bar.ID, v3[1].ID)
	require.Equal(bar.Name, v3[1].Name)
	require.Equal(7.5, v3[1].Average)

	var v4 []struct {
		ID    int    `sql:"id"`
		Name  string `sql:"name"`
		Owner string `sql:"owner"`
	}
	client.Pet.Query().
		Where(func(s *sql.Selector) {
			t := sql.Table(user.Table).As(user.Table)
			s.Join(t).On(s.C(pet.OwnerColumn), t.C(user.FieldID)) // owner_id = id for edge fields.
			s.AppendSelect(sql.As(t.C(user.FieldName), "owner"))
		}).
		Order(ent.Asc(pet.FieldID)).
		Select(pet.FieldID, pet.FieldName).
		ScanX(ctx, &v4)
	require.Equal(v4[0].Name, "a")
	require.Equal(v4[0].Owner, "foo")
	require.Equal(v4[1].Name, "b")
	require.Equal(v4[1].Owner, "foo")
	require.Equal(v4[2].Name, "c")
	require.Equal(v4[2].Owner, "bar")
	require.Equal(v4[3].Name, "d")
	require.Equal(v4[3].Owner, "bar")
}

func ClearFields(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	img := client.File.Create().With(file.F.Name.Set("foo"), file.F.Size.Set(100), file.F.User.Set("a8m"), file.F.Group.Set("Github")).SaveX(ctx)

	t.Log("clear one field")
	img = client.File.UpdateOne(img).With(file.F.User.Clear()).SaveX(ctx)
	require.Nil(t, img.User)
	img = client.File.Query().OnlyX(ctx)
	require.Nil(t, img.User)
	require.Equal(t, "Github", img.Group)

	t.Log("clear many fields")
	img = client.File.UpdateOne(img).With(file.F.User.Clear(), file.F.Group.Clear()).SaveX(ctx)
	require.Nil(t, img.User)
	img = client.File.Query().OnlyX(ctx)
	require.Nil(t, img.User)
	require.Empty(t, img.Group)

	t.Log("revert previous set")
	img = client.File.UpdateOne(img).With(file.F.User.Set("a8m"), file.F.User.Clear()).SaveX(ctx)
	require.Nil(t, img.User)
}

func ClearEdges(t *testing.T, client *ent.Client) {
	ctx := context.Background()

	t.Log("clear o2m edges")
	ft := client.FileType.Create().With(filetype.F.Name.Set("photo")).SaveX(ctx)
	client.File.CreateBulk(
		client.File.Create().With(file.F.Name.Set("A"), file.F.Size.Set(10), file.E.Type.SetID(ft.ID)),
		client.File.Create().With(file.F.Name.Set("B"), file.F.Size.Set(20), file.E.Type.SetID(ft.ID)),
	).ExecX(ctx)
	require.NotZero(t, ent.QueryFileTypeFiles(client.FileType, ft).CountX(ctx))
	ft = client.FileType.UpdateOne(ft).With(filetype.E.Files.Clear()).SaveX(ctx)
	require.Zero(t, ent.QueryFileTypeFiles(client.FileType, ft).CountX(ctx))

	t.Log("clear m2m edges")
	a8m := client.User.Create().With(user.F.Name.Set("a8m"), user.F.Age.Set(30)).SaveX(ctx)
	nat := client.User.Create().With(user.F.Name.Set("nati"), user.F.Age.Set(28)).SaveX(ctx)
	inf := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx)
	hub := client.Group.Create().With(group.F.Name.Set("GitHub"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(a8m.ID, nat.ID)).SaveX(ctx)
	lab := client.Group.Create().With(group.F.Name.Set("GitLab"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(a8m.ID, nat.ID)).SaveX(ctx)
	require.Equal(t, 2, ent.QueryUserGroups(client.User, a8m).CountX(ctx))
	client.User.UpdateOne(a8m).With(user.E.Groups.Clear()).ExecX(ctx)
	require.Zero(t, ent.QueryUserGroups(client.User, a8m).CountX(ctx))
	err := client.Group.Update().With(group.E.Users.AddIDs(a8m.ID)).Exec(ctx)
	require.NoError(t, err, "return the user-edge back to groups")
	require.Equal(t, 2, ent.QueryUserGroups(client.User, a8m).CountX(ctx))

	t.Log("clear m2m inverse-edges")
	require.Equal(t, 2, ent.QueryGroupUsers(client.Group, hub).CountX(ctx))
	hub = client.Group.UpdateOne(hub).With(group.E.Users.Clear()).SaveX(ctx)
	require.Zero(t, ent.QueryGroupUsers(client.Group, hub).CountX(ctx))
	require.Equal(t, 2, ent.QueryGroupUsers(client.Group, lab).CountX(ctx))
	client.Group.Update().With(group.E.Users.Clear()).ExecX(ctx)
	require.Zero(t, ent.QueryGroupUsers(client.Group, lab).CountX(ctx))
	require.Zero(t, ent.QueryUserGroups(client.User, a8m).CountX(ctx))
	require.Zero(t, ent.QueryUserGroups(client.User, nat).CountX(ctx))

	t.Log("clear m2m bidi-edges")
	friends := client.User.CreateBulk(
		client.User.Create().With(user.F.Name.Set("f1"), user.F.Age.Set(30), user.E.Friends.AddIDs(a8m.ID, nat.ID)),
		client.User.Create().With(user.F.Name.Set("f2"), user.F.Age.Set(30), user.E.Friends.AddIDs(a8m.ID, nat.ID)),
		client.User.Create().With(user.F.Name.Set("f3"), user.F.Age.Set(30), user.E.Friends.AddIDs(a8m.ID, nat.ID)),
	).SaveX(ctx)
	for i := range friends {
		require.Equal(t, 2, ent.QueryUserFriends(client.User, friends[i]).CountX(ctx))
	}
	require.Equal(t, 3, ent.QueryUserFriends(client.User, a8m).CountX(ctx))
	require.Equal(t, 3, ent.QueryUserFriends(client.User, nat).CountX(ctx))
	nat = client.User.UpdateOne(nat).With(user.E.Friends.Clear()).SaveX(ctx)
	require.Zero(t, ent.QueryUserFriends(client.User, nat).CountX(ctx))
	require.Equal(t, 3, ent.QueryUserFriends(client.User, a8m).CountX(ctx))
	for i := range friends {
		require.Equal(t, 1, ent.QueryUserFriends(client.User, friends[i]).CountX(ctx))
	}
	client.User.Update().With(user.E.Friends.Clear()).ExecX(ctx)
	require.Zero(t, client.User.Query().Where(user.E.Friends.Has()).CountX(ctx))

	t.Log("clear m2m inverse-bidi-edges")
	a8m = client.User.UpdateOne(a8m).With(user.E.Following.AddIDs(friends[0].ID, friends[1].ID, friends[2].ID)).SaveX(ctx)
	require.Equal(t, 3, ent.QueryUserFollowing(client.User, a8m).CountX(ctx))
	require.Zero(t, ent.QueryUserFollowers(client.User, a8m).CountX(ctx))
	nat = client.User.UpdateOne(nat).With(user.E.Followers.AddIDs(friends[0].ID, friends[1].ID, friends[2].ID)).SaveX(ctx)
	require.Zero(t, ent.QueryUserFollowing(client.User, nat).CountX(ctx))
	require.Equal(t, 3, ent.QueryUserFollowers(client.User, nat).CountX(ctx))
	for i := range friends {
		require.Equal(t, 1, ent.QueryUserFollowers(client.User, friends[i]).CountX(ctx))
		require.Equal(t, 1, ent.QueryUserFollowing(client.User, friends[i]).CountX(ctx))
	}
	client.User.UpdateOne(nat).With(user.E.Following.Clear()).ExecX(ctx)
	require.Equal(t, 3, ent.QueryUserFollowers(client.User, nat).CountX(ctx), "expect no effect on followers")
	client.User.UpdateOne(nat).With(user.E.Followers.Clear()).ExecX(ctx)
	require.Zero(t, ent.QueryUserFollowers(client.User, nat).CountX(ctx))
	for i := range friends {
		require.Equal(t, 1, ent.QueryUserFollowers(client.User, friends[i]).CountX(ctx), "expect no effect to followers")
		require.Zero(t, ent.QueryUserFollowing(client.User, friends[i]).CountX(ctx))
	}
	client.User.UpdateOne(a8m).With(user.E.Followers.Clear()).ExecX(ctx)
	require.Equal(t, 3, ent.QueryUserFollowing(client.User, a8m).CountX(ctx), "expect no effect on following")
	client.User.UpdateOne(a8m).With(user.E.Following.Clear()).ExecX(ctx)
	require.Zero(t, ent.QueryUserFollowing(client.User, a8m).CountX(ctx))
	for i := range friends {
		require.Zero(t, ent.QueryUserFollowers(client.User, friends[i]).CountX(ctx))
		require.Zero(t, ent.QueryUserFollowing(client.User, friends[i]).CountX(ctx))
	}

	t.Log("remove/clear and add edges")
	a8m = client.User.UpdateOne(a8m).With(user.E.Following.AddIDs(friends[0].ID, friends[1].ID)).SaveX(ctx)
	require.Equal(t, []int{friends[0].ID, friends[1].ID}, ent.QueryUserFollowing(client.User, a8m).Order(ent.Asc(user.FieldID)).IDsX(ctx))
	a8m = client.User.UpdateOne(a8m).With(user.E.Following.RemoveIDs(friends[0].ID, friends[1].ID), user.E.Following.AddIDs(friends[2].ID)).SaveX(ctx)
	require.Equal(t, friends[2].ID, ent.QueryUserFollowing(client.User, a8m).OnlyIDX(ctx))
	a8m = client.User.UpdateOne(a8m).With(user.E.Following.Clear(), user.E.Following.AddIDs(friends[0].ID)).SaveX(ctx)
	require.Equal(t, friends[0].ID, ent.QueryUserFollowing(client.User, a8m).OnlyIDX(ctx))
}

func UniqueConstraint(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("unique constraint violation on 1 field")
	foo := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("foo"), user.F.Nickname.Set("baz")).SaveX(ctx)
	_, err := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("bar"), user.F.Nickname.Set("baz")).Save(ctx)
	require.True(ent.IsConstraintError(err))
	bar := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("bar"), user.F.Nickname.Set("bar"), user.F.Phone.Set("1")).SaveX(ctx)

	t.Log("unique constraint violation on 2 fields")
	err = client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("baz"), user.F.Nickname.Set("bar"), user.F.Phone.Set("1")).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("baz"), user.F.Nickname.Set("qux"), user.F.Phone.Set("1")).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("baz"), user.F.Nickname.Set("bar"), user.F.Phone.Set("2")).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("baz"), user.F.Nickname.Set("qux"), user.F.Phone.Set("2")).ExecX(ctx)
	err = client.User.UpdateOne(foo).With(user.F.Nickname.Set("bar"), user.F.Phone.Set("1")).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(foo).With(user.F.Nickname.Set("bar"), user.F.Phone.Set("2")).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.CreateBulk(
		client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("foo"), user.F.Nickname.Set("baz")),
		client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("foo"), user.F.Nickname.Set("baz")),
	).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2o unique constraint on creation")
	dan := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("dan"), user.F.Nickname.Set("dan"), user.E.Spouse.SetID(foo.ID)).SaveX(ctx)
	require.Equal(dan.Name, ent.QueryUserSpouse(client.User, foo).OnlyX(ctx).Name)
	err = client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("b"), user.E.Spouse.SetID(foo.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2m/m2o unique constraint on creation")
	c1 := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("c1"), user.F.Nickname.Set("c1"), user.E.Parent.SetID(foo.ID)).SaveX(ctx)
	c2 := client.User.Create().With(user.F.Age.Set(1), user.F.Name.Set("c2"), user.F.Nickname.Set("c2"), user.E.Parent.SetID(foo.ID)).SaveX(ctx)
	err = client.User.Create().With(user.F.Age.Set(10), user.F.Name.Set("z"), user.F.Nickname.Set("z"), user.E.Children.AddIDs(c1.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err), "c1 already has a parent")
	err = client.User.Create().With(user.F.Age.Set(10), user.F.Name.Set("z"), user.F.Nickname.Set("z"), user.E.Children.AddIDs(c2.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err), "c2 already has a parent")
	err = client.User.Create().With(user.F.Age.Set(10), user.F.Name.Set("z"), user.F.Nickname.Set("z"), user.E.Children.AddIDs(c1.ID, c2.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	inf := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx)
	grp := client.Group.Create().With(group.F.Name.Set("Github"), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID)).SaveX(ctx)
	err = client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc"), groupinfo.E.Groups.AddIDs(grp.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	p1 := client.Pet.Create().With(pet.F.Name.Set("p1"), pet.E.Owner.SetID(foo.ID)).SaveX(ctx)
	p2 := client.Pet.Create().With(pet.F.Name.Set("p2"), pet.E.Owner.SetID(foo.ID)).SaveX(ctx)
	err = client.User.Create().With(user.F.Age.Set(10), user.F.Name.Set("new-owner"), user.E.Pets.AddIDs(p1.ID, p2.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	err = client.User.UpdateOne(c2).With(user.F.Nickname.Set(c1.Nickname)).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2o unique constraint on update")
	err = client.User.UpdateOne(bar).With(user.E.Spouse.SetID(foo.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(foo).With(user.E.Spouse.SetID(bar.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	client.User.UpdateOne(bar).With(user.E.Spouse.Clear()).ExecX(ctx)
	client.User.UpdateOne(foo).With(user.E.Spouse.Clear(), user.E.Spouse.SetID(bar.ID)).ExecX(ctx)
	require.False(ent.QueryUserSpouse(client.User, dan).ExistX(ctx))
	require.Equal(bar.Name, ent.QueryUserSpouse(client.User, foo).OnlyX(ctx).Name)
	require.Equal(foo.Name, ent.QueryUserSpouse(client.User, bar).OnlyX(ctx).Name)

	t.Log("o2m unique constraint on update")
	err = client.User.UpdateOne(bar).With(user.F.Age.Set(1), user.F.Name.Set("new-owner"), user.E.Pets.AddIDs(p1.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(bar).With(user.F.Age.Set(1), user.F.Name.Set("new-owner"), user.E.Pets.AddIDs(p1.ID, p2.ID)).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("unique constraint violation when updating more than 1 vertex")
	err = client.User.Update().With(user.F.Nickname.Set("yada")).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	require.False(client.User.Query().Where(user.F.Nickname.EQ("yada")).ExistX(ctx))
	client.User.Update().Where(user.F.Nickname.EQ("dan")).With(user.F.Nickname.Set("yada")).ExecX(ctx)
	require.False(client.User.Query().Where(user.F.Nickname.EQ("dan")).ExistX(ctx))
	require.True(client.User.Query().Where(user.F.Nickname.EQ("yada")).ExistX(ctx))

	t.Log("unique constraint on numeric fields")
	cm1 := client.Comment.Create().With(comment.F.UniqueInt.Set(42), comment.F.UniqueFloat.Set(math.Pi)).SaveX(ctx)
	err = client.Comment.Create().With(comment.F.UniqueInt.Set(42), comment.F.UniqueFloat.Set(math.E)).Exec(ctx)
	require.Error(err)
	err = client.Comment.Create().With(comment.F.UniqueInt.Set(7), comment.F.UniqueFloat.Set(math.Pi)).Exec(ctx)
	require.Error(err)
	client.Comment.Create().With(comment.F.UniqueInt.Set(7), comment.F.UniqueFloat.Set(math.E)).ExecX(ctx)
	err = client.Comment.UpdateOne(cm1).With(comment.F.UniqueInt.Set(7)).Exec(ctx)
	require.Error(err)
	err = client.Comment.UpdateOne(cm1).With(comment.F.UniqueFloat.Set(math.E)).Exec(ctx)
	require.Error(err)

	t.Log("unique constraint on time fields")
	now := time.Now()
	client.File.Create().With(file.F.Name.Set("a"), file.F.Size.Set(10), file.F.CreateTime.Set(now)).ExecX(ctx)
	err = client.File.Create().With(file.F.Name.Set("b"), file.F.Size.Set(20), file.F.CreateTime.Set(now)).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err))
	now = now.Add(time.Second)
	client.File.Create().With(file.F.Name.Set("b"), file.F.Size.Set(20), file.F.CreateTime.Set(now)).ExecX(ctx)
}

type mocker struct{ mock.Mock }

func (m *mocker) onCommit(err error)   { m.Called(err) }
func (m *mocker) onRollback(err error) { m.Called(err) }
func (m *mocker) rHook() ent.RollbackHook {
	return func(next ent.Rollbacker) ent.Rollbacker {
		return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
			err := next.Rollback(ctx, tx)
			m.onRollback(err)
			return err
		})
	}
}

func Tx(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	t.Run("Rollback", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		var m mocker
		m.On("onRollback", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnRollback(m.rHook())
		tx.Node.Create().ExecX(ctx)
		require.NoError(t, tx.Rollback())
		require.Zero(t, client.Node.Query().CountX(ctx), "rollback should discard all changes")
	})
	t.Run("Commit", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		var m mocker
		m.On("onCommit", mock.Anything).Twice()
		defer m.AssertExpectations(t)
		tx.OnCommit(func(next ent.Committer) ent.Committer {
			return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
				err := next.Commit(ctx, tx)
				m.onCommit(err)
				return err
			})
		})
		nde := tx.Node.Create().SaveX(ctx)
		require.NoError(t, tx.Commit())
		require.Error(t, tx.Commit(), "should return an error on the second call")
		require.NotZero(t, client.Node.Query().CountX(ctx), "commit should save all changes")
		// After commit, the committed data should be visible via the client.
		require.Zero(t, ent.QueryNodeNext(client.Node, client.Node.GetX(ctx, nde.ID)).CountX(ctx), "node should have no next after commit")
	})
	t.Run("Nested", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		var m mocker
		m.On("onRollback", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnRollback(m.rHook())
		_, err = tx.Client().Tx(ctx)
		require.Error(t, err, "cannot start a transaction within a transaction")
		require.NoError(t, tx.Rollback())
	})
	t.Run("TxOptions Rollback", func(t *testing.T) {
		skip(t, "SQLite")
		tx, err := client.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		require.NoError(t, err)
		var m mocker
		m.On("onRollback", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
			return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
				err := next.Rollback(ctx, tx)
				m.onRollback(err)
				require.NotNil(t, ctx)
				return err
			})
		})
		err = tx.Item.Create().Exec(ctx)
		require.Error(t, err, "expect creation to fail in read-only tx")
		require.NoError(t, tx.Rollback())
	})
	t.Run("TxOptions Commit", func(t *testing.T) {
		skip(t, "SQLite")
		tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: stdsql.LevelReadCommitted})
		require.NoError(t, err)
		var m mocker
		m.On("onCommit", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnCommit(func(next ent.Committer) ent.Committer {
			return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
				err := next.Commit(ctx, tx)
				m.onCommit(err)
				require.NotNil(t, ctx)
				return err
			})
		})
		err = tx.Item.Create().Exec(ctx)
		require.NoError(t, tx.Commit())
		require.NoError(t, err)
	})
}

func DefaultValue(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	c1 := client.Card.Create().With(card.F.Number.Set("102030"), card.F.Name.Set("Firstname Lastname")).SaveX(ctx)
	ctime, mtime := c1.CreateTime, c1.UpdateTime
	require.False(t, ctime.IsZero())
	require.False(t, mtime.IsZero())
	c1 = client.Card.UpdateOne(c1).With(card.F.Name.Set("F Lastname")).SaveX(ctx)
	require.False(t, c1.CreateTime.IsZero())
	require.False(t, c1.UpdateTime.IsZero())
	require.False(t, mtime.Equal(c1.UpdateTime))

	// Enum default value
	usr := client.User.
		Create().With(user.F.Age.Set(23), user.F.Name.Set("dario")).
		SaveX(ctx)
	require.Equal(t, usr.Role, user.Role("user"))

	b := time.Now().Add(-time.Hour)
	n1 := client.Node.Create().With(node.F.Value.Set(1), node.F.UpdatedAt.Set(b)).SaveX(ctx)
	require.NotNil(t, n1.UpdatedAt)
	require.WithinDuration(t, b, *n1.UpdatedAt, time.Second)
	n1 = client.Node.UpdateOne(n1).With(node.F.Value.Set(2)).SaveX(ctx)
	require.NotNil(t, n1.UpdatedAt)
	require.False(t, b.Equal(*n1.UpdatedAt))
}

// ImmutableValue used to check, via reflection, that Update/UpdateOne
// builders had no Set<F>/SetNillable<F> method for an immutable field
// (created_at, number) and did have one for a mutable field (name) — the
// old per-field codegen enforced immutability structurally, by generating
// Set<F> only on the create builder for an immutable field. Every builder
// now shares the same F.<Field> handle surface regardless of mutability, so
// that structural gate moved into entbuilder.Mutation.SetField et al
// (rejecting a write to an Immutable-flagged field on an Update/UpdateOne
// op) — checked here by asserting the write actually errors.
func ImmutableValue(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	c := client.Card.Create().With(card.F.Number.Set("42")).SaveX(ctx)

	err := client.Card.Update().With(card.F.Number.Set("43")).Exec(ctx)
	require.Error(t, err)
	err = client.Card.UpdateOne(c).With(card.F.Number.Set("43")).Exec(ctx)
	require.Error(t, err)
	err = client.Card.Update().With(card.F.CreateTime.Set(time.Now())).Exec(ctx)
	require.Error(t, err)
	err = client.Card.UpdateOne(c).With(card.F.CreateTime.Set(time.Now())).Exec(ctx)
	require.Error(t, err)

	// A mutable field still works on both builders.
	client.Card.Update().With(card.F.Name.Set("updated")).ExecX(ctx)
	client.Card.UpdateOne(c).With(card.F.Name.Set("updated-one")).ExecX(ctx)
}

// IDFieldImmutableOnUpdate guards against a silent-row-retargeting bug:
// Item.ID is a user-defined field, so F.ID.Set(v) routes through the same
// entbuilder.Mutation.SetField as any other field (old codegen made this
// path structurally unreachable from an update builder — no SetID method
// existed there at all). SetField must still reject it on Update/UpdateOne,
// or an update's WHERE-row target (sqlSave reads it off the mutation) could
// be silently swapped to a different row's ID.
func IDFieldImmutableOnUpdate(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	a := client.Item.Create().With(item.F.ID.Set("a"), item.F.Text.Set("original")).SaveX(ctx)
	client.Item.Create().With(item.F.ID.Set("b")).SaveX(ctx)

	err := client.Item.Update().With(item.F.ID.Set("b")).Exec(ctx)
	require.Error(err)
	err = client.Item.UpdateOne(a).With(item.F.ID.Set("b")).Exec(ctx)
	require.Error(err)

	// Neither attempt moved anything: "a" is untouched, "b" untouched too.
	got := client.Item.GetX(ctx, "a")
	require.Equal("original", got.Text)
	require.True(client.Item.Query().Where(item.F.ID.EQ("b")).ExistX(ctx))
	require.Equal(2, client.Item.Query().CountX(ctx))
}

func Sensitive(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	usr := client.User.Create().With(user.F.Name.Set("foo"), user.F.Age.Set(20), user.F.Password.Set("secret-password")).SaveX(ctx)
	require.Equal("secret-password", usr.Password)
	require.Contains(usr.String(), "password=<sensitive>")
	b, err := json.Marshal(usr)
	require.NoError(err)
	require.NotContains(string(b), "secret-password")
}

func EagerLoading(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	a8m := client.User.Create().With(user.F.Name.Set("a8m"), user.F.Age.Set(30)).SaveX(ctx)
	nati := client.User.Create().With(user.F.Name.Set("nati"), user.F.Age.Set(28), user.E.Spouse.SetID(a8m.ID)).SaveX(ctx)
	alex := client.User.Create().With(user.F.Name.Set("alexsn"), user.F.Age.Set(35), user.E.Friends.AddIDs(a8m.ID)).SaveX(ctx)
	client.Pet.Create().With(pet.F.Name.Set("xabi")).ExecX(ctx)
	client.Pet.Create().With(pet.F.Name.Set("pedro"), pet.E.Owner.SetID(a8m.ID), pet.E.Team.SetID(nati.ID)).ExecX(ctx)
	client.Card.Create().With(card.F.Number.Set("102030"), card.E.Owner.SetID(a8m.ID)).ExecX(ctx)

	inf := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx)
	files := ent.Files{
		client.File.Create().With(file.F.Name.Set("a"), file.F.Size.Set(10)).SaveX(ctx),
		client.File.Create().With(file.F.Name.Set("b"), file.F.Size.Set(10)).SaveX(ctx),
		client.File.Create().With(file.F.Name.Set("c"), file.F.Size.Set(10)).SaveX(ctx),
	}
	typ := client.FileType.Create().With(filetype.F.Name.Set("type"), filetype.E.Files.AddIDs(fileIDs(files)...)).SaveX(ctx)
	hub := client.Group.Create().With(group.F.Name.Set("GitHub"), group.F.Expire.Set(time.Now()), group.E.Users.AddIDs(alex.ID, a8m.ID), group.E.Info.SetID(inf.ID)).SaveX(ctx)
	lab := client.Group.Create().With(group.F.Name.Set("GitLab"), group.F.Expire.Set(time.Now()), group.E.Users.AddIDs(nati.ID, a8m.ID), group.E.Info.SetID(inf.ID), group.E.Files.AddIDs(fileIDs(files)...)).SaveX(ctx)

	t.Run("O2O", func(t *testing.T) {
		users := ent.WithUserParent(ent.WithUserCard(ent.WithUserSpouse(client.User.
			Query().
			Where(user.E.Spouse.Has())))).
			Order(ent.Asc(user.FieldName)).
			AllX(ctx)
		require.Len(users, 2)
		require.NotNil(users[0].Edges.Spouse)
		require.NotNil(users[1].Edges.Spouse)
		require.NotNil(nati.Name, users[0].Edges.Spouse.Name)
		require.NotNil(a8m.Name, users[1].Edges.Spouse.Name)
		require.NotNil(users[0].Edges.Card)
		require.Nil(users[1].Edges.Card)

		edges := users[0].Edges
		pets, err := edges.PetsOrErr()
		require.True(ent.IsNotLoaded(err))
		require.Nil(pets)
		groups, err := edges.GroupsOrErr()
		require.True(ent.IsNotLoaded(err))
		require.Nil(groups)
		card, err := edges.CardOrErr()
		require.Nil(err)
		require.NotNil(card)
		spouse, err := edges.SpouseOrErr()
		require.Nil(err)
		require.NotNil(spouse)
		parent, err := edges.ParentOrErr()
		require.True(ent.IsNotFound(err), "loaded but was not found")
		require.Nil(parent)
	})

	t.Run("O2M", func(t *testing.T) {
		pets := client.Pet.Query().AllX(ctx)
		require.Nil(pets[0].Edges.Team)
		require.Nil(pets[0].Edges.Owner)
		require.Nil(pets[1].Edges.Team)
		require.Nil(pets[1].Edges.Owner)

		pedro := ent.WithPetOwner(client.Pet.Query().Where(pet.E.Owner.Has())).OnlyX(ctx)
		require.Nil(pedro.Edges.Team)
		require.NotNil(pedro.Edges.Owner)
		require.Equal(a8m.Name, pedro.Edges.Owner.Name)

		pedro = ent.WithPetTeam(ent.WithPetOwner(client.Pet.Query().Where(pet.E.Owner.Has()))).OnlyX(ctx)
		require.NotNil(pedro.Edges.Team)
		require.NotNil(pedro.Edges.Owner)
		require.Equal(a8m.Name, pedro.Edges.Owner.Name)
		require.Equal(nati.Name, pedro.Edges.Team.Name)
	})

	t.Run("M2O", func(t *testing.T) {
		a8m := client.User.Query().Where(user.F.ID.EQ(a8m.ID)).OnlyX(ctx)
		require.Empty(a8m.Edges.Pets)

		a8m = ent.WithUserPets(client.User.
			Query().
			Where(user.F.ID.EQ(a8m.ID)), func(q *ent.PetQuery) {
			ent.WithPetTeam(q).Order(ent.Asc(pet.FieldName))
		}).
			OnlyX(ctx)
		require.Len(a8m.Edges.Pets, 1)
		require.Equal("pedro", a8m.Edges.Pets[0].Name)
		require.Equal(nati.Name, a8m.Edges.Pets[0].Edges.Team.Name)
		for _, p := range a8m.Edges.Pets {
			require.Equal(a8m, p.Edges.Owner)
			u, err := p.Edges.OwnerOrErr()
			require.NoError(err)
			require.Equal(a8m, u)
		}

		a8m = ent.WithUserPets(client.User.
			Query().
			Where(user.F.ID.EQ(a8m.ID)), func(q *ent.PetQuery) {
			q.Where(pet.F.Name.EQ("unknown"))
		}).
			OnlyX(ctx)
		require.Empty(a8m.Edges.Pets)
		require.NotNil(a8m.Edges.Pets)
	})

	t.Run("M2M", func(t *testing.T) {
		users := ent.WithUserGroups(ent.WithUserFriends(client.User.
			Query()), func(q *ent.GroupQuery) {
			q.Order(ent.Desc(group.FieldName))
		}).
			Order(ent.Asc(user.FieldName)).
			AllX(ctx)
		require.Equal(a8m.Name, users[0].Name)
		require.Len(users[0].Edges.Groups, 2)
		require.Len(users[0].Edges.Friends, 1)
		require.Equal(alex.Name, users[0].Edges.Friends[0].Name)
		g1, g2 := users[0].Edges.Groups[0], users[0].Edges.Groups[1]
		require.Equal(lab.Name, g1.Name)
		require.Equal(hub.Name, g2.Name)

		groups := ent.WithGroupUsers(client.Group.
			Query(), func(q *ent.UserQuery) {
			q.Order(ent.Asc(user.FieldName))
		}).
			Order(ent.Asc(group.FieldName)).
			AllX(ctx)
		require.Len(groups, 2)
		g1, g2 = groups[0], groups[1]
		require.Equal(hub.Name, g1.Name)
		require.Equal(lab.Name, g2.Name)
		require.Equal(a8m.Name, g1.Edges.Users[0].Name)
		require.Equal(alex.Name, g1.Edges.Users[1].Name)
		require.Equal(a8m.Name, g2.Edges.Users[0].Name)
		require.Equal(nati.Name, g2.Edges.Users[1].Name)
		require.Equal(g1.Edges.Users[0], g2.Edges.Users[0], "should share the same object")
	})

	t.Run("Graph", func(t *testing.T) {
		users := ent.WithUserGroups(ent.WithUserFriends(ent.WithUserSpouse(client.User.
			Query())), func(q *ent.GroupQuery) {
			ent.WithGroupInfo(q)
			ent.WithGroupFiles(q, func(q *ent.FileQuery) {
				ent.WithFileType(q)
				q.Order(ent.Asc(file.FieldName))
			})
			q.Order(ent.Desc(group.FieldName))
		}).
			Order(ent.Asc(user.FieldName)).
			AllX(ctx)
		require.Equal(a8m.Name, users[0].Name)
		require.NotNil(users[0].Edges.Spouse)
		require.Equal(nati.Name, users[0].Edges.Spouse.Name)
		require.Len(users[0].Edges.Groups, 2)
		require.Len(users[0].Edges.Friends, 1)
		require.Equal(alex.Name, users[0].Edges.Friends[0].Name)

		require.Equal(alex.Name, users[1].Name)
		require.Len(users[1].Edges.Groups, 1)
		require.Equal(hub.Name, users[1].Edges.Groups[0].Name)

		require.Equal(nati.Name, users[2].Name)
		require.Len(users[2].Edges.Groups, 1)
		require.Equal(lab.Name, users[2].Edges.Groups[0].Name)

		g1, g2 := users[0].Edges.Groups[0], users[0].Edges.Groups[1]
		require.Equal(lab.Name, g1.Name)
		require.Equal(hub.Name, g2.Name)
		require.Equal(inf.Desc, g1.Edges.Info.Desc)
		require.Equal([]string{"a", "c"}, []string{g1.Edges.Files[0].Name, g1.Edges.Files[2].Name})
		for _, f := range g1.Edges.Files {
			require.NotNil(f.Edges.Type)
			require.Equal(typ.Name, f.Edges.Type.Name)
		}
	})

	t.Run("LimitRows/O2M", func(t *testing.T) {
		skip(t, "MySQL/5")
		client.Pet.Delete().ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("nala"), pet.E.Owner.SetID(nati.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("xabi3"), pet.E.Owner.SetID(a8m.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("xabi2"), pet.E.Owner.SetID(a8m.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("xabi1"), pet.E.Owner.SetID(a8m.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("lola4"), pet.E.Owner.SetID(alex.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("lola3"), pet.E.Owner.SetID(alex.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("lola2"), pet.E.Owner.SetID(alex.ID)).ExecX(ctx)
		client.Pet.Create().With(pet.F.Name.Set("lola1"), pet.E.Owner.SetID(alex.ID)).ExecX(ctx)

		users := ent.WithUserPets(client.User.Query()).Order(ent.Asc(user.FieldID)).AllX(ctx)
		require.Len(users[0].Edges.Pets, 3)
		require.Len(users[1].Edges.Pets, 1)
		require.Len(users[2].Edges.Pets, 4)

		users = ent.WithUserPets(client.User.
			Query(), func(q *ent.PetQuery) {
			q.Modify(limitRows(pet.OwnerColumn, 2))
		}).
			Order(ent.Asc(user.FieldID)).
			AllX(ctx)
		require.Len(users[0].Edges.Pets, 2)
		require.Equal(users[0].Edges.Pets[0].Name, "xabi3")
		require.Equal(users[0].Edges.Pets[1].Name, "xabi2")
		require.Len(users[1].Edges.Pets, 1)
		require.Equal(users[1].Edges.Pets[0].Name, "nala")
		require.Len(users[2].Edges.Pets, 2)
		require.Equal(users[2].Edges.Pets[0].Name, "lola4")
		require.Equal(users[2].Edges.Pets[1].Name, "lola3")

		users = ent.WithUserPets(client.User.
			Query(), func(q *ent.PetQuery) {
			q.Modify(limitRows(pet.OwnerColumn, 1, pet.FieldName))
		}).
			Order(ent.Asc(user.FieldID)).
			AllX(ctx)
		require.Len(users[0].Edges.Pets, 1)
		require.Equal(users[0].Edges.Pets[0].Name, "xabi1")
		require.Len(users[1].Edges.Pets, 1)
		require.Equal(users[1].Edges.Pets[0].Name, "nala")
		require.Len(users[2].Edges.Pets, 1)
		require.Equal(users[2].Edges.Pets[0].Name, "lola1")
	})

	t.Run("LimitRows/M2M", func(t *testing.T) {
		skip(t, "MySQL/5")
		users := ent.WithUserGroups(client.User.Query()).Order(ent.Asc(user.FieldID)).AllX(ctx)
		require.Len(users[0].Edges.Groups, 2)
		require.Len(users[1].Edges.Groups, 1)
		require.Len(users[2].Edges.Groups, 1)

		users = ent.WithUserGroups(client.User.
			Query(), func(q *ent.GroupQuery) {
			q.Modify(limitRows(user.GroupsPrimaryKey[0], 1))
		}).
			Order(ent.Asc(user.FieldID)).
			AllX(ctx)
		require.Len(users[0].Edges.Groups, 1)
		require.Equal(users[0].Edges.Groups[0].Name, "GitHub")
		require.Len(users[1].Edges.Groups, 1)
		require.Equal(users[1].Edges.Groups[0].Name, "GitLab")
		require.Len(users[2].Edges.Groups, 1)
		require.Equal(users[2].Edges.Groups[0].Name, "GitHub")

		client.Group.Create().With(group.F.Name.Set("BitBucket"), group.F.Expire.Set(time.Now()), group.E.Users.AddIDs(alex.ID, a8m.ID), group.E.Info.SetID(inf.ID)).SaveX(ctx)
		users = ent.WithUserGroups(client.User.
			Query(), func(q *ent.GroupQuery) {
			q.Modify(limitRows(user.GroupsPrimaryKey[0], 1, group.FieldName))
		}).
			Order(ent.Asc(user.FieldID)).
			AllX(ctx)
		require.Len(users[0].Edges.Groups, 1)
		require.Equal(users[0].Edges.Groups[0].Name, "BitBucket")
		require.Len(users[1].Edges.Groups, 1)
		require.Equal(users[1].Edges.Groups[0].Name, "GitLab")
		require.Len(users[2].Edges.Groups, 1)
		require.Equal(users[2].Edges.Groups[0].Name, "BitBucket")
	})
}

func limitRows(partitionBy string, limit int, orderBy ...string) func(s *sql.Selector) {
	return func(s *sql.Selector) {
		d := sql.Dialect(s.Dialect())
		s.SetDistinct(false)
		if len(orderBy) == 0 {
			orderBy = append(orderBy, "id")
		}
		with := d.With("src_query").
			As(s.Clone()).
			With("limited_query").
			As(
				d.Select("*").
					AppendSelectExprAs(
						sql.RowNumber().PartitionBy(partitionBy).OrderBy(orderBy...),
						"row_number",
					).
					From(d.Table("src_query")),
			)
		t := d.Table("limited_query").As(s.TableName())
		*s = *d.Select(s.UnqualifiedColumns()...).
			From(t).
			Where(sql.LTE(t.C("row_number"), limit)).
			Prefix(with)
	}
}

func NamedEagerLoading(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	a8m := client.User.Create().With(user.F.Name.Set("a8m"), user.F.Age.Set(30)).SaveX(ctx)
	p1 := client.Pet.Create().With(pet.F.Name.Set("pet1"), pet.E.Owner.SetID(a8m.ID), pet.F.Trained.Set(true)).SaveX(ctx)
	p2 := client.Pet.Create().With(pet.F.Name.Set("pet2"), pet.E.Owner.SetID(a8m.ID), pet.F.Trained.Set(false)).SaveX(ctx)

	a8m = ent.WithNamedUserPets(ent.WithNamedUserPets(client.User.Query(),
		"Trained", func(q *ent.PetQuery) { q.Where(pet.F.Trained.EQ(true)) }),
		"Untrained", func(q *ent.PetQuery) { q.Where(pet.F.Trained.EQ(false)) }).
		OnlyX(ctx)
	trained, err := a8m.NamedPets("Trained")
	require.NoError(t, err)
	require.Len(t, trained, 1)
	require.Equal(t, p1.ID, trained[0].ID)
	untrained, err := a8m.NamedPets("Untrained")
	require.NoError(t, err)
	require.Len(t, untrained, 1)
	require.Equal(t, p2.ID, untrained[0].ID)
	unknown, err := a8m.NamedPets("Unknown")
	require.True(t, ent.IsNotLoaded(err))
	require.Nil(t, unknown)

	exists := ent.WithNamedUserPets(client.User.Query(),
		"WithSelection", func(q *ent.PetQuery) {
			q.Select(pet.FieldID)
		}).
		ExistX(ctx)
	require.True(t, exists)
}

// writerFunc is an io.Writer implemented by the underlying func.
type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func userIDs(users ent.Users) []int {
	ids := make([]int, len(users))
	for i := range users {
		ids[i] = users[i].ID
	}
	return ids
}

func groupIDs(groups ent.Groups) []int {
	ids := make([]int, len(groups))
	for i := range groups {
		ids[i] = groups[i].ID
	}
	return ids
}

func fileIDs(files ent.Files) []int {
	ids := make([]int, len(files))
	for i := range files {
		ids[i] = files[i].ID
	}
	return ids
}

func NoSchemaChanges(t *testing.T, client *ent.Client) {
	w := writerFunc(func(p []byte) (int, error) {
		stmt := strings.Trim(string(p), "\n;")
		ok := []*regexp.Regexp{
			regexp.MustCompile("^BEGIN$"),
			regexp.MustCompile("^COMMIT$"),
		}
		switch {
		case strings.Contains(t.Name(), "SQLite"):
			ok = append(ok, regexp.MustCompile("^PRAGMA foreign_keys = (off|on)$"))
		case strings.Contains(t.Name(), "MySQL"), strings.Contains(t.Name(), "Maria"):
			ok = append(ok, regexp.MustCompile("^ALTER TABLE `\\w+` AUTO_INCREMENT \\d+$"))
		}
		if !slices.ContainsFunc(ok, func(re *regexp.Regexp) bool {
			return re.MatchString(stmt)
		}) {
			// MySQL 5.6 + 5.7, and MariaDB 10.x store auto-increment counter in memory. In cases the server is
			// restarted, and there are no rows, the counter is reset. Atlas "fixes" this by setting the auto-increment
			// value in those cases. Therefore, statements following the pattern
			// "ALTER TABLE `<table>` AUTO_INCREMENT = <value>" are allowed.
			t.Errorf("expect no statement to execute. got: %q", stmt)
		}
		return len(p), nil
	})
	tables, err := sqlschema.CopyTables(migrate.Tables)
	require.NoError(t, err)
	err = migrate.Create(
		context.Background(),
		migrate.NewSchema(&sqlschema.WriteDriver{Writer: w, Driver: client.Driver()}),
		tables,
		migrate.WithDropIndex(true),
		migrate.WithDropColumn(true),
	)
	require.NoError(t, err)
}

func Mutation(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	setName := func(ns interface {
		SetField(string, ent.Value) error
	}, name string) {
		_ = ns.SetField("name", name)
	}
	ub := client.User.Create().With(user.F.Age.Set(30))
	setName(ub.Mutation(), "a8m")
	pb := client.Pet.Create()
	setName(pb.Mutation(), "pedro")

	a8m := ub.SaveX(ctx)
	require.Equal(t, "a8m", a8m.Name)
	pedro := pb.SaveX(ctx)
	require.Equal(t, "pedro", pedro.Name)

	setUsers := func(ms ...*ent.UserMutation) {
		for _, m := range ms {
			_ = m.SetField(user.FieldName, "boring")
			_ = m.SetField(user.FieldAge, 30)
		}
	}
	uu := client.User.UpdateOne(a8m).With(user.E.Pets.AddIDs(pedro.ID))
	ub = client.User.Create()
	setUsers(ub.Mutation(), uu.Mutation())
	a8m = uu.SaveX(ctx)
	usr := ub.SaveX(ctx)
	require.Equal(t, "boring", a8m.Name)
	require.Equal(t, "boring", usr.Name)

	require.Equal(t, []int{usr.ID}, entbuilder.EdgeIDsAs[int](client.User.UpdateOne(a8m).With(user.E.Friends.AddIDs(usr.ID)).Mutation(), user.EdgeFriends))
	require.Empty(t, entbuilder.EdgeIDsAs[int](client.User.UpdateOne(a8m).With(user.E.Friends.AddIDs(usr.ID), user.E.Friends.RemoveIDs(usr.ID)).Mutation(), user.EdgeFriends))
	require.Equal(t, []int{usr.ID}, entbuilder.EdgeIDsAs[int](client.User.UpdateOne(a8m).With(user.E.Friends.AddIDs(usr.ID), user.E.Friends.RemoveIDs(a8m.ID)).Mutation(), user.EdgeFriends))
	client.User.UpdateOne(a8m).With(user.E.Friends.AddIDs(usr.ID)).ExecX(ctx)

	t.Run("IDs", func(t *testing.T) {
		ids := client.User.Query().IDsX(ctx)
		u := client.User.Update().Where(user.F.ID.In(ids...)).With(user.F.Age.Add(1))
		mids, err := u.Mutation().IDs(ctx)
		require.NoError(t, err)
		// Order can change between the 2 queries.
		sort.Ints(ids)
		sort.Ints(mids)
		require.Equal(t, ids, mids)
		u.ExecX(ctx)

		u = client.User.
			Update().With(user.F.Age.Add(1)).
			Where(user.F.Name.EQ(a8m.Name), user.E.Pets.Has(), user.E.Pets.HasWith(pet.F.Name.EQ(pedro.Name)))
		mids, err = u.Mutation().IDs(ctx)
		require.NoError(t, err)
		require.Len(t, mids, 1)
		require.Equal(t, a8m.ID, mids[0])
		u.ExecX(ctx)
	})

	t.Run("Predicate", func(t *testing.T) {
		updater := client.User.UpdateOne(a8m)
		updater.Mutation().WhereP(user.F.Name.EQ(a8m.Name))
		updater.With(user.F.Name.Set("mashraki"))
		a8m, err := updater.Save(ctx)
		require.NoError(t, err, "predicate should not affect the returned object")
		require.Equal(t, "mashraki", a8m.Name)

		updater = client.User.UpdateOne(a8m)
		updater.Mutation().WhereP(user.F.Name.EQ(a8m.Name + a8m.Name))
		updater.With(user.F.Name.Set("a8m"))
		a8m, err = updater.Save(ctx)
		require.True(t, ent.IsNotFound(err))
		require.Nil(t, a8m)
	})
}

// Test templates codegen.
var (
	_ = ent.CardExtension{}
	_ = ent.Card{}.StaticField
	_ = ent.Client{}.TemplateField
	_ = []filetype.State{filetype.StateOn, filetype.StateOff}
	_ = []filetype.Type{filetype.TypeJPG, filetype.TypePNG, filetype.TypeSVG}
)

func CreateBulk(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	cards := client.Card.CreateBulk(
		client.Card.Create().With(card.F.Number.Set("10"), card.F.Name.Set("1st")),
		client.Card.Create().With(card.F.Number.Set("20"), card.F.Name.Set("2nd")),
		client.Card.Create().With(card.F.Number.Set("30"), card.F.Name.Set("3rd")),
	).SaveX(ctx)
	require.Equal(t, cards[0].ID, cards[1].ID-1)
	require.Equal(t, cards[1].ID, cards[2].ID-1)

	inf := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("group info")).SaveX(ctx)
	names := []string{"GitHub", "GitLab"}
	groups := client.Group.MapCreateBulk(names, func(c *group.GroupCreate, i int) {
		c.With(group.F.Name.Set(names[i]), group.F.Expire.Set(time.Now()), group.E.Info.SetID(inf.ID))
	}).SaveX(ctx)
	require.Equal(t, inf.ID, ent.QueryGroupInfo(client.Group, groups[0]).OnlyIDX(ctx))
	require.Equal(t, inf.ID, ent.QueryGroupInfo(client.Group, groups[1]).OnlyIDX(ctx))

	_, err := client.Group.MapCreateBulk(1, nil).Save(ctx)
	require.Error(t, err)

	// OnConflict on a bulk builder created from an error (e.g. wrong-type
	// MapCreateBulk input) must not panic on the nil driver when building
	// the upsert config; the original error should still surface.
	err = client.User.MapCreateBulk(1, nil).OnConflict().Exec(ctx)
	require.Error(t, err)

	client.User.Use(
		func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
				_ = m.SetField(user.FieldPassword, "password")
				return next.Mutate(ctx, m)
			})
		},
		func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
				name, _ := entbuilder.GetField[string](m, user.FieldName)
				_ = m.SetField(user.FieldNickname, "@"+name)
				return next.Mutate(ctx, m)
			})
		},
	)

	users := client.User.CreateBulk(
		client.User.Create().With(user.F.Name.Set("a8m"), user.F.Age.Set(20), user.E.Groups.AddIDs(groupIDs(groups)...)),
		client.User.Create().With(user.F.Name.Set("nati"), user.F.Age.Set(20), user.E.Card.SetID(cards[0].ID), user.E.Groups.AddIDs(groups[0].ID)),
	).SaveX(ctx)
	require.Equal(t, 2, ent.QueryUserGroups(client.User, users[0]).CountX(ctx))
	require.False(t, ent.QueryUserCard(client.User, users[0]).ExistX(ctx))
	require.Equal(t, "password", users[0].Password)
	require.Equal(t, "@a8m", users[0].Nickname)
	require.Equal(t, groups[0].ID, ent.QueryUserGroups(client.User, users[1]).OnlyIDX(ctx))
	require.Equal(t, cards[0].ID, ent.QueryUserCard(client.User, users[1]).OnlyIDX(ctx))
	require.Equal(t, "password", users[1].Password)
	require.Equal(t, "@nati", users[1].Nickname)

	pets := client.Pet.CreateBulk(
		client.Pet.Create().With(pet.F.Name.Set("pedro"), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("xabi"), pet.E.Owner.SetID(users[1].ID)),
		client.Pet.Create().With(pet.F.Name.Set("layla")),
	).SaveX(ctx)
	require.Equal(t, "pedro", pets[0].Name)
	require.Equal(t, users[0].ID, ent.QueryPetOwner(client.Pet, pets[0]).OnlyIDX(ctx))
	require.Equal(t, "xabi", pets[1].Name)
	require.Equal(t, users[1].ID, ent.QueryPetOwner(client.Pet, pets[1]).OnlyIDX(ctx))
	require.Equal(t, "layla", pets[2].Name)
	require.False(t, ent.QueryPetOwner(client.Pet, pets[2]).ExistX(ctx))
}

func ConstraintChecks(t *testing.T, client *ent.Client) {
	var cerr *ent.ConstraintError
	err := client.Pet.Create().With(pet.F.Name.Set("orphan"), pet.E.Owner.SetID(0)).Exec(context.Background())
	require.True(t, errors.As(err, &cerr))
	require.True(t, sqlgraph.IsForeignKeyConstraintError(err))
	require.False(t, sqlgraph.IsUniqueConstraintError(err))

	client.FileType.Create().With(filetype.F.Name.Set("a unique name")).SaveX(context.Background())
	err = client.FileType.Create().With(filetype.F.Name.Set("a unique name")).Exec(context.Background())
	require.True(t, errors.As(err, &cerr))
	require.False(t, sqlgraph.IsForeignKeyConstraintError(err))
	require.True(t, sqlgraph.IsUniqueConstraintError(err))
}

func Lock(t *testing.T, client *ent.Client) {
	skip(t, "SQLite", "MySQL/5", "Maria/10.2")
	ctx := context.Background()
	xabi := client.Pet.Create().With(pet.F.Name.Set("Xabi")).SaveX(ctx)

	t.Run("ForUpdate", func(t *testing.T) {
		tx1, err := client.Tx(ctx)
		require.NoError(t, err)
		tx2, err := client.Tx(ctx)
		require.NoError(t, err)
		tx3, err := client.Tx(ctx)
		require.NoError(t, err)
		p1 := tx1.Pet.Query().Where(pet.F.ID.EQ(xabi.ID)).ForUpdate().OnlyX(ctx)
		_, err = tx2.Pet.Query().Where(pet.F.ID.EQ(xabi.ID)).ForUpdate(sql.WithLockAction(sql.NoWait)).Only(ctx)
		switch name := t.Name(); {
		case strings.Contains(name, "Postgres"):
			err := err.(*pq.Error)
			require.EqualValues(t, "55P03", err.Code)
			require.EqualValues(t, `could not obtain lock on row in relation "pet"`, err.Message)
		case strings.Contains(name, "MySQL"):
			err := err.(*mysql.MySQLError)
			require.EqualValues(t, 3572, err.Number)
			require.EqualValues(t, "Statement aborted because lock(s) could not be acquired immediately and NOWAIT is set.", err.Message)
		case strings.Contains(name, "Maria"):
			err := err.(*mysql.MySQLError)
			require.EqualValues(t, 1205, err.Number)
			require.EqualValues(t, "Lock wait timeout exceeded; try restarting transaction", err.Message)
		}
		require.NoError(t, tx2.Rollback())
		tx1.Pet.UpdateOne(p1).With(pet.F.Name.Set("updated")).ExecX(ctx)
		require.NoError(t, tx1.Commit())
		tx3.Pet.Query().Where(pet.F.ID.EQ(xabi.ID)).ForUpdate().OnlyX(ctx)
		require.NoError(t, tx3.Rollback())
	})

	t.Run("ForShare", func(t *testing.T) {
		skip(t, "Maria")
		tx1, err := client.Tx(ctx)
		require.NoError(t, err)
		tx2, err := client.Tx(ctx)
		require.NoError(t, err)
		tx3, err := client.Tx(ctx)
		require.NoError(t, err)
		tx1.Pet.Query().Where(pet.F.ID.EQ(xabi.ID)).ForShare().OnlyX(ctx)
		tx2.Pet.Query().Where(pet.F.ID.EQ(xabi.ID)).ForShare().OnlyX(ctx)
		_, err = tx3.Pet.Query().
			Where(pet.F.ID.EQ(xabi.ID)).
			ForUpdate(
				sql.WithLockTables(pet.Table),
				sql.WithLockAction(sql.NoWait),
			).
			Only(ctx)
		require.Error(t, err)
		require.NoError(t, tx1.Rollback())
		require.NoError(t, tx2.Rollback())
		require.NoError(t, tx3.Rollback())
	})
}

func ExtValueScan(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	u, err := url.Parse("https://entgo.io")
	require.NoError(t, err)
	check := func(ex *ent.ExValueScan, i *big.Int, u, b64, custom string, ub *url.URL) {
		for _, e := range []*ent.ExValueScan{ex, client.ExValueScan.GetX(ctx, ex.ID)} {
			require.Equal(t, i, e.Text)
			require.Equal(t, u, e.Binary.String())
			require.Equal(t, ub, e.BinaryBytes)
			require.Equal(t, b64, e.Base64)
			require.Equal(t, custom, e.Custom)
		}
	}
	ex := client.ExValueScan.Create().With(exvaluescan.F.Text.Set(big.NewInt(10)), exvaluescan.F.Binary.Set(u), exvaluescan.F.BinaryBytes.Set(u), exvaluescan.F.Base64.Set("a8m"), exvaluescan.F.Custom.Set("atlasgo.io")).
		SaveX(ctx)
	check(ex, big.NewInt(10), u.String(), "a8m", "atlasgo.io", u)

	// Ensure the database values store as expected.
	var raw []struct {
		Text        string
		Binary      string
		BinaryBytes []byte `sql:"binary_bytes"`
		Base64      string
		Custom      string
	}
	client.ExValueScan.Query().
		Select(
			exvaluescan.FieldText,
			exvaluescan.FieldBinary,
			exvaluescan.FieldBinaryBytes,
			exvaluescan.FieldBase64,
			exvaluescan.FieldCustom,
		).
		ScanX(ctx, &raw)
	require.Len(t, raw, 1)
	require.Equal(t, "10", raw[0].Text)
	require.Equal(t, u.String(), raw[0].Binary)
	ub, err := u.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, ub, raw[0].BinaryBytes)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte(ex.Base64)), raw[0].Base64)
	require.Equal(t, "0x:"+hex.EncodeToString([]byte(ex.Custom)), raw[0].Custom)

	// Update the values and ensure they are updated as expected.
	u.Path = "/docs"
	ex = client.ExValueScan.UpdateOne(ex).With(exvaluescan.F.Binary.Set(u), exvaluescan.F.BinaryBytes.Set(u), exvaluescan.F.Text.Set(big.NewInt(20)), exvaluescan.F.Base64.Set("m8a"), exvaluescan.F.Custom.Set("entgo.io")).SaveX(ctx)
	check(ex, big.NewInt(20), u.String(), "m8a", "entgo.io", u)

	// Check predicates.
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.F.Text.EQ(big.NewInt(20))).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.F.Text.EQ(big.NewInt(10))).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.F.Text.LTE(big.NewInt(20))).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.F.Text.LTE(big.NewInt(10))).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.F.Binary.EQ(u)).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.F.Binary.EQ(&url.URL{})).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.F.Base64.In("m8a")).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.F.Base64.In("a8m")).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.F.Custom.HasPrefix("ent")).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.F.Custom.HasPrefix("atlas")).ExistX(ctx))
	// HasSuffix cannot work with this field as the value is stored as hex (with additional prefix).
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.F.Custom.HasSuffix("io")).ExistX(ctx))
}

func OrderByFluent(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	users := client.User.CreateBulk(
		client.User.Create().With(user.F.Name.Set("a"), user.F.Age.Set(1)),
		client.User.Create().With(user.F.Name.Set("b"), user.F.Age.Set(2)),
		client.User.Create().With(user.F.Name.Set("c"), user.F.Age.Set(3)),
		client.User.Create().With(user.F.Name.Set("d"), user.F.Age.Set(4)),
		client.User.Create().With(user.F.Name.Set("e"), user.F.Age.Set(5)),
	).SaveX(ctx)
	pets := client.Pet.CreateBulk(
		client.Pet.Create().With(pet.F.Name.Set("aa"), pet.E.Owner.SetID(users[1].ID), pet.F.Age.Set(2)),
		client.Pet.Create().With(pet.F.Name.Set("ab"), pet.E.Owner.SetID(users[1].ID), pet.F.Age.Set(2)),
		client.Pet.Create().With(pet.F.Name.Set("ac"), pet.E.Owner.SetID(users[0].ID), pet.F.Age.Set(1)),
		client.Pet.Create().With(pet.F.Name.Set("ba"), pet.E.Owner.SetID(users[0].ID), pet.F.Age.Set(1)),
		client.Pet.Create().With(pet.F.Name.Set("bb"), pet.E.Owner.SetID(users[0].ID), pet.F.Age.Set(1)),
		client.Pet.Create().With(pet.F.Name.Set("ca"), pet.E.Owner.SetID(users[2].ID), pet.F.Age.Set(10)),
		client.Pet.Create().With(pet.F.Name.Set("d")),
		client.Pet.Create().With(pet.F.Name.Set("e")),
	).SaveX(ctx)

	t.Run("M2O", func(t *testing.T) {
		ids := client.Pet.Query().
			Order(
				pet.E.Owner.OrderBy(sql.OrderByField(user.FieldName)), pet.F.ID.Order(),
			).
			IDsX(ctx)
		require.Equal(t, []int{pets[6].ID, pets[7].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[0].ID, pets[1].ID, pets[5].ID}, ids)

		ids = client.Pet.Query().
			Order(
				pet.E.Owner.OrderBy(sql.OrderByField(user.FieldName, sql.OrderDesc())), pet.F.ID.Order(sql.OrderDesc()),
			).
			IDsX(ctx)
		require.Equal(t, []int{pets[5].ID, pets[1].ID, pets[0].ID, pets[4].ID, pets[3].ID, pets[2].ID, pets[7].ID, pets[6].ID}, ids)
	})

	t.Run("M2O/SelectedOwner", func(t *testing.T) {
		const as = "owner_name"
		query := client.Pet.Query().
			Order(
				pet.E.Owner.OrderBy(sql.OrderByField(
					user.FieldName,
					sql.OrderSelectAs(as),
				)),
			)
		// No owner.
		for _, u := range query.Clone().Limit(2).AllX(ctx) {
			name, err := u.Value(as)
			require.NoError(t, err)
			require.Nil(t, name)
		}
		// User "a".
		for _, u := range query.Clone().Offset(2).Limit(3).AllX(ctx) {
			name, err := u.Value(as)
			require.NoError(t, err)
			require.EqualValues(t, users[0].Name, name)
		}
		// User "b".
		for _, u := range query.Clone().Offset(5).Limit(2).AllX(ctx) {
			name, err := u.Value(as)
			require.NoError(t, err)
			require.EqualValues(t, users[1].Name, name)
		}
		// User "c".
		name, err := query.Clone().Offset(7).OnlyX(ctx).Value(as)
		require.NoError(t, err)
		require.EqualValues(t, users[2].Name, name)
	})

	t.Run("O2M/Count", func(t *testing.T) {
		ids := client.User.Query().
			Order(user.E.Pets.OrderByCount(), user.F.ID.Order(sql.OrderDesc())).
			IDsX(ctx)
		require.Equal(t, []int{users[4].ID, users[3].ID, users[2].ID, users[1].ID, users[0].ID}, ids)

		ids = client.User.Query().
			Order(user.E.Pets.OrderByCount(sql.OrderDesc()), user.F.ID.Order(sql.OrderDesc())).
			IDsX(ctx)
		require.Equal(t, []int{users[0].ID, users[1].ID, users[2].ID, users[4].ID, users[3].ID}, ids)
	})

	t.Run("O2M/SelectedCount", func(t *testing.T) {
		const as = "pets_count"
		ordered := client.User.Query().
			Order(user.E.Pets.OrderByCount(
				sql.OrderSelectAs(as),
			), user.F.ID.Order(sql.OrderDesc()),
			).
			AllX(ctx)
		for i, v := range []struct {
			id    int
			count ent.Value
		}{
			{users[4].ID, nil}, {users[3].ID, nil}, {users[2].ID, 1}, {users[1].ID, 2}, {users[0].ID, 3},
		} {
			require.Equal(t, v.id, ordered[i].ID)
			c, err := ordered[i].Value(as)
			require.NoError(t, err)
			require.EqualValues(t, v.count, c)
		}

		ordered = client.User.Query().
			Order(user.E.Pets.OrderByCount(
				sql.OrderDesc(),
				sql.OrderSelectAs(as),
			), user.F.ID.Order(),
			).
			AllX(ctx)
		for i, v := range []struct {
			id    int
			count ent.Value
		}{
			{users[0].ID, 3}, {users[1].ID, 2}, {users[2].ID, 1}, {users[3].ID, nil}, {users[4].ID, nil},
		} {
			require.Equal(t, v.id, ordered[i].ID)
			c, err := ordered[i].Value(as)
			require.NoError(t, err)
			require.EqualValues(t, v.count, c)
		}
	})

	t.Run("O2M/Sum", func(t *testing.T) {
		ordered := client.User.Query().
			Order(user.E.Pets.OrderBy(
				sql.OrderBySum(
					pet.FieldAge,
					sql.OrderDesc(),
				),
			), user.F.ID.Order(),
			).
			AllX(ctx)
		require.Equal(t,
			[]int{users[2].ID, users[1].ID, users[0].ID, users[3].ID, users[4].ID},
			[]int{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID, ordered[4].ID},
		)

		ordered = client.User.Query().
			Order(user.E.Pets.OrderBy(
				sql.OrderBySum(
					pet.FieldAge,
					sql.OrderDesc(),
					sql.OrderSelected(),
				),
			), user.F.ID.Order(
				sql.OrderDesc(),
			),
			).
			AllX(ctx)
		require.Equal(t,
			[]int{users[2].ID, users[1].ID, users[0].ID, users[3].ID, users[4].ID},
			[]int{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[4].ID, ordered[3].ID},
		)
		s, err := ordered[0].Value("sum_age")
		require.NoError(t, err)
		require.EqualValues(t, 10, s)
		s, err = ordered[1].Value("sum_age")
		require.NoError(t, err)
		require.EqualValues(t, 4, s)
		s, err = ordered[2].Value("sum_age")
		require.NoError(t, err)
		require.EqualValues(t, 3, s)
	})
}

// Testing the "low-level" behavior of the sqlgraph package.
// This functionality may be extended to the generated fluent API.
func OrderByEdgeCount(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	users := client.User.CreateBulk(
		client.User.Create().With(user.F.Name.Set("a"), user.F.Age.Set(1)),
		client.User.Create().With(user.F.Name.Set("b"), user.F.Age.Set(2)),
		client.User.Create().With(user.F.Name.Set("c"), user.F.Age.Set(3)),
		client.User.Create().With(user.F.Name.Set("d"), user.F.Age.Set(4)),
	).SaveX(ctx)
	pets := client.Pet.CreateBulk(
		client.Pet.Create().With(pet.F.Name.Set("aa"), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ab"), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ac"), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ba"), pet.E.Owner.SetID(users[1].ID)),
		client.Pet.Create().With(pet.F.Name.Set("bb"), pet.E.Owner.SetID(users[1].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ca"), pet.E.Owner.SetID(users[2].ID)),
		client.Pet.Create().With(pet.F.Name.Set("d")),
		client.Pet.Create().With(pet.F.Name.Set("e")),
	).SaveX(ctx)
	// O2M edge.
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{users[0].ID, users[1].ID, users[2].ID, users[3].ID}},
		{ids: []int{users[3].ID, users[2].ID, users[1].ID, users[0].ID}},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(pet.Table, pet.OwnerColumn),
						sqlgraph.Edge(sqlgraph.O2M, false, pet.Table, pet.OwnerColumn),
					),
					tt.opts...,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	// M2O edge (true or false).
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{pets[6].ID, pets[7].ID, pets[0].ID, pets[1].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[5].ID}},
		{ids: []int{pets[0].ID, pets[1].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[5].ID, pets[6].ID, pets[7].ID}},
	} {
		ids := client.Pet.Query().
			Order(
				func(s *sql.Selector) {
					sqlgraph.OrderByNeighborsCount(s,
						sqlgraph.NewStep(
							sqlgraph.From(pet.Table, pet.OwnerColumn),
							sqlgraph.To(user.Table, user.FieldID),
							sqlgraph.Edge(sqlgraph.M2O, true, pet.Table, pet.OwnerColumn),
						),
						tt.opts...,
					)
				},
				ent.Asc(pet.FieldID),
			).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	inf, exp := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx), time.Now()
	groups := client.Group.CreateBulk(
		client.Group.Create().With(group.F.Name.Set("Group: 4 users"), group.F.Expire.Set(exp), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users)...)),
		client.Group.Create().With(group.F.Name.Set("Group: 3 users"), group.F.Expire.Set(exp), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users[:3])...)),
		client.Group.Create().With(group.F.Name.Set("Group: 2 users"), group.F.Expire.Set(exp), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users[:2])...)),
		client.Group.Create().With(group.F.Name.Set("Group: 1 users"), group.F.Expire.Set(exp), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users[:1])...)),
		client.Group.Create().With(group.F.Name.Set("Group: 0 users"), group.F.Expire.Set(exp), group.E.Info.SetID(inf.ID)),
	).SaveX(ctx)
	// M2M edge (inverse).
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{groups[0].ID, groups[1].ID, groups[2].ID, groups[3].ID, groups[4].ID}},
		{ids: []int{groups[4].ID, groups[3].ID, groups[2].ID, groups[1].ID, groups[0].ID}},
	} {
		ids := client.Group.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(group.Table, group.FieldID),
						sqlgraph.To(user.Table, user.FieldID),
						sqlgraph.Edge(sqlgraph.M2M, true, group.UsersTable, group.UsersPrimaryKey...),
					),
					tt.opts...,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	// M2M edge (assoc).
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{users[0].ID, users[1].ID, users[2].ID, users[3].ID}},
		{ids: []int{users[3].ID, users[2].ID, users[1].ID, users[0].ID}},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(group.Table, group.FieldID),
						sqlgraph.Edge(sqlgraph.M2M, false, user.GroupsTable, user.GroupsPrimaryKey...),
					),
					tt.opts...,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}

	t.Run("Value", func(t *testing.T) {
		const as = "pets_count"
		nodes := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(pet.Table, pet.FieldID),
						sqlgraph.Edge(sqlgraph.O2M, false, pet.Table, pet.OwnerColumn),
					),
					sql.OrderDesc(),
					sql.OrderSelectAs(as),
				)
			}).
			AllX(ctx)
		require.Equal(t, 4, len(nodes))
		// Values may be nil in case of NULL.
		for i, v := range []any{3, 2, 1, nil} {
			vv, err := nodes[i].Value(as)
			require.NoError(t, err)
			require.EqualValues(t, v, vv)
		}
		// An error is returned if the value was not selected.
		_, err := nodes[0].Value("unknown")
		require.Error(t, err)
	})
}

// Testing the "low-level" behavior of the sqlgraph package.
// This functionality may be extended to the generated fluent API.
func OrderByEdgeTerms(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	users := client.User.CreateBulk(
		client.User.Create().With(user.F.Name.Set("a"), user.F.Age.Set(1)),
		client.User.Create().With(user.F.Name.Set("b"), user.F.Age.Set(2)),
		client.User.Create().With(user.F.Name.Set("c"), user.F.Age.Set(3)),
		client.User.Create().With(user.F.Name.Set("d"), user.F.Age.Set(4)),
	).SaveX(ctx)
	pets := client.Pet.CreateBulk(
		client.Pet.Create().With(pet.F.Name.Set("aa"), pet.F.Age.Set(2), pet.E.Owner.SetID(users[1].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ab"), pet.F.Age.Set(2), pet.E.Owner.SetID(users[1].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ac"), pet.F.Age.Set(1), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ba"), pet.F.Age.Set(1), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("bb"), pet.F.Age.Set(1), pet.E.Owner.SetID(users[0].ID)),
		client.Pet.Create().With(pet.F.Name.Set("ca"), pet.F.Age.Set(3), pet.E.Owner.SetID(users[2].ID)),
		client.Pet.Create().With(pet.F.Name.Set("d")),
		client.Pet.Create().With(pet.F.Name.Set("e")),
	).SaveX(ctx)
	// M2O edge (inverse).
	// Order pets by their owner's name.
	for _, tt := range []struct {
		opt sql.OrderTerm
		ids []int
	}{
		{
			opt: sql.OrderByField(user.FieldName),
			ids: []int{pets[6].ID, pets[7].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[0].ID, pets[1].ID, pets[5].ID},
		},
		{
			opt: sql.OrderByField(user.FieldName, sql.OrderDesc()),
			ids: []int{pets[5].ID, pets[0].ID, pets[1].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[6].ID, pets[7].ID},
		},
	} {
		ids := client.Pet.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborTerms(s,
					sqlgraph.NewStep(
						sqlgraph.From(pet.Table, pet.FieldID),
						sqlgraph.To(user.Table, user.FieldID),
						sqlgraph.Edge(sqlgraph.M2O, true, pet.Table, pet.OwnerColumn),
					),
					tt.opt,
				)
			}).
			Order(ent.Asc(pet.FieldID)).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	// O2M edge (aggregation).
	for _, tt := range []struct {
		opt sql.OrderTerm
		ids []int
	}{
		{
			opt: sql.OrderBySum(user.FieldAge),
			ids: []int{users[3].ID, users[0].ID, users[2].ID, users[1].ID},
		},
		{
			opt: sql.OrderBySum(user.FieldAge, sql.OrderDesc()),
			ids: []int{users[1].ID, users[0].ID, users[2].ID, users[3].ID},
		},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborTerms(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(pet.Table, pet.FieldID),
						sqlgraph.Edge(sqlgraph.O2M, false, pet.Table, pet.OwnerColumn),
					),
					tt.opt,
				)
			}).
			Order(ent.Asc(user.FieldID)).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}

	inf, exp := client.GroupInfo.Create().With(groupinfo.F.Desc.Set("desc")).SaveX(ctx), time.Now()
	client.Group.CreateBulk(
		client.Group.Create().With(group.F.Name.Set("Group: 4 users"), group.F.Expire.Set(exp), group.F.MaxUsers.Set(40), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users)...)),
		client.Group.Create().With(group.F.Name.Set("Group: 3 users"), group.F.Expire.Set(exp), group.F.MaxUsers.Set(20), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users[:3])...)),
		client.Group.Create().With(group.F.Name.Set("Group: 2 users"), group.F.Expire.Set(exp), group.F.MaxUsers.Set(20), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users[:2])...)),
		client.Group.Create().With(group.F.Name.Set("Group: 1 users"), group.F.Expire.Set(exp), group.F.MaxUsers.Set(100), group.E.Info.SetID(inf.ID), group.E.Users.AddIDs(userIDs(users[:1])...)),
		client.Group.Create().With(group.F.Name.Set("Group: 0 users"), group.F.Expire.Set(exp), group.E.Info.SetID(inf.ID)),
	).ExecX(ctx)
	// M2M edge.
	for _, tt := range []struct {
		opt sql.OrderTerm
		ids []int
	}{
		{
			opt: sql.OrderBySum(
				group.FieldMaxUsers,
			),
			ids: []int{users[3].ID, users[2].ID, users[1].ID, users[0].ID},
		},
		{
			opt: sql.OrderBySum(
				group.FieldMaxUsers,
				sql.OrderDesc(),
			),
			ids: []int{users[0].ID, users[1].ID, users[2].ID, users[3].ID},
		},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborTerms(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(group.Table, group.FieldID),
						sqlgraph.Edge(sqlgraph.M2M, false, user.GroupsTable, user.GroupsPrimaryKey...),
					),
					tt.opt,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
}

func skip(t *testing.T, names ...string) {
	for _, n := range names {
		if strings.Contains(t.Name(), n) {
			t.Skipf("skip %s", n)
		}
	}
}

func drop(t *testing.T, client *ent.Client) {
	t.Log("drop data from database")
	ctx := context.Background()
	client.Pet.Delete().ExecX(ctx)
	client.Item.Delete().ExecX(ctx)
	client.Task.Delete().ExecX(ctx)
	client.File.Delete().ExecX(ctx)
	client.Card.Delete().ExecX(ctx)
	client.Node.Delete().ExecX(ctx)
	client.User.Delete().ExecX(ctx)
	client.Group.Delete().ExecX(ctx)
	client.Comment.Delete().ExecX(ctx)
	client.GroupInfo.Delete().ExecX(ctx)
	client.FieldType.Delete().ExecX(ctx)
	client.FileType.Delete().ExecX(ctx)
	client.ExValueScan.Delete().ExecX(ctx)
}
