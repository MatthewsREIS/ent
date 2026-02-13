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

	path := filepath.Join(target, "task_delete.go")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	file, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
	require.NoError(t, err)

	wantPath := "gremlinregen/ent/task"
	var total, aliased int
	for _, imp := range file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)
		if importPath != wantPath {
			continue
		}
		total++
		if imp.Name != nil && imp.Name.Name == "enttask" {
			aliased++
		}
	}
	require.Equalf(t, 1, total, "unexpected imports in %s:\n%s", path, content)
	require.Equalf(t, 1, aliased, "unexpected imports in %s:\n%s", path, content)
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

	testFile := filepath.Join(target, "user_create_generated_id_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package ent

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

	cmd := exec.Command("go", "test", "-mod=mod", "./ent", "-run", "TestUserCreateDescriptorAssignGeneratedInt", "-count=1")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test output:\n%s", out)
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

	testFile := filepath.Join(target, "task_delete_modifier_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package ent

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

func TestTaskDeleteHasModify(t *testing.T) {
	td := &TaskDelete{}
	_ = td.Modify(func(*sql.DeleteBuilder) {})
}
`), 0644))

	cmd := exec.Command("go", "test", "-mod=mod", "./ent", "-run", "TestTaskDeleteHasModify", "-count=1")
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

	userQuery, err := os.ReadFile(filepath.Join(target, "user_query.go"))
	require.NoError(t, err)
	require.Contains(t, string(userQuery), "joinT.Schema(_q.schemaConfig.UserGroups)")

	groupUpdate, err := os.ReadFile(filepath.Join(target, "group_update.go"))
	require.NoError(t, err)
	require.Truef(
		t,
		strings.Count(string(groupUpdate), "edge.Schema = _u.schemaConfig.UserGroups") >= 3,
		"expected schema hook to be applied for clear/remove/add edge mutations, got:\n%s",
		groupUpdate,
	)
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
