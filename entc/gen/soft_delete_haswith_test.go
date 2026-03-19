package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"

	"github.com/stretchr/testify/require"
)

// TestGraph_Gen_HasWithSoftDeleteFilter verifies that the HasXWith predicate template
// auto-injects sql.FieldIsNull("deleted_at") inside the HasNeighborsWith closure when
// the edge target type has a deleted_at field, and does NOT inject it otherwise.
func TestGraph_Gen_HasWithSoftDeleteFilter(t *testing.T) {
	mod := writeTempModule(t, "softdeletehaswith")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package: "softdeletehaswith/ent",
		Target:  target,
		Storage: drivers[0],
	}, &load.Schema{
		Name: "Parent",
		Fields: []*load.Field{
			{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
		},
		Edges: []*load.Edge{
			{Name: "children", Type: "Child"},
		},
	}, &load.Schema{
		Name: "Child",
		Fields: []*load.Field{
			{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			{Name: "deleted_at", Info: &field.TypeInfo{Type: field.TypeTime, Nillable: true}, Optional: true, Nillable: true},
		},
		Edges: []*load.Edge{
			{Name: "parent", Type: "Parent", RefName: "children", Unique: true, Inverse: true},
		},
	})
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	// Parent -> Child edge: Child has deleted_at, so HasChildrenWith should inject soft-delete filter.
	parentWhere, err := os.ReadFile(filepath.Join(target, "parent", "where.go"))
	require.NoError(t, err)
	require.Contains(t, string(parentWhere), `sql.FieldIsNull("deleted_at")(s)`,
		"HasChildrenWith should auto-inject soft-delete filter because Child has deleted_at field")

	// Child -> Parent edge: Parent does NOT have deleted_at, so HasParentWith should NOT inject filter.
	childWhere, err := os.ReadFile(filepath.Join(target, "child", "where.go"))
	require.NoError(t, err)
	require.NotContains(t, string(childWhere), `sql.FieldIsNull("deleted_at")(s)`,
		"HasParentWith should NOT inject soft-delete filter because Parent has no deleted_at field")

	// Verify generated code compiles.
	testFile := filepath.Join(target, "softdelete_compile_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package ent

import "testing"

func TestSoftDeleteHasWithCompiles(t *testing.T) {
	_ = (*ParentQuery).WithChildren
	_ = (*ChildQuery).WithParent
}
`), 0644))

	cmd := exec.Command("go", "test", "-mod=mod", "./ent", "-run", "TestSoftDeleteHasWithCompiles", "-count=1")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test output:\n%s", out)
}

// TestGraph_Gen_HasWithSoftDeleteFilter_ThroughEdge verifies that when a through-edge
// junction entity has a deleted_at field, the generated HasX and HasXWith predicates
// inject step.EdgePredicates to filter soft-deleted junction rows.
func TestGraph_Gen_HasWithSoftDeleteFilter_ThroughEdge(t *testing.T) {
	mod := writeTempModule(t, "softdeletethroughedge")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package: "softdeletethroughedge/ent",
		Target:  target,
		Storage: drivers[0],
	},
		// Group schema — no deleted_at
		&load.Schema{
			Name: "Group",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
			Edges: []*load.Edge{
				{Name: "users", Type: "User", Through: &struct{ N, T string }{N: "memberships", T: "Membership"}},
			},
		},
		// User schema — no deleted_at
		&load.Schema{
			Name: "User",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
			Edges: []*load.Edge{
				{Name: "groups", Type: "Group", RefName: "users", Inverse: true, Through: &struct{ N, T string }{N: "memberships", T: "Membership"}},
			},
		},
		// Membership (junction) schema — has deleted_at for soft delete
		&load.Schema{
			Name: "Membership",
			Fields: []*load.Field{
				{Name: "group_id", Info: &field.TypeInfo{Type: field.TypeInt}},
				{Name: "user_id", Info: &field.TypeInfo{Type: field.TypeInt}},
				{Name: "deleted_at", Info: &field.TypeInfo{Type: field.TypeTime, Nillable: true}, Optional: true, Nillable: true},
			},
			Edges: []*load.Edge{
				{Name: "group", Type: "Group", Ref: &load.Edge{Name: "users"}, Field: "group_id", Unique: true, Required: true},
				{Name: "user", Type: "User", Ref: &load.Edge{Name: "groups"}, Field: "user_id", Unique: true, Required: true},
			},
		},
	)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	// Group -> User through Membership: Membership has deleted_at,
	// so both HasUsers and HasUsersWith should inject EdgePredicates.
	groupWhere, err := os.ReadFile(filepath.Join(target, "group", "where.go"))
	require.NoError(t, err)
	groupWhereStr := string(groupWhere)
	require.Contains(t, groupWhereStr, `step.EdgePredicates = append(step.EdgePredicates`,
		"Group's HasUsers/HasUsersWith should inject EdgePredicates for soft-deleted junction rows")

	// User -> Group through Membership (inverse): same junction, should also get EdgePredicates.
	userWhere, err := os.ReadFile(filepath.Join(target, "user", "where.go"))
	require.NoError(t, err)
	userWhereStr := string(userWhere)
	require.Contains(t, userWhereStr, `step.EdgePredicates = append(step.EdgePredicates`,
		"User's HasGroups/HasGroupsWith should inject EdgePredicates for soft-deleted junction rows (inverse)")

	// Verify that HasMembershipsWith (targeting Membership which has deleted_at) correctly
	// gets the target-entity soft-delete filter too.
	require.Contains(t, groupWhereStr, `sql.FieldIsNull("deleted_at")(s)`,
		"Group's HasMembershipsWith should inject target-entity soft-delete filter because Membership has deleted_at")
	require.Contains(t, userWhereStr, `sql.FieldIsNull("deleted_at")(s)`,
		"User's HasMembershipsWith should inject target-entity soft-delete filter because Membership has deleted_at")

	// Verify generated code compiles.
	testFile := filepath.Join(target, "throughedge_compile_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package ent

import "testing"

func TestThroughEdgeSoftDeleteCompiles(t *testing.T) {
	_ = (*GroupQuery).WithUsers
	_ = (*UserQuery).WithGroups
	_ = (*MembershipQuery).WithGroup
	_ = (*MembershipQuery).WithUser
}
`), 0644))

	cmd := exec.Command("go", "test", "-mod=mod", "./ent", "-run", "TestThroughEdgeSoftDeleteCompiles", "-count=1")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test output:\n%s", out)
}
