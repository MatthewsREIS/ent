// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/ent"
	"entgo.io/ent/entc/integration/ent/card"
	"entgo.io/ent/entc/integration/ent/enttest"
	"entgo.io/ent/entc/integration/ent/hook"
	"entgo.io/ent/entc/integration/ent/migrate"
	"entgo.io/ent/entc/integration/ent/user"

	"github.com/stretchr/testify/require"
)

// TestMutationStateSurface covers mutation-state machinery paths (ResetX,
// Fields, AddedFields, FieldCleared, AddedField, Mutation().IDs, Mutation().Op,
// Mutation().Where) on the root integration entities. Regression net for
// Phase 4 of the ent code-reduction refactor.
func TestMutationStateSurface(t *testing.T) {
	ctx := context.Background()
	openClient := func(t *testing.T) *ent.Client {
		t.Helper()
		return enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=1",
			enttest.WithMigrateOptions(
				migrate.WithDropIndex(true),
				migrate.WithDropColumn(true),
			),
		)
	}

	t.Run("CardMutation_Fields_ReturnsKnownFields", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		var captured []string
		client.Card.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.CardFunc(func(ctx context.Context, m *ent.CardMutation) (ent.Value, error) {
				captured = m.Fields()
				return next.Mutate(ctx, m)
			})
		}, ent.OpCreate))
		u := client.User.Create().SetAge(1).SetName("a8m").SaveX(ctx)
		client.Card.Create().SetNumber("1234").SetOwnerID(u.ID).ExecX(ctx)
		require.Contains(t, captured, card.FieldNumber, "Fields() should include 'number'")
	})

	t.Run("CardMutation_ResetName_ClearsField", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		var nameSetBeforeReset bool
		var nameAfterReset string
		client.Card.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.CardFunc(func(ctx context.Context, m *ent.CardMutation) (ent.Value, error) {
				_, nameSetBeforeReset = m.Name()
				m.ResetName()
				nameAfterReset, _ = m.Name()
				return next.Mutate(ctx, m)
			})
		}, ent.OpCreate))
		u := client.User.Create().SetAge(1).SetName("a8m").SaveX(ctx)
		client.Card.Create().SetNumber("1234").SetName("before-reset").SetOwnerID(u.ID).ExecX(ctx)
		require.True(t, nameSetBeforeReset, "Name() should be set before ResetName()")
		require.Empty(t, nameAfterReset, "Name() should be empty after ResetName()")
	})

	t.Run("CardMutation_FieldCleared_DetectsClearName", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		u := client.User.Create().SetAge(1).SetName("a8m").SaveX(ctx)
		crd := client.Card.Create().SetNumber("1234").SetName("mycard").SetOwnerID(u.ID).SaveX(ctx)
		var nameCleared bool
		client.Card.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.CardFunc(func(ctx context.Context, m *ent.CardMutation) (ent.Value, error) {
				nameCleared = m.FieldCleared(card.FieldName)
				return next.Mutate(ctx, m)
			})
		}, ent.OpUpdateOne))
		client.Card.UpdateOneID(crd.ID).ClearName().SaveX(ctx)
		require.True(t, nameCleared, "FieldCleared('name') should be true after ClearName()")
	})

	t.Run("CardMutation_AddBalance_VisibleViaAddedField", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		var addedFields []string
		var balanceDelta ent.Value
		var ok bool
		client.Card.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.CardFunc(func(ctx context.Context, m *ent.CardMutation) (ent.Value, error) {
				addedFields = m.AddedFields()
				balanceDelta, ok = m.AddedField(card.FieldBalance)
				return next.Mutate(ctx, m)
			})
		}, ent.OpUpdate))
		u := client.User.Create().SetAge(1).SetName("a8m").SaveX(ctx)
		client.Card.Create().SetNumber("1234").SetOwnerID(u.ID).ExecX(ctx)
		client.Card.Update().AddBalance(3.14).ExecX(ctx)
		require.Contains(t, addedFields, card.FieldBalance, "AddedFields() should include 'balance'")
		require.True(t, ok, "AddedField('balance') should return true")
		require.Equal(t, 3.14, balanceDelta, "AddedField('balance') should return the delta")
	})

	t.Run("UserMutation_Op_ReturnsCreate", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		var observedOp ent.Op
		client.User.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
				observedOp = m.Op()
				return next.Mutate(ctx, m)
			})
		}, ent.OpCreate))
		client.User.Create().SetAge(1).SetName("a8m").ExecX(ctx)
		require.True(t, observedOp.Is(ent.OpCreate), "Op() should be OpCreate during Create hook")
	})

	t.Run("UserMutation_IDs_ReturnsTargetIDs", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		target := client.User.Create().SetAge(1).SetName("target-ids").SaveX(ctx)
		client.User.Create().SetAge(2).SetName("other-ids").ExecX(ctx)
		var capturedIDs []int
		client.User.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
				ids, err := m.IDs(ctx)
				if err == nil {
					capturedIDs = ids
				}
				return next.Mutate(ctx, m)
			})
		}, ent.OpUpdate))
		client.User.Update().Where(user.Name("target-ids")).SetName("updated-ids").ExecX(ctx)
		require.Equal(t, []int{target.ID}, capturedIDs, "IDs() should return only the filtered user's ID")
	})

	t.Run("UserMutation_Where_FiltersPredicates", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		a8m := client.User.Create().SetAge(1).SetName("a8m").SaveX(ctx)
		client.User.Create().SetAge(2).SetName("nati").ExecX(ctx)
		// Attach a hook that narrows bulk-update to only the 'a8m' user.
		client.User.Use(hook.On(func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
				m.Where(user.Name("a8m"))
				return next.Mutate(ctx, m)
			})
		}, ent.OpUpdate))
		client.User.Update().SetAge(99).ExecX(ctx)
		// Only a8m should have been updated.
		require.Equal(t, 99, client.User.GetX(ctx, a8m.ID).Age)
		require.Equal(t, 2, client.User.Query().Where(user.Name("nati")).OnlyX(ctx).Age)
	})

	t.Run("HookHasFields_Combinator", func(t *testing.T) {
		client := openClient(t)
		defer client.Close()
		var fired bool
		client.User.Use(hook.If(func(next ent.Mutator) ent.Mutator {
			return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
				fired = true
				return next.Mutate(ctx, m)
			})
		}, hook.HasFields(user.FieldName)))
		// Create — always sets name, so hook should fire.
		fired = false
		client.User.Create().SetAge(1).SetName("a8m").ExecX(ctx)
		require.True(t, fired, "hook should fire when 'name' field is set on Create")
		// Bulk update that only touches age — hook should NOT fire.
		fired = false
		client.User.Update().SetAge(2).ExecX(ctx)
		require.False(t, fired, "hook should not fire when 'name' field is absent from Update")
	})
}
