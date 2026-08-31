// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package privacy

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/entc/integration/privacy/ent/enttest"
	"entgo.io/ent/entc/integration/privacy/ent/privacy"
	"entgo.io/ent/entc/integration/privacy/ent/task"
	"entgo.io/ent/entc/integration/privacy/ent/team"
	"entgo.io/ent/entc/integration/privacy/ent/user"
	"entgo.io/ent/entc/integration/privacy/rule"
	"entgo.io/ent/entc/integration/privacy/viewer"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestPrivacyRules(t *testing.T) {
	client := enttest.Open(t, "sqlite3",
		"file:ent?mode=memory&cache=shared&_fk=1",
	)
	defer client.Close()
	logf := rule.SetMutationLogFunc(func(string, ...any) {
		require.FailNow(t, "hook called on privacy deny")
	})
	ctx := context.Background()
	_, err := client.Team.Create().With(team.Field.Name.Set("ent")).Save(ctx)
	require.True(t, errors.Is(err, privacy.Deny), "policy requires viewer context")
	view := viewer.NewContext(ctx, viewer.AppViewer{
		Role: viewer.View,
	})
	_, err = client.Team.CreateBulk(
		client.Team.Create().With(team.Field.Name.Set("ent")),
		client.Team.Create().With(team.Field.Name.Set("ent-contrib")),
	).Save(view)
	require.True(t, errors.Is(err, privacy.Deny), "team policy requires admin user")
	rule.SetMutationLogFunc(logf)

	admin := viewer.NewContext(ctx, viewer.AppViewer{
		Role: viewer.Admin,
	})
	teams := client.Team.CreateBulk(
		client.Team.Create().With(team.Field.Name.Set("ent")),
		client.Team.Create().With(team.Field.Name.Set("ent-contrib")),
	).SaveX(admin)

	_, err = client.User.Create().With(user.Field.Name.Set("a8m"), user.Edge.Teams.AddIDs(teams[0].ID)).Save(view)
	require.True(t, errors.Is(err, privacy.Deny), "user creation requires admin user")
	a8m := client.User.Create().With(user.Field.Name.Set("a8m"), user.Edge.Teams.AddIDs(teams[0].ID, teams[1].ID)).SaveX(admin)
	nat := client.User.Create().With(user.Field.Name.Set("nati"), user.Edge.Teams.AddIDs(teams[1].ID)).SaveX(admin)

	_, err = client.Task.Create().With(task.Field.Title.Set("task 1"), task.Edge.Teams.AddIDs(teams[0].ID), task.Edge.Owner.SetID(a8m.ID)).Save(ctx)
	require.True(t, errors.Is(err, privacy.Deny), "task creation requires viewer/owner match")

	a8mctx := viewer.NewContext(view, &viewer.UserViewer{User: a8m, Role: viewer.View | viewer.Edit})
	client.Task.Create().With(task.Field.Title.Set("task 1"), task.Edge.Teams.AddIDs(teams[0].ID), task.Edge.Owner.SetID(a8m.ID)).SaveX(a8mctx)
	_, err = client.Task.Create().With(task.Field.Title.Set("task 2"), task.Edge.Teams.AddIDs(teams[1].ID), task.Edge.Owner.SetID(nat.ID)).Save(a8mctx)
	require.True(t, errors.Is(err, privacy.Deny), "task creation requires viewer/owner match")

	natctx := viewer.NewContext(view, &viewer.UserViewer{User: nat, Role: viewer.View | viewer.Edit})
	client.Task.Create().With(task.Field.Title.Set("task 2"), task.Edge.Teams.AddIDs(teams[1].ID), task.Edge.Owner.SetID(nat.ID)).SaveX(natctx)

	tasks := client.Task.Query().AllX(a8mctx)
	require.Len(t, tasks, 2, "returned tasks from teams 1, 2")
	task2 := client.Task.Query().OnlyX(natctx)
	require.Equal(t, "task 2", task2.Title, "returned tasks must be from the same team")

	task3 := client.Task.Create().With(task.Field.Title.Set("multi-team-task (1, 2)"), task.Edge.Teams.AddIDs(teams[0].ID, teams[1].ID), task.Edge.Owner.SetID(a8m.ID)).SaveX(a8mctx)
	_, err = client.Task.UpdateOne(task3).With(task.Field.Status.Set(task.StatusClosed)).Save(natctx)
	require.True(t, errors.Is(err, privacy.Deny), "viewer 2 is not allowed to change the task status")

	// DecisionContext returns a new context from the parent with a decision attached to it.
	client.Task.UpdateOne(task3).With(task.Field.Status.Set(task.StatusClosed)).SaveX(privacy.DecisionContext(natctx, privacy.Allow))
	client.Task.UpdateOne(task3).With(task.Field.Status.Set(task.StatusClosed)).SaveX(a8mctx)
	// Update description is allowed for other users in the team.
	client.Task.UpdateOne(task3).With(task.Field.Description.Set("boring description")).SaveX(natctx)
	client.Task.UpdateOne(task3).With(task.Field.Description.Set("boring description")).SaveX(a8mctx)
}
