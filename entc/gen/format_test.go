// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/imports"
)

func TestPruneUnusedImports(t *testing.T) {
	src := `package p

import (
	"fmt"
	"strings"
	u "github.com/google/uuid"
	_ "embed"
	. "math"
)

func f() string { return fmt.Sprint(u.Nil, Pi) }
`
	out, ok := pruneUnusedImports("p.go", []byte(src), nil)
	require.True(t, ok)
	s := string(out)
	require.NotContains(t, s, `"strings"`)
	require.Contains(t, s, `"fmt"`)
	require.Contains(t, s, `u "github.com/google/uuid"`)
	require.Contains(t, s, `_ "embed"`)
	require.Contains(t, s, `. "math"`)
}

func TestPruneUnusedImportsMissingQualifier(t *testing.T) {
	_, ok := pruneUnusedImports("p.go", []byte("package p\n\nfunc f() { fmt.Println() }\n"), nil)
	require.False(t, ok)
}

func TestPruneUnusedImportsShadowedQualifier(t *testing.T) {
	src := "package p\n\nimport \"strings\"\n\ntype T struct{ Builder int }\n\nfunc f(strings T) int { return strings.Builder }\n"
	out, ok := pruneUnusedImports("p.go", []byte(src), nil)
	require.True(t, ok)
	require.NotContains(t, string(out), `"strings"`)
}

func TestFormatSourceMatchesGoimports(t *testing.T) {
	src := "package p\n\nimport (\n\t\"strings\"\n\t\"github.com/google/uuid\"\n\t\"fmt\"\n)\n\nfunc f() string { return fmt.Sprint(uuid.Nil) }\n"
	want, err := imports.Process("p.go", []byte(src), nil)
	require.NoError(t, err)
	got, err := FormatSource("p.go", []byte(src), nil)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}

func TestAssumedPackageName(t *testing.T) {
	for in, want := range map[string]string{
		"fmt":                               "fmt",
		"database/sql/driver":               "driver",
		"github.com/lib/pq":                 "pq",
		"github.com/lestrrat-go/jwx/v2/jwt": "jwt",
		"github.com/jackc/pgx/v5":           "pgx",
		"gopkg.in/yaml.v3":                  "yaml",
		"github.com/foo/go-bar":             "bar",
	} {
		require.Equal(t, want, assumedPackageName(in), in)
	}
}

func TestPruneUnusedImportsSeveral(t *testing.T) {
	src := "package p\n\nimport (\n\t\"bytes\"\n\t\"fmt\"\n\t\"io\"\n\t\"os\"\n\t\"strings\"\n)\n\nfunc f() string { return fmt.Sprint(os.Args) }\n"
	out, ok := pruneUnusedImports("p.go", []byte(src), nil)
	require.True(t, ok)
	s := string(out)
	for _, gone := range []string{`"bytes"`, `"io"`, `"strings"`} {
		require.NotContains(t, s, gone)
	}
	require.Contains(t, s, `"fmt"`)
	require.Contains(t, s, `"os"`)
}

func TestPruneUnusedImportsSiblingValueQualifier(t *testing.T) {
	src := "package p\n\nimport \"fmt\"\n\nfunc f() string { return fmt.Sprint(Field.ID) }\n"
	_, ok := pruneUnusedImports("p.go", []byte(src), nil)
	require.False(t, ok)
	out, ok := pruneUnusedImports("p.go", []byte(src), map[string]bool{"Field": true})
	require.True(t, ok)
	require.Contains(t, string(out), `"fmt"`)
}

func TestPruneUnusedImportsUppercasePackageQualifier(t *testing.T) {
	src := "package p\n\nimport \"example.com/sdk\"\n\nfunc f() { SDK.Value() }\n"
	_, ok := pruneUnusedImports("p.go", []byte(src), nil)
	require.False(t, ok)
}

func TestPackageLevelNames(t *testing.T) {
	src := "package p\n\nimport \"fmt\"\n\nvar A, b = 1, 2\n\nconst C = 1\n\ntype T struct{}\n\nfunc F() { fmt.Println() }\n\nfunc (T) M() {}\n"
	require.Equal(t, []string{"A", "b", "C", "T", "F"}, PackageLevelNames("p.go", []byte(src)))
	require.Nil(t, PackageLevelNames("p.go", []byte("not go")))
}
