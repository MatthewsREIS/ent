// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"testing"

	"entgo.io/ent/entc/integration/ent"

	"github.com/stretchr/testify/require"
)

func Indexes(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)
	t.Log("prevent inserting 2 files with the same (name, user)")
	f1, err := client.File.Create().SetName("foo").SetSize(10).SetUser("bar").Save(ctx)
	require.NoError(err)
	require.Equal("foo", f1.Name)
	require.Equal("bar", *f1.User)
	f2, err := client.File.Create().SetName("foo").SetSize(10).SetUser("bar").Save(ctx)
	require.Nil(f2)
	require.Error(err)
	require.True(ent.IsConstraintError(err))

	t.Log("deletion should allow recreation")
	client.File.DeleteOne(f1).ExecX(ctx)
	f3, err := client.File.Create().SetName("foo").SetSize(10).SetUser("bar").Save(ctx)
	require.NoError(err)
	require.Equal("foo", f3.Name)
	require.Equal("bar", *f3.User)

	t.Log("allow inserting 2 files the same name, type and NULL user (optional field)")
	png := client.FileType.Create().SetName("png").SaveX(ctx)
	f4 := client.File.Create().SetName("foo").SetSize(10).SetTypeID(png.ID).SaveX(ctx)
	f5 := client.File.Create().SetName("foo").SetSize(10).SetTypeID(png.ID).SaveX(ctx)

	t.Log("index on edge sub-graph")
	a8m := client.User.Create().SetName("a8m").SetAge(18).SaveX(ctx)
	err = client.User.UpdateOne(a8m).AddFileIDs(f4.ID).Exec(ctx)
	require.NoError(err)
	err = client.User.UpdateOne(a8m).AddFileIDs(f5.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err), "cannot have 2 files with the same (name, type, owner)")
	client.FileType.UpdateOne(png).RemoveFileIDs(f5.ID).ExecX(ctx)
	err = client.User.UpdateOne(a8m).AddFileIDs(f5.ID).Exec(ctx)
	require.NoError(err)
	err = client.FileType.UpdateOne(png).AddFileIDs(f5.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err))
	client.User.UpdateOne(a8m).RemoveFileIDs(f4.ID, f5.ID).ExecX(ctx)
	client.FileType.UpdateOne(png).AddFileIDs(f5.ID).ExecX(ctx)

	t.Log("prevent inserting duplicates files in the same insert")
	err = client.User.UpdateOne(a8m).AddFileIDs(f4.ID, f5.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err))
	require.Zero(client.User.QueryFiles(a8m).CountX(ctx))

	t.Log("edge indexes should applied on the edge sub-graph")
	nati := client.User.Create().SetName("nati").SetAge(18).AddFileIDs(f5.ID).SaveX(ctx)
	err = client.User.UpdateOne(nati).AddFileIDs(f4.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(a8m).AddFileIDs(f4.ID).Exec(ctx)
	require.NoError(err)

	require.Equal(1, client.User.QueryFiles(a8m).CountX(ctx))
	require.Equal(1, client.User.QueryFiles(nati).CountX(ctx))
}
