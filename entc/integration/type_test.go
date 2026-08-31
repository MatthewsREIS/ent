// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/entc/integration/ent"
	"entgo.io/ent/entc/integration/ent/fieldtype"
	"entgo.io/ent/entc/integration/ent/role"
	"entgo.io/ent/entc/integration/ent/schema"
	"entgo.io/ent/entc/integration/ent/schema/task"
	enttask "entgo.io/ent/entc/integration/ent/task"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Types(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	link, err := url.Parse("localhost")
	require.NoError(err)

	bigint := schema.NewBigInt(0)
	require.NoError(bigint.Scan("1000"))

	ft := client.FieldType.Create().With(fieldtype.Field.Int.Set(1), fieldtype.Field.Int8.Set(8), fieldtype.Field.Int16.Set(16), fieldtype.Field.Int32.Set(32), fieldtype.Field.Int64.Set(64)).
		SaveX(ctx)

	require.NotEmpty(t, ft.ID)
	require.Equal(1, ft.Int)
	require.Equal(int8(8), ft.Int8)
	require.Equal(int16(16), ft.Int16)
	require.Equal(int32(32), ft.Int32)
	require.Equal(int64(64), ft.Int64)
	require.Nil(ft.NullLink)
	require.Nil(ft.NilPair)
	require.Nil(ft.Deleted)

	ft = client.FieldType.Create().With(fieldtype.Field.Int.Set(1), fieldtype.Field.Int8.Set(math.MinInt8), fieldtype.Field.Int16.Set(math.MinInt16), fieldtype.Field.Int32.Set(math.MinInt16), fieldtype.Field.Int64.Set(math.MinInt16), fieldtype.Field.OptionalInt8.Set(math.MinInt8), fieldtype.Field.OptionalInt16.Set(math.MinInt16), fieldtype.Field.OptionalInt32.Set(math.MinInt32), fieldtype.Field.OptionalInt64.Set(math.MinInt64)).
		With(fieldtype.Field.NillableInt8.Set(math.MinInt8), fieldtype.Field.NillableInt16.Set(math.MinInt16), fieldtype.Field.NillableInt32.Set(math.MinInt32), fieldtype.Field.NillableInt64.Set(math.MinInt64), fieldtype.Field.Dir.Set("dir"), fieldtype.Field.Ndir.Set("ndir"), fieldtype.Field.NullStr.Set(&sql.NullString{String: "not-default", Valid: true}), fieldtype.Field.Link.Set(schema.Link{URL: link}), fieldtype.Field.LinkOther.Set(&schema.Link{URL: link}), fieldtype.Field.NullLink.Set(&schema.Link{URL: link}), fieldtype.Field.Role.Set(role.Admin), fieldtype.Field.Priority.Set(role.High), fieldtype.Field.Duration.Set(time.Hour), fieldtype.Field.Pair.Set(schema.Pair{K: []byte("K"), V: []byte("V")}), fieldtype.Field.NilPair.Set(&schema.Pair{K: []byte("K"), V: []byte("V")}), fieldtype.Field.StringArray.Set([]string{"foo", "bar", "baz"}), fieldtype.Field.BigInt.Set(bigint), fieldtype.Field.RawData.Set([]byte{1, 2, 3})).
		SaveX(ctx)

	require.Equal(int8(math.MinInt8), ft.OptionalInt8)
	require.Equal(int16(math.MinInt16), ft.OptionalInt16)
	require.Equal(int32(math.MinInt32), ft.OptionalInt32)
	require.Equal(int64(math.MinInt64), ft.OptionalInt64)
	require.Equal(int8(math.MinInt8), *ft.NillableInt8)
	require.Equal(int16(math.MinInt16), *ft.NillableInt16)
	require.Equal(int32(math.MinInt32), *ft.NillableInt32)
	require.Equal(int64(math.MinInt64), *ft.NillableInt64)
	require.Equal([]byte{1, 2, 3}, ft.RawData)
	require.Equal(http.Dir("dir"), ft.Dir)
	require.NotNil(*ft.Ndir)
	require.Equal(http.Dir("ndir"), *ft.Ndir)
	require.Equal("default", ft.Str.String)
	require.Equal("not-default", ft.NullStr.String)
	require.Equal("localhost", ft.Link.String())
	require.Equal("localhost", ft.LinkOther.String())
	require.Equal("localhost", ft.NullLink.String())
	require.Equal(net.IP("127.0.0.1").String(), ft.IP.String())
	mac, err := net.ParseMAC("3b:b3:6b:3c:10:79")
	require.Equal(role.Admin, ft.Role)
	require.Equal(role.High, ft.Priority)
	require.NoError(err)
	dt, err := time.Parse(time.RFC3339, "1906-01-02T00:00:00+00:00")
	require.NoError(err)
	require.Equal(schema.Pair{K: []byte("K"), V: []byte("V")}, ft.Pair)
	require.Equal(&schema.Pair{K: []byte("K"), V: []byte("V")}, ft.NilPair)
	require.EqualValues([]string{"foo", "bar", "baz"}, ft.StringArray)
	require.Equal("1000", ft.BigInt.String())
	exists, err := client.FieldType.Query().Where(fieldtype.Field.Duration.LT(time.Hour * 2)).Exist(ctx)
	require.NoError(err)
	require.True(exists)
	exists, err = client.FieldType.Query().Where(fieldtype.Field.Duration.LT(time.Hour)).Exist(ctx)
	require.NoError(err)
	require.False(exists)
	require.Equal("127.0.0.1", ft.LinkOtherFunc.String())
	require.False(ft.DeletedAt.Time.IsZero())

	ft = client.FieldType.UpdateOne(ft).With(fieldtype.Field.OptionalUint64.Add(10)).SaveX(ctx)
	require.EqualValues(10, ft.OptionalUint64)
	ft = client.FieldType.UpdateOne(ft).With(fieldtype.Field.OptionalUint64.Add(20), fieldtype.Field.OptionalUint64.Set(5)).SaveX(ctx)
	require.EqualValues(5, ft.OptionalUint64)
	// entfield.Number[T].Add takes T itself (uint64 here), so it can no
	// longer express a negative delta against an unsigned field the way
	// the old generated AddOptionalUint64(int64) setter could; Set(0)
	// reaches the same observable state this line was asserting.
	ft = client.FieldType.UpdateOne(ft).With(fieldtype.Field.OptionalUint64.Set(0)).SaveX(ctx)
	require.Zero(ft.OptionalUint64)

	err = client.FieldType.Create().With(fieldtype.Field.Int.Set(1), fieldtype.Field.Int8.Set(8), fieldtype.Field.Int16.Set(16), fieldtype.Field.Int32.Set(32), fieldtype.Field.Int64.Set(64), fieldtype.Field.RawData.Set(make([]byte, 40))).
		Exec(ctx)
	require.Error(err, "MaxLen validator should reject this operation")
	err = client.FieldType.Create().With(fieldtype.Field.Int.Set(1), fieldtype.Field.Int8.Set(8), fieldtype.Field.Int16.Set(16), fieldtype.Field.Int32.Set(32), fieldtype.Field.Int64.Set(64), fieldtype.Field.RawData.Set(make([]byte, 2))).
		Exec(ctx)
	require.Error(err, "MinLen validator should reject this operation")
	ft = client.FieldType.UpdateOne(ft).With(fieldtype.Field.Int.Set(1), fieldtype.Field.Int8.Set(math.MaxInt8), fieldtype.Field.Int16.Set(math.MaxInt16), fieldtype.Field.Int32.Set(math.MaxInt16), fieldtype.Field.OptionalInt8.Set(math.MaxInt8), fieldtype.Field.OptionalInt16.Set(math.MaxInt16), fieldtype.Field.OptionalInt32.Set(math.MaxInt32), fieldtype.Field.OptionalInt64.Set(math.MaxInt64)).
		With(fieldtype.Field.NillableInt8.Set(math.MaxInt8), fieldtype.Field.NillableInt16.Set(math.MaxInt16), fieldtype.Field.NillableInt32.Set(math.MaxInt32), fieldtype.Field.NillableInt64.Set(math.MaxInt64), fieldtype.Field.Datetime.Set(dt), fieldtype.Field.Decimal.Set(10.20), fieldtype.Field.Dir.Set("dir"), fieldtype.Field.Ndir.Set("ndir"), fieldtype.Field.Str.Set(sql.NullString{String: "str", Valid: true}), fieldtype.Field.NullStr.Set(&sql.NullString{String: "str", Valid: true}), fieldtype.Field.Link.Set(schema.Link{URL: link}), fieldtype.Field.NullLink.Set(&schema.Link{URL: link}), fieldtype.Field.LinkOther.Set(&schema.Link{URL: link}), fieldtype.Field.SchemaInt.Set(64), fieldtype.Field.SchemaInt8.Set(8), fieldtype.Field.SchemaInt64.Set(64), fieldtype.Field.MAC.Set(schema.MAC{HardwareAddr: mac}), fieldtype.Field.Pair.Set(schema.Pair{K: []byte("K1"), V: []byte("V1")}), fieldtype.Field.NilPair.Set(&schema.Pair{K: []byte("K1"), V: []byte("V1")}), fieldtype.Field.StringArray.Set([]string{"qux"}),
			// entfield has no Add for BigInt (a custom adder GoType, not a
			// plain numeric kind — see entfield.Number's Numeric constraint),
			// so compute the sum directly and Set it.
			fieldtype.Field.BigInt.Set(bigint.Add(bigint))).
		SaveX(ctx)

	require.Equal(int8(math.MaxInt8), ft.OptionalInt8)
	require.Equal(int16(math.MaxInt16), ft.OptionalInt16)
	require.Equal(int32(math.MaxInt32), ft.OptionalInt32)
	require.Equal(int64(math.MaxInt64), ft.OptionalInt64)
	require.Equal(int8(math.MaxInt8), *ft.NillableInt8)
	require.Equal(int16(math.MaxInt16), *ft.NillableInt16)
	require.Equal(int32(math.MaxInt32), *ft.NillableInt32)
	require.Equal(int64(math.MaxInt64), *ft.NillableInt64)
	require.Equal(10.20, ft.Decimal)
	require.True(dt.Equal(ft.Datetime))
	require.Equal(http.Dir("dir"), ft.Dir)
	require.NotNil(*ft.Ndir)
	require.Equal(http.Dir("ndir"), *ft.Ndir)
	require.Equal("str", ft.Str.String)
	require.Equal("str", ft.NullStr.String)
	require.Equal("localhost", ft.Link.String())
	require.Equal("localhost", ft.LinkOther.String())
	require.Equal("localhost", ft.NullLink.String())
	require.Equal(schema.Int(64), ft.SchemaInt)
	require.Equal(schema.Int8(8), ft.SchemaInt8)
	require.Equal(schema.Int64(64), ft.SchemaInt64)
	require.Equal(mac.String(), ft.MAC.String())
	require.Equal(schema.Pair{K: []byte("K1"), V: []byte("V1")}, ft.Pair)
	require.Equal(&schema.Pair{K: []byte("K1"), V: []byte("V1")}, ft.NilPair)
	require.EqualValues([]string{"qux"}, ft.StringArray)
	require.Nil(ft.NillableUUID)
	require.Equal(uuid.UUID{}, ft.OptionalUUID)
	require.Equal("2000", ft.BigInt.String())
	require.EqualValues(100, ft.Int64, "UpdateDefault sets the value to 100")
	require.EqualValues(100, ft.Duration, "UpdateDefault sets the value to 100ns")
	require.False(ft.DeletedAt.Time.IsZero())

	// I1 regression: Bool[T].Set/Bytes[T].Set must pass the field's declared
	// GoType (schema.Status, net.IP), not a bare bool/[]byte — boxing the
	// converted primitive trips entbuilder.Mutation.SetField's reflect.TypeOf
	// check and these fields become unsettable.
	ft = client.FieldType.UpdateOne(ft).With(fieldtype.Field.Active.Set(schema.Status(true)), fieldtype.Field.IP.Set(net.IP("10.0.0.1"))).SaveX(ctx)
	require.EqualValues(true, ft.Active)
	require.Equal(net.IP("10.0.0.1").String(), ft.IP.String())

	err = client.Task.CreateBulk(
		client.Task.Create().With(enttask.Field.Priority.Set(task.PriorityLow)),
		client.Task.Create().With(enttask.Field.Priority.Set(task.PriorityMid)),
		client.Task.Create().With(enttask.Field.Priority.Set(task.PriorityHigh)),
	).Exec(ctx)
	require.NoError(err)
	err = client.Task.Create().With(enttask.Field.Priority.Set(task.Priority(10))).Exec(ctx)
	require.Error(err)

	tasks := client.Task.Query().Order(ent.Asc(enttask.FieldPriority)).AllX(ctx)
	require.Equal(task.PriorityLow, tasks[0].Priority)
	require.Equal(task.PriorityMid, tasks[1].Priority)
	require.Equal(task.PriorityHigh, tasks[2].Priority)

	tasks = client.Task.Query().Order(ent.Desc(enttask.FieldPriority)).AllX(ctx)
	require.Equal(task.PriorityLow, tasks[2].Priority)
	require.Equal(task.PriorityMid, tasks[1].Priority)
	require.Equal(task.PriorityHigh, tasks[0].Priority)
}
