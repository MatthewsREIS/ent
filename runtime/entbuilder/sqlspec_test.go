// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"reflect"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/runtime/entbuilder"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

// thingEnt is the fixture entity type for sqlspec tests.
type thingEnt struct {
	ID      int
	Count   int
	Note    string
	Tags    []string // JSON list field, for the Append path.
	OwnerID int      // OwnFK node-field-assignment target for the "owner" edge.
	Coupon  *string  // NillableValue node-field-assignment target.
	Code    string   // HasValueScanner field's node-field-assignment target.
}

func thingDescriptor() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name: "Thing", IDType: reflect.TypeFor[int](),
		Table: "things", IDColumn: "id", IDSQLType: field.TypeInt,
		TableColumns: []string{"id", "count", "note_col", "labels"},
		Fields: map[string]entbuilder.FieldSpec{
			"count":  {Type: reflect.TypeFor[int](), GoName: "Count", Numeric: true, Column: "count", SQLType: field.TypeInt},
			"note":   {Type: reflect.TypeFor[string](), GoName: "Note", Nillable: true, Column: "note_col", SQLType: field.TypeString},
			"labels": {Type: reflect.TypeFor[[]string](), GoName: "Tags", Nillable: true, Column: "labels", SQLType: field.TypeJSON},
			"coupon": {Type: reflect.TypeFor[string](), GoName: "Coupon", Nillable: true, Column: "coupon", SQLType: field.TypeString},
			"code":   {Type: reflect.TypeFor[string](), GoName: "Code", Column: "code", SQLType: field.TypeString, HasValueScanner: true},
		},
		Edges: map[string]entbuilder.EdgeSpec{
			"owner": {Cardinality: entbuilder.O2OUnique, Target: "User", TargetIDType: reflect.TypeFor[int](),
				Rel: sqlgraph.M2O, StorageTable: "things", StorageColumns: []string{"thing_owner"},
				TargetIDColumn: "id", TargetIDSQLType: field.TypeInt, SchemaKey: "User", NodeField: "OwnerID"},
			"tags": {Cardinality: entbuilder.M2M, Target: "Tag", TargetIDType: reflect.TypeFor[int](),
				Rel: sqlgraph.M2M, StorageTable: "thing_tags", StorageColumns: []string{"thing_id", "tag_id"},
				TargetIDColumn: "id", TargetIDSQLType: field.TypeInt},
		},
		SchemaKey: "Thing",
	}
}

// TestApplyUpdateSpec covers the shape recorded in Step 1 of the task brief
// (entc/integration/ent/user/update.go:220-460): independent Set/Add/Clear
// per field, and independent EdgeCleared/RemovedEdgeIDs(guarded)/EdgeIDs
// checks per edge — NOT an implicit Clear+Add pairing for a "replaced"
// unique edge. SetEdgeID alone (no explicit ClearEdge call) only appends to
// spec.Edges.Add, mirroring the "card" edge in the referenced update.go:
// its EdgeCleared("card") and EdgeIDs("card") checks are two independent
// `if` blocks (lines 301-329), and the generated entfield EdgeField[T].Set
// (runtime/entfield/assign.go) never calls ClearEdge itself.
func TestApplyUpdateSpec(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpUpdateOne, desc)
	require.NoError(t, m.SetField("count", 3))
	require.NoError(t, m.AddField("count", 2))
	require.NoError(t, m.ClearField("note"))
	require.NoError(t, m.SetEdgeID("owner", 7))
	require.NoError(t, m.RemoveEdgeIDs("tags", 1, 2))

	spec := sqlgraph.NewUpdateSpec("things", []string{"id", "count", "note_col"},
		sqlgraph.NewFieldSpec("id", field.TypeInt))
	ApplyUpdateSpecNoSchema(m, spec)

	// Fields: count=3 (SetField, update.go:232-234), count+=2 (AddField,
	// update.go:235-238), note_col cleared (ClearField, update.go:264-266).
	require.Len(t, spec.Fields.Set, 1)
	require.Equal(t, "count", spec.Fields.Set[0].Column)
	require.Equal(t, 3, spec.Fields.Set[0].Value)
	require.Len(t, spec.Fields.Add, 1)
	require.Equal(t, "count", spec.Fields.Add[0].Column)
	require.Equal(t, 2, spec.Fields.Add[0].Value)
	require.Len(t, spec.Fields.Clear, 1)
	require.Equal(t, "note_col", spec.Fields.Clear[0].Column)

	// Edges: owner was only Set (never explicitly cleared) => Add only,
	// matching the independent "if EdgeCleared"/"if EdgeIDs" blocks at
	// update.go:301-329 (no implicit Clear+Add pairing).
	require.Len(t, spec.Edges.Add, 1)
	require.Equal(t, "things", spec.Edges.Add[0].Table)
	require.Equal(t, []string{"thing_owner"}, spec.Edges.Add[0].Columns)
	require.Len(t, spec.Edges.Add[0].Target.Nodes, 1)
	require.Equal(t, 7, spec.Edges.Add[0].Target.Nodes[0])

	// tags: RemoveEdgeIDs(1, 2) with no ClearEdge call => appended to
	// spec.Edges.Clear via the "!EdgeCleared" guarded branch
	// (update.go:343-358), not spec.Edges.Add.
	require.Len(t, spec.Edges.Clear, 1)
	require.Equal(t, "thing_tags", spec.Edges.Clear[0].Table)
	require.ElementsMatch(t, []any{1, 2}, spec.Edges.Clear[0].Target.Nodes)
}

// TestApplyUpdateSpec_ExplicitClearThenSet documents the case the brief's
// draft assertion anticipated (Clear+Add pair) — it only occurs when the
// caller explicitly calls ClearEdge in addition to SetEdgeID, e.g. via
// F.Owner.Clear() then F.Owner.Set(v) (or an update-builder ClearOwner()
// followed by SetOwnerID()), not implicitly on every Set.
func TestApplyUpdateSpec_ExplicitClearThenSet(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpUpdateOne, desc)
	require.NoError(t, m.ClearEdge("owner"))
	require.NoError(t, m.SetEdgeID("owner", 9))

	spec := sqlgraph.NewUpdateSpec("things", []string{"id"}, sqlgraph.NewFieldSpec("id", field.TypeInt))
	ApplyUpdateSpecNoSchema(m, spec)

	require.Len(t, spec.Edges.Clear, 1)
	require.Len(t, spec.Edges.Add, 1)
	require.Len(t, spec.Edges.Add[0].Target.Nodes, 1)
	require.Equal(t, 9, spec.Edges.Add[0].Target.Nodes[0])
}

// TestApplyUpdateSpec_AppendedJSON reproduces the sqljson.Append shape used
// for JSON list fields (see entc/integration/ent/fieldtype/update.go:628-632):
// an AddModifier wrapping sqljson.Append, independent of Set/Clear.
func TestApplyUpdateSpec_AppendedJSON(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpUpdateOne, desc)
	require.NoError(t, m.AppendField("labels", []string{"a", "b"}))

	spec := sqlgraph.NewUpdateSpec("things", []string{"id"}, sqlgraph.NewFieldSpec("id", field.TypeInt))
	ApplyUpdateSpecNoSchema(m, spec)

	require.Empty(t, spec.Fields.Set)
	require.Len(t, spec.Modifiers, 1)
}

// TestApplyUpdateSpec_HasValueScanner covers FieldSpec.HasValueScanner: the
// applier must not call spec.SetField for such a field, leaving it to the
// template's residual per-field block (which converts via the field's
// ValueFunc first) — see the doc comment on FieldSpec.HasValueScanner.
func TestApplyUpdateSpec_HasValueScanner(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpUpdateOne, desc)
	require.NoError(t, m.SetField("code", "xyz"))

	spec := sqlgraph.NewUpdateSpec("things", []string{"id"}, sqlgraph.NewFieldSpec("id", field.TypeInt))
	ApplyUpdateSpecNoSchema(m, spec)

	require.Empty(t, spec.Fields.Set)
}

// TestApplyUpdateSpec_SchemaOf covers multischema resolution: Node.Schema
// from Descriptor.SchemaKey, and each edge's Schema from EdgeSpec.SchemaKey
// (only "owner" declares one), matching multischema/ent/user/update.go's
// `edge.Schema = _u.Config.SchemaConfig().Pet` shape (per-edge, independent
// of Clear/Add).
func TestApplyUpdateSpec_SchemaOf(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpUpdateOne, desc)
	require.NoError(t, m.SetEdgeID("owner", 1))
	require.NoError(t, m.AddEdgeIDs("tags", 5))

	spec := sqlgraph.NewUpdateSpec("things", []string{"id"}, sqlgraph.NewFieldSpec("id", field.TypeInt))
	schemaOf := func(key string) string { return "alt_" + key }
	entbuilder.ApplyUpdateSpec(m, spec, schemaOf)

	require.Equal(t, "alt_Thing", spec.Node.Schema)
	require.Len(t, spec.Edges.Add, 2)
	for _, e := range spec.Edges.Add {
		switch e.Table {
		case "things":
			require.Equal(t, "alt_User", e.Schema)
		case "thing_tags":
			require.Empty(t, e.Schema) // "tags" has no SchemaKey.
		default:
			t.Fatalf("unexpected edge table %q", e.Table)
		}
	}
}

// TestApplyCreateSpec covers createSpec()'s shape (see
// entc/integration/ent/user/create.go:153-384): SetField per set field,
// and one Add-only edge append per populated edge (no Clear/Remove on
// create).
func TestApplyCreateSpec(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpCreate, desc)
	require.NoError(t, m.SetField("count", 5))
	require.NoError(t, m.SetEdgeID("owner", 3))
	require.NoError(t, m.AddEdgeIDs("tags", 10, 11))

	spec := sqlgraph.NewCreateSpec("things", sqlgraph.NewFieldSpec("id", field.TypeInt))
	node := &thingEnt{}
	entbuilder.ApplyCreateSpec(m, node, spec, nil)

	require.Len(t, spec.Fields, 1)
	require.Equal(t, "count", spec.Fields[0].Column)
	require.Equal(t, 5, spec.Fields[0].Value)

	require.Len(t, spec.Edges, 2)
	for _, e := range spec.Edges {
		switch e.Table {
		case "things":
			require.Len(t, e.Target.Nodes, 1)
			require.Equal(t, 3, e.Target.Nodes[0])
		case "thing_tags":
			require.ElementsMatch(t, []any{10, 11}, e.Target.Nodes)
		default:
			t.Fatalf("unexpected edge table %q", e.Table)
		}
	}

	// Fields: node.<GoName> is always assigned the raw mutation value,
	// matching createSpec()'s `_node.Count = value` unroll.
	require.Equal(t, 5, node.Count)
	// Edges: an OwnFK edge (NodeField != "") also assigns node.<NodeField>
	// = nodes[0], matching the `$e.OwnFK` block (e.g.
	// entc/integration/edgefield/ent/pet/create.go's `_node.OwnerID =
	// nodes[0]`). "tags" has no NodeField (M2M, doesn't own an FK) so it's
	// unaffected.
	require.Equal(t, 3, node.OwnerID)
}

// TestApplyCreateSpec_NodeFieldPointerWrap covers the NillableValue case:
// the mutation value is a plain string, but the entity struct field is
// *string, so ApplyCreateSpec must wrap it in a pointer — matching
// createSpec()'s `_node.LicensedAt = &value` unroll (see
// entc/integration/template/ent/pet/create.go:115).
func TestApplyCreateSpec_NodeFieldPointerWrap(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpCreate, desc)
	require.NoError(t, m.SetField("coupon", "SAVE10"))

	spec := sqlgraph.NewCreateSpec("things", sqlgraph.NewFieldSpec("id", field.TypeInt))
	node := &thingEnt{}
	entbuilder.ApplyCreateSpec(m, node, spec, nil)

	require.NotNil(t, node.Coupon)
	require.Equal(t, "SAVE10", *node.Coupon)
}

// TestApplyCreateSpec_HasValueScanner covers FieldSpec.HasValueScanner:
// ApplyCreateSpec must NOT call spec.SetField for such a field (the
// generated createSpec() keeps a residual block that does the ValueFunc
// conversion + SetField itself — calling SetField twice for the same
// column would emit a broken duplicate SQL assignment), but node
// population still uses the raw mutation value unconditionally, matching
// createSpec()'s `_node.StructField = value` line which runs regardless of
// $f.HasValueScanner.
func TestApplyCreateSpec_HasValueScanner(t *testing.T) {
	desc := thingDescriptor()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpCreate, desc)
	require.NoError(t, m.SetField("code", "abc"))

	spec := sqlgraph.NewCreateSpec("things", sqlgraph.NewFieldSpec("id", field.TypeInt))
	node := &thingEnt{}
	entbuilder.ApplyCreateSpec(m, node, spec, nil)

	require.Empty(t, spec.Fields)
	require.Equal(t, "abc", node.Code)
}

// ApplyUpdateSpecNoSchema is a thin wrapper so tests that don't exercise
// multischema resolution don't need to pass a nil func literal at every
// call site.
func ApplyUpdateSpecNoSchema(m *entbuilder.Mutation[thingEnt, int], spec *sqlgraph.UpdateSpec) {
	entbuilder.ApplyUpdateSpec(m, spec, nil)
}
