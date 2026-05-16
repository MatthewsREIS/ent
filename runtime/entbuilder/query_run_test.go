// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

// fakeUser is a stand-in for a generated User type.
type fakeUser struct{ Name string }

// fakeUserQuery is a stand-in for a generated *UserQuery; it satisfies ent.Query (the empty interface).
type fakeUserQuery struct {
	entbuilder.QueryState[testPred]
}

func TestRunAll_Success(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prepared := false
	sqlAll := func(ctx context.Context) ([]*fakeUser, error) {
		require.True(t, prepared, "prepareQuery must run before sqlAll")
		return []*fakeUser{{Name: "alice"}, {Name: "bob"}}, nil
	}
	prep := func(_ context.Context) error { prepared = true; return nil }

	nodes, err := entbuilder.RunAll[[]*fakeUser](context.Background(), q, q.Ctx, "QueryAll", nil, prep, sqlAll)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	require.Equal(t, "alice", nodes[0].Name)
	require.Equal(t, "QueryAll", q.Ctx.Op, "op must be attached to ctx via QueryState.Ctx")
}

func TestRunAll_PrepareQueryError_Aborts(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	wantErr := errors.New("prep boom")
	prep := func(_ context.Context) error { return wantErr }
	sqlCalled := false
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		sqlCalled = true
		return nil, nil
	}

	nodes, err := entbuilder.RunAll[[]*fakeUser](context.Background(), q, q.Ctx, "QueryAll", nil, prep, sqlAll)
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, nodes)
	require.False(t, sqlCalled, "sqlAll must not run when prepareQuery fails")
}

func TestRunAll_InterceptorChainRunsAroundSqlAll(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	var order []string
	mk := func(name string) ent.Interceptor {
		return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, qq ent.Query) (ent.Value, error) {
				order = append(order, name+":pre")
				v, err := next.Query(ctx, qq)
				order = append(order, name+":post")
				return v, err
			})
		})
	}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		order = append(order, "sql")
		return []*fakeUser{}, nil
	}

	_, err := entbuilder.RunAll[[]*fakeUser](context.Background(), q, q.Ctx, "QueryAll",
		[]ent.Interceptor{mk("outer"), mk("inner")}, prep, sqlAll)
	require.NoError(t, err)
	require.Equal(t, []string{"outer:pre", "inner:pre", "sql", "inner:post", "outer:post"}, order)
}

func TestRunCount_Success(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlCount := func(context.Context) (int, error) { return 7, nil }

	got, err := entbuilder.RunCount(context.Background(), q, q.Ctx, "QueryCount", nil, prep, sqlCount)
	require.NoError(t, err)
	require.Equal(t, 7, got)
}

func TestRunFirst_NodeFound(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		return []*fakeUser{{Name: "first"}, {Name: "second"}}, nil
	}
	notFound := func(label string) error { return errors.New(label + ": not found") }

	n, err := entbuilder.RunFirst[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryFirst", "User", nil, prep, sqlAll, notFound)
	require.NoError(t, err)
	require.Equal(t, "first", n.Name)
}

func TestRunFirst_NotFoundError(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) { return nil, nil }
	notFound := func(label string) error { return errors.New(label + ": not found") }

	n, err := entbuilder.RunFirst[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryFirst", "User", nil, prep, sqlAll, notFound)
	require.Error(t, err)
	require.Contains(t, err.Error(), "User: not found")
	require.Nil(t, n)
}

func TestRunOnly_ExactlyOne(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) { return []*fakeUser{{Name: "x"}}, nil }
	notFound := func(label string) error { return errors.New(label + ": not found") }
	notSingular := func(label string) error { return errors.New(label + ": not singular") }

	n, err := entbuilder.RunOnly[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryOnly", "User", nil, prep, sqlAll, notFound, notSingular)
	require.NoError(t, err)
	require.Equal(t, "x", n.Name)
}

func TestRunOnly_MultipleResults_Error(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		return []*fakeUser{{Name: "a"}, {Name: "b"}}, nil
	}
	notFound := func(label string) error { return errors.New(label + ": not found") }
	notSingular := func(label string) error { return errors.New(label + ": not singular") }

	_, err := entbuilder.RunOnly[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryOnly", "User", nil, prep, sqlAll, notFound, notSingular)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not singular")
}
