// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplates_PR6SubPackageFormat(t *testing.T) {
	ty := &Type{Name: "Task"}
	want := map[string]string{
		"client/type":            "task/client.go",
		"query":                  "task/query.go",
		"mutation/type":          "task/mutation.go",
		"dialect/sql/entql/type": "task/entql.go",
	}
	got := map[string]string{}
	for _, tmpl := range Templates {
		if _, ok := want[tmpl.Name]; ok {
			require.True(t, tmpl.SubPackage, "template %q must be SubPackage", tmpl.Name)
			got[tmpl.Name] = tmpl.Format(ty)
		}
	}
	require.Equal(t, want, got)
}

func TestTemplates_PR6DeletedTemplates(t *testing.T) {
	want := []string{"%s_client.go", "%s_query.go", "%s_mutation.go", "%s_entql.go"}
	for _, p := range want {
		require.Contains(t, deletedTypeTemplates, p, "deletedTypeTemplates missing %q", p)
	}
}

func TestTemplates_FacadeRegistered(t *testing.T) {
	ty := &Type{Name: "Task"}
	for _, tmpl := range Templates {
		if tmpl.Name == "facade/type" {
			require.False(t, tmpl.SubPackage, "facade/type must NOT be SubPackage (lives at root)")
			require.Equal(t, "task_facade.go", tmpl.Format(ty))
			return
		}
	}
	t.Fatal("facade/type template not registered")
}
