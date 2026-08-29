package entfield_test

import (
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entfield"
	"github.com/stretchr/testify/require"
)

func render(t *testing.T, p func(*sql.Selector)) (string, []any) {
	t.Helper()
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users"))
	p(s)
	q, args := s.Query()
	require.NoError(t, s.Err())
	return q, args
}

func renderErr(t *testing.T, p func(*sql.Selector)) error {
	t.Helper()
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users"))
	p(s)
	s.Query()
	return s.Err()
}

func TestStringOps(t *testing.T) {
	name := entfield.NewString[string]("name", "name")
	q, args := render(t, name.EQ("a"))
	require.Contains(t, q, `"name" = $1`)
	require.Equal(t, []any{"a"}, args)
	q, _ = render(t, name.In("a", "b"))
	require.Contains(t, q, `"name" IN ($1, $2)`)
	q, _ = render(t, name.ContainsFold("x"))
	require.Contains(t, q, "name")
	q, _ = render(t, name.IsNil())
	require.Contains(t, q, `"name" IS NULL`)
}

func TestCustomStringType(t *testing.T) {
	type Status string
	status := entfield.NewEnum[Status]("status", "status")
	q, args := render(t, status.EQ(Status("active")))
	require.Contains(t, q, `"status" = $1`)
	require.Equal(t, []any{"active"}, args)
	q, args = render(t, status.In(Status("a"), Status("b")))
	require.Contains(t, q, "IN")
	require.Equal(t, []any{"a", "b"}, args)
}

func TestNumberOps(t *testing.T) {
	age := entfield.NewNumber[int]("age", "age")
	q, args := render(t, age.GT(3))
	require.Contains(t, q, `"age" > $1`)
	require.Equal(t, []any{3}, args)
}

func TestTimeAndBoolAndValue(t *testing.T) {
	at := entfield.NewTime("created_at", "created_at")
	q, _ := render(t, at.LTE(time.Unix(0, 0)))
	require.Contains(t, q, `"created_at" <= $1`)
	ok := entfield.NewBool[bool]("ok", "ok")
	q, _ = render(t, ok.EQ(true))
	require.Contains(t, q, `WHERE "users"."ok"`)
	q, _ = render(t, ok.EQ(false))
	require.Contains(t, q, `WHERE NOT "users"."ok"`)
	id := entfield.NewValue[[16]byte]("id", "id")
	q, _ = render(t, id.NEQ([16]byte{}))
	require.Contains(t, q, `"id" <> $1`)
}

// TestBytesOps guards Bytes[T]'s predicate behavior across the Bytes ->
// Bytes[T ~[]byte] generic conversion (I1): predicates must still compare
// against the raw []byte column value regardless of T.
func TestBytesOps(t *testing.T) {
	raw := entfield.NewBytes[[]byte]("payload", "payload")
	q, args := render(t, raw.EQ([]byte("hi")))
	require.Contains(t, q, `"payload" = $1`)
	require.Equal(t, []any{[]byte("hi")}, args)

	type IP []byte
	ip := entfield.NewBytes[IP]("ip", "ip")
	q, args = render(t, ip.EQ(IP{127, 0, 0, 1}))
	require.Contains(t, q, `"ip" = $1`)
	require.Equal(t, []any{[]byte{127, 0, 0, 1}}, args)
}

func TestOrders(t *testing.T) {
	name := entfield.NewString[string]("name", "name")
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users"))
	name.Desc()(s)
	q, _ := s.Query()
	require.Contains(t, q, "ORDER BY")
	require.Contains(t, q, "DESC")
}

func TestScannerConversionAndError(t *testing.T) {
	good := entfield.NewStringScan[string]("slug", "slug", func(v string) (driver.Value, error) { return "s:" + v, nil })
	_, args := render(t, good.EQ("x"))
	require.Equal(t, []any{"s:x"}, args)
	bad := entfield.NewStringScan[string]("slug", "slug", func(v string) (driver.Value, error) { return nil, fmt.Errorf("boom") })
	require.ErrorContains(t, renderErr(t, bad.EQ("x")), "boom")
	require.ErrorContains(t, renderErr(t, bad.In("a", "b")), "boom")
}

// TestScannedStringOps guards against a real regression: Contains/HasPrefix/
// HasSuffix/EqualFold/ContainsFold must run values through the scan func
// too, like EQ/NEQ/etc already do — otherwise a substring match runs
// against the raw Go value while the column actually stores the scanned
// (encoded) form, silently matching nothing. See
// entc/integration/ent/exvaluescan's "custom" (hex-encoded) field for the
// real-world case this guards.
func TestScannedStringOps(t *testing.T) {
	hex := entfield.NewStringScan[string]("custom", "custom", func(v string) (driver.Value, error) { return "0x:" + v, nil })
	q, args := render(t, hex.HasPrefix("ent"))
	require.Contains(t, q, `"custom" LIKE`)
	require.Equal(t, []any{"0x:ent%"}, args)

	notStr := entfield.NewStringScan[string]("custom", "custom", func(v string) (driver.Value, error) { return 7, nil })
	require.ErrorContains(t, renderErr(t, notStr.Contains("ent")), "not a string")
}
