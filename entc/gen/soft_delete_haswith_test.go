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
