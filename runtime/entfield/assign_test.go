package entfield_test

import (
	"database/sql/driver"
	"errors"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"entgo.io/ent/runtime/entfield"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion: *entbuilder.Mutation[T, I] satisfies entfield.Mutable
// without an adapter (Mutable's method set is copied verbatim from
// runtime/entbuilder/mutation_methods.go).
var _ entfield.Mutable = (*entbuilder.Mutation[struct{}, int])(nil)

func ptr[T any](v T) *T { return &v }

// op is a recorded Mutable call.
type op struct {
	Op    string
	Name  string
	Value any
}

// recorder is a fake Mutable that logs every call.
type recorder struct {
	ops []op
}

func (r *recorder) SetField(name string, value ent.Value) error {
	r.ops = append(r.ops, op{"SetField", name, value})
	return nil
}

func (r *recorder) AddField(name string, value ent.Value) error {
	r.ops = append(r.ops, op{"AddField", name, value})
	return nil
}

func (r *recorder) AppendField(name string, value ent.Value) error {
	r.ops = append(r.ops, op{"AppendField", name, value})
	return nil
}

func (r *recorder) ClearField(name string) error {
	r.ops = append(r.ops, op{"ClearField", name, nil})
	return nil
}

func (r *recorder) ResetField(name string) error {
	r.ops = append(r.ops, op{"ResetField", name, nil})
	return nil
}

func (r *recorder) SetEdgeID(edge string, id any) error {
	r.ops = append(r.ops, op{"SetEdgeID", edge, id})
	return nil
}

func (r *recorder) AddEdgeIDs(edge string, ids ...any) error {
	r.ops = append(r.ops, op{"AddEdgeIDs", edge, ids})
	return nil
}

func (r *recorder) RemoveEdgeIDs(edge string, ids ...any) error {
	r.ops = append(r.ops, op{"RemoveEdgeIDs", edge, ids})
	return nil
}

func (r *recorder) ClearEdge(edge string) error {
	r.ops = append(r.ops, op{"ClearEdge", edge, nil})
	return nil
}

// errFake wraps a recorder, but fails on the named field/edge instead of recording.
type errFake struct {
	*recorder
	failOn string
	err    error
}

func (f *errFake) SetField(name string, value ent.Value) error {
	if name == f.failOn {
		return f.err
	}
	return f.recorder.SetField(name, value)
}

func TestFieldAssignments(t *testing.T) {
	rec := &recorder{}
	name := entfield.NewString[string]("name")
	require.NoError(t, entfield.Apply(rec, name.Set("a"), name.Clear(), name.SetNillable(nil), name.SetNillable(ptr("b"))))
	require.Equal(t, []op{{"SetField", "name", "a"}, {"ClearField", "name", nil}, {"SetField", "name", "b"}}, rec.ops)
}

func TestNumberSetResetsAdd(t *testing.T) {
	rec := &recorder{}
	age := entfield.NewNumber[int]("age")
	require.NoError(t, entfield.Apply(rec, age.Set(5)))
	require.Equal(t, []op{{"ResetField", "age", nil}, {"SetField", "age", 5}}, rec.ops)

	rec2 := &recorder{}
	require.NoError(t, entfield.Apply(rec2, age.Add(3)))
	require.Equal(t, []op{{"AddField", "age", 3}}, rec2.ops)
}

func TestEnumSetConvertsToString(t *testing.T) {
	type Status string
	rec := &recorder{}
	status := entfield.NewEnum[Status]("status")
	require.NoError(t, entfield.Apply(rec, status.Set(Status("active"))))
	require.Equal(t, []op{{"SetField", "status", "active"}}, rec.ops)
}

func TestJSONAppendAndSet(t *testing.T) {
	rec := &recorder{}
	tags := entfield.NewJSON[[]string]("tags")
	require.NoError(t, entfield.Apply(rec, tags.Set([]string{"a"}), tags.Append([]string{"b"}), tags.Clear()))
	require.Equal(t, []op{
		{"SetField", "tags", []string{"a"}},
		{"AppendField", "tags", []string{"b"}},
		{"ClearField", "tags", nil},
	}, rec.ops)
}

func TestEdgeFieldRoutesThroughEdge(t *testing.T) {
	rec := &recorder{}
	owner := entfield.NewEdgeField[int]("owner_id", "owner")
	require.NoError(t, entfield.Apply(rec, owner.Set(7), owner.SetNillable(nil), owner.Clear()))
	require.Equal(t, []op{{"SetEdgeID", "owner", 7}, {"ClearEdge", "owner", nil}}, rec.ops)

	// Predicates still hit the column, not the edge.
	q, args := render(t, owner.EQ(7))
	require.Contains(t, q, `"owner_id" = $1`)
	require.Equal(t, []any{7}, args)
}

func TestEdgeAssignments(t *testing.T) {
	rec := &recorder{}
	e := entfield.NewEdge[entfield.P, int]("pets", petsStep)
	require.NoError(t, entfield.Apply(rec,
		e.SetID(1),
		e.SetNillableID(nil),
		e.AddIDs(2, 3),
		e.RemoveIDs(4),
		e.Clear(),
	))
	require.Equal(t, []op{
		{"SetEdgeID", "pets", 1},
		{"AddEdgeIDs", "pets", []any{2, 3}},
		{"RemoveEdgeIDs", "pets", []any{4}},
		{"ClearEdge", "pets", nil},
	}, rec.ops)
}

func TestApplyStopsOnFirstError(t *testing.T) {
	boom := errors.New("boom")
	fake := &errFake{recorder: &recorder{}, failOn: "bad", err: boom}
	name := entfield.NewString[string]("name")
	bad := entfield.NewString[string]("bad")
	err := entfield.Apply(fake, name.Set("a"), bad.Set("x"), name.Set("z"))
	require.Equal(t, boom, err)
	require.Equal(t, []op{{"SetField", "name", "a"}}, fake.recorder.ops)
}

// TestScanHandleSetUsesRawValue pins that Set on a Scan handle passes the RAW
// Go value to the mutation, not the scan-func-encoded form: the old setter
// codegen passed raw values, and scanning happened later at spec-build time.
func TestScanHandleSetUsesRawValue(t *testing.T) {
	rec := &recorder{}
	slug := entfield.NewStringScan[string]("slug", func(v string) (driver.Value, error) { return "s:" + v, nil })
	require.NoError(t, entfield.Apply(rec, slug.Set("hi")))
	require.Equal(t, []op{{"SetField", "slug", "hi"}}, rec.ops)
}
