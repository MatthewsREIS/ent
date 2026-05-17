// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"

	"github.com/stretchr/testify/require"
)

func TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport(t *testing.T) {
	mod := writeTempModule(t, "gremlinregen")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package: "gremlinregen/ent",
		Target:  target,
		Storage: drivers[1],
	}, &load.Schema{
		Name: "Task",
		Fields: []*load.Field{
			{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
		},
	})
	require.NoError(t, err)
	require.Len(t, graph.Nodes, 1)

	// Force alias handling to reproduce the duplicate import regression.
	graph.Nodes[0].alias = "enttask"

	require.NoError(t, graph.Gen())

	// In the sub-package layout, the delete builder lives in task/delete.go
	// (inside the task package itself). A self-import of "gremlinregen/ent/task"
	// must not appear — the entity symbols are unqualified within the sub-package.
	path := filepath.Join(target, "task", "delete.go")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	file, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
	require.NoError(t, err)

	selfPath := "gremlinregen/ent/task"
	for _, imp := range file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)
		require.NotEqualf(t, selfPath, importPath,
			"task/delete.go must not self-import %q (file content:\n%s)", selfPath, content)
	}
}

func TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt(t *testing.T) {
	mod := writeTempModule(t, "useridregen")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package: "useridregen/ent",
		Target:  target,
		Storage: drivers[0],
	}, &load.Schema{
		Name: "User",
		Fields: []*load.Field{
			{Name: "id", Info: &field.TypeInfo{Type: field.TypeInt}},
		},
	})
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	// PR 6: userCreateDescriptor lives in the user sub-package now, so
	// the regression test moves alongside it instead of asserting from
	// the root ent package.
	testFile := filepath.Join(target, "user", "user_create_generated_id_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package user

import "testing"

func TestUserCreateDescriptorAssignGeneratedInt(t *testing.T) {
	node := &User{}
	if err := userCreateDescriptor.ID.AssignGenerated(node, int(7)); err != nil {
		t.Fatalf("assign generated int: %v", err)
	}
	if node.ID != 7 {
		t.Fatalf("unexpected id: %d", node.ID)
	}
}
`), 0644))

	cmd := exec.Command("go", "test", "-mod=mod", "./ent/user", "-run", "TestUserCreateDescriptorAssignGeneratedInt", "-count=1")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test output:\n%s", out)
}

// TestGraph_Gen_AssignGeneratedInt64IDNoDuplicateCase verifies that when a
// schema declares an explicit user-defined int64 id field, the rendered
// AssignGenerated type switch contains exactly one "case int64:" clause.
//
// Without the int64-aware guard in the template, the generic numeric
// fallback "case int64:" collides with the primary "case {{ $.ID.Type }}:"
// (which is also int64), producing "duplicate case int64 in type switch".
func TestGraph_Gen_AssignGeneratedInt64IDNoDuplicateCase(t *testing.T) {
	mod := writeTempModule(t, "int64idregen")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package: "int64idregen/ent",
		Target:  target,
		Storage: drivers[0],
	}, &load.Schema{
		Name: "RiverJob",
		Fields: []*load.Field{
			// User-defined id of type int64 — same shape as the gemini
			// consumer's riverjob entity that surfaced the bug.
			{Name: "id", Info: &field.TypeInfo{Type: field.TypeInt64}},
		},
	})
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	queryPath := filepath.Join(target, "riverjob", "query.go")
	content, err := os.ReadFile(queryPath)
	require.NoError(t, err)
	src := string(content)

	// Locate the AssignGenerated body and count "case int64:" occurrences
	// only within that switch statement (not the entire file).
	idx := strings.Index(src, "AssignGenerated: func(n *RiverJob")
	require.GreaterOrEqualf(t, idx, 0, "AssignGenerated body not found in %s:\n%s", queryPath, src)
	tail := src[idx:]
	end := strings.Index(tail, "\n\t\t},")
	require.GreaterOrEqual(t, end, 0)
	body := tail[:end]

	got := strings.Count(body, "case int64:")
	require.Equalf(t, 1, got,
		"expected exactly one `case int64:` clause in AssignGenerated, got %d. Body:\n%s",
		got, body)

	// Belt-and-suspenders: the generated file must actually compile.
	cmd := exec.Command("go", "build", "-mod=mod", "./...")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go build output:\n%s", out)
}

func TestGraph_Gen_SQLModifierDeleteBuilderHasModify(t *testing.T) {
	mod := writeTempModule(t, "deletemodifierregen")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package:  "deletemodifierregen/ent",
		Target:   target,
		Storage:  drivers[0],
		Features: []Feature{FeatureModifier},
	}, &load.Schema{
		Name: "Task",
		Fields: []*load.Field{
			{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
		},
	})
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	// In the sub-package layout, TaskDelete lives in ent/task/delete.go
	// (package "task"), not in the root ent package. Write the test there.
	testFile := filepath.Join(target, "task", "task_delete_modifier_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package task

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

func TestTaskDeleteHasModify(t *testing.T) {
	td := &TaskDelete{}
	_ = td.Modify(func(*sql.DeleteBuilder) {})
}
`), 0644))

	cmd := exec.Command("go", "test", "-mod=mod", "./ent/task", "-run", "TestTaskDeleteHasModify", "-count=1")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test output:\n%s", out)
}

func TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths(t *testing.T) {
	mod := writeTempModule(t, "schemaconfigregen")
	target := filepath.Join(mod, "ent")

	graph, err := NewGraph(&Config{
		Package:  "schemaconfigregen/ent",
		Target:   target,
		Storage:  drivers[0],
		Features: []Feature{FeatureSchemaConfig},
	}, &load.Schema{
		Name: "User",
		Fields: []*load.Field{
			{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
		},
		Edges: []*load.Edge{
			{Name: "groups", Type: "Group"},
		},
	}, &load.Schema{
		Name: "Group",
		Fields: []*load.Field{
			{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
		},
		Edges: []*load.Edge{
			{Name: "users", Type: "User", RefName: "groups", Inverse: true},
		},
	})
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	// PR 6: the M2M load (and the joinT.Schema hook on it) moved out of
	// user_query.go into the root facade's loadUserGroups in
	// user_facade.go. The sub-query parameter there is named `query`,
	// so the hook emits query.Config.SchemaConfig() (Config is shared
	// across builders in a client).
	userFacade, err := os.ReadFile(filepath.Join(target, "user_facade.go"))
	require.NoError(t, err)
	require.Contains(t, string(userFacade), "joinT.Schema(query.Config.SchemaConfig().UserGroups)")
	require.NotContains(t, string(userFacade), "edge.Schema = query.Config.SchemaConfig().UserGroups")

	groupUpdate, err := os.ReadFile(filepath.Join(target, "group", "update.go"))
	require.NoError(t, err)
	require.Truef(
		t,
		strings.Count(string(groupUpdate), "edge.Schema = _u.Config.SchemaConfig().UserGroups") >= 3,
		"expected schema hook to be applied for clear/remove/add edge mutations, got:\n%s",
		groupUpdate,
	)
	require.NotContains(t, string(groupUpdate), "edge.Schema = _u.schemaConfig.UserGroups")

	// PR 6: WithGroups moved from a method on *UserQuery to the
	// root-facade free function WithUserGroups; ClearUsers stays a
	// method on *GroupUpdate inside the group sub-package.
	testFile := filepath.Join(target, "schemaconfig_compile_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package ent

import (
	"testing"

	"schemaconfigregen/ent/group"
)

func TestGeneratedSchemaConfigDescriptorPathsCompile(t *testing.T) {
	_ = WithUserGroups
	_ = (*group.GroupUpdate).ClearUsers
}
`), 0644))

	cmd := exec.Command("go", "test", "-mod=mod", "./ent", "-run", "TestGeneratedSchemaConfigDescriptorPathsCompile", "-count=1")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test output:\n%s", out)
}

func writeTempModule(t *testing.T, module string) string {
	t.Helper()
	mod := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	goMod := fmt.Sprintf(`module %s

go 1.23

require entgo.io/ent v0.0.0

replace entgo.io/ent => %s
`, module, repoRoot)
	require.NoError(t, os.WriteFile(filepath.Join(mod, "go.mod"), []byte(goMod), 0644))
	return mod
}
