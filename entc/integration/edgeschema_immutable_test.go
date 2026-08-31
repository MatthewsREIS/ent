// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/edgeschema/ent"
	"entgo.io/ent/entc/integration/edgeschema/ent/friendship"
	"entgo.io/ent/entc/integration/edgeschema/ent/migrate"
	"entgo.io/ent/entc/integration/edgeschema/ent/user"
	_ "entgo.io/ent/entc/integration/edgeschema/ent/runtime"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestSQLiteEdgeSchemaFriendshipImmutableEdges is C1's integration regression
// test: edgeschema's Friendship has two required+unique+immutable edges
// ("user"/"friend") backed by immutable FK fields (user_id/friend_id — see
// entc/integration/edgeschema/ent/schema/friendship.go). Before C1,
// EdgeSpec had no Immutable flag, so E.User.SetID / F.UserID.Set / E.Friend.
// Clear on an UpdateOneID builder compiled, returned a nil error from Save,
// and wrote nothing (sqlSave's per-edge {{continue}} on $e.Immutable never
// even reads the mutation's edge state on update — the write vanished
// silently). Named with the TestSQLite prefix so `go test -run TestSQLite
// ./...` from entc/integration picks it up.
func TestSQLiteEdgeSchemaFriendshipImmutableEdges(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	a8m := client.User.Create().With(user.F.Name.Set("a8m")).SaveX(ctx)
	nat := client.User.Create().With(user.F.Name.Set("nati")).SaveX(ctx)
	other := client.User.Create().With(user.F.Name.Set("other")).SaveX(ctx)

	f := client.Friendship.Create().
		With(friendship.E.User.SetID(a8m.ID), friendship.E.Friend.SetID(nat.ID)).
		SaveX(ctx)
	require.Equal(t, a8m.ID, f.UserID)
	require.Equal(t, nat.ID, f.FriendID)

	// E.<Edge>.SetID on an immutable edge must error at Save, not silently
	// no-op.
	err = client.Friendship.UpdateOneID(f.ID).With(friendship.E.User.SetID(other.ID)).Exec(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "immutable")

	// F.<EdgeField>.Set routes through the same SetEdgeID guard.
	err = client.Friendship.UpdateOneID(f.ID).With(friendship.F.UserID.Set(other.ID)).Exec(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "immutable")

	// E.<Edge>.Clear on an immutable edge must error too.
	err = client.Friendship.UpdateOneID(f.ID).With(friendship.E.Friend.Clear()).Exec(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "immutable")

	// Nothing above actually wrote: the row still points at the original
	// user/friend.
	got := client.Friendship.GetX(ctx, f.ID)
	require.Equal(t, a8m.ID, got.UserID)
	require.Equal(t, nat.ID, got.FriendID)
}
