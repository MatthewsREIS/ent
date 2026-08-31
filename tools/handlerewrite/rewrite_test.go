package main

import (
	"fmt"
	"go/format"
	"testing"
)

// escrowManifest is the common manifest used by most cases: fields Name,
// ID, Status and edge Parcels.
var escrowManifest = Manifest{
	"escrow": {
		Fields: map[string]bool{"Name": true, "ID": true, "Status": true},
		Edges:  map[string]bool{"Parcels": true},
	},
}

func wrap(imports, body string) string {
	return fmt.Sprintf("package p\n\nimport (\n%s)\n\nfunc f() {\n%s\n}\n", imports, body)
}

const escrowImport = "\t\"example.com/gen/escrow\"\n"

type rewriteCase struct {
	name     string
	manifest Manifest
	input    string
	want     string
}

func opSuffixCases() []rewriteCase {
	ops := []struct{ op, args string }{
		{"EQ", `"x"`}, {"NEQ", `"x"`}, {"In", `"x", "y"`}, {"NotIn", `"x", "y"`},
		{"GT", `1`}, {"GTE", `1`}, {"LT", `1`}, {"LTE", `1`},
		{"Contains", `"x"`}, {"HasPrefix", `"x"`}, {"HasSuffix", `"x"`},
		{"EqualFold", `"x"`}, {"ContainsFold", `"x"`},
		{"IsNil", ``}, {"NotNil", ``},
	}
	var cases []rewriteCase
	for _, o := range ops {
		cases = append(cases, rewriteCase{
			name:     "op_" + o.op,
			manifest: escrowManifest,
			input:    wrap(escrowImport, fmt.Sprintf("escrow.Name%s(%s)", o.op, o.args)),
			want:     wrap(escrowImport, fmt.Sprintf("escrow.Field.Name.%s(%s)", o.op, o.args)),
		})
	}
	return cases
}

// shadowingCase builds one file with four sibling top-level functions: a
// param, a ":=", and a range-var shadow of "escrow" (each must suppress
// rewriting within its own function only), plus a clean sibling function
// that still rewrites normally — proving shadow analysis is per-decl, not
// whole-file.
func shadowingCase() rewriteCase {
	input := `package p

import "example.com/gen/escrow"

func paramShadow(escrow *Escrow) {
	_ = escrow.NameEQ("x")
}

func defineShadow() {
	escrow, err := load()
	_ = err
	_ = escrow.NameEQ("x")
}

func rangeShadow(xs []int) {
	for _, escrow := range xs {
		_ = escrow.NameEQ("x")
	}
}

func clean() {
	_ = escrow.NameEQ("y")
}
`
	want := `package p

import "example.com/gen/escrow"

func paramShadow(escrow *Escrow) {
	_ = escrow.NameEQ("x")
}

func defineShadow() {
	escrow, err := load()
	_ = err
	_ = escrow.NameEQ("x")
}

func rangeShadow(xs []int) {
	for _, escrow := range xs {
		_ = escrow.NameEQ("x")
	}
}

func clean() {
	_ = escrow.Field.Name.EQ("y")
}
`
	return rewriteCase{name: "shadowing_is_per_function_and_conservative", manifest: escrowManifest, input: input, want: want}
}

func TestRewriteSource(t *testing.T) {
	cases := []rewriteCase{
		{
			name:     "bare_equality",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.Name("x")`),
			want:     wrap(escrowImport, `escrow.Field.Name.EQ("x")`),
		},
		{
			name:     "id_bare",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.ID("x")`),
			want:     wrap(escrowImport, `escrow.Field.ID.EQ("x")`),
		},
		{
			name:     "id_op",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.IDGT(1)`),
			want:     wrap(escrowImport, `escrow.Field.ID.GT(1)`),
		},
		{
			name:     "has_edge",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.HasParcels()`),
			want:     wrap(escrowImport, `escrow.Edge.Parcels.Has()`),
		},
		{
			name:     "has_edge_with",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.HasParcelsWith(p1, p2)`),
			want:     wrap(escrowImport, `escrow.Edge.Parcels.HasWith(p1, p2)`),
		},
		{
			name:     "by_field",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.ByStatus(asc)`),
			want:     wrap(escrowImport, `escrow.Field.Status.Order(asc)`),
		},
		{
			name:     "by_edge",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.ByParcels(t1, t2)`),
			want:     wrap(escrowImport, `escrow.Edge.Parcels.OrderBy(t1, t2)`),
		},
		{
			// Ambiguity direction 1: "ParcelsCount" is a field (edge
			// "Parcels" does not exist) -> By<Field>.
			name: "ambiguous_by_field_wins",
			manifest: Manifest{"escrow": {
				Fields: map[string]bool{"ParcelsCount": true},
				Edges:  map[string]bool{},
			}},
			input: wrap(escrowImport, `escrow.ByParcelsCount(asc)`),
			want:  wrap(escrowImport, `escrow.Field.ParcelsCount.Order(asc)`),
		},
		{
			// Ambiguity direction 2: "Parcels" is an edge (field
			// "ParcelsCount" does not exist) -> By<Edge>Count.
			name: "ambiguous_by_edge_count_wins",
			manifest: Manifest{"escrow": {
				Fields: map[string]bool{},
				Edges:  map[string]bool{"Parcels": true},
			}},
			input: wrap(escrowImport, `escrow.ByParcelsCount(asc)`),
			want:  wrap(escrowImport, `escrow.Edge.Parcels.OrderByCount(asc)`),
		},
		{
			// True ambiguity (not the direction-dependent cases above):
			// both field "ContactListContactsCount" and edge
			// "ContactListContacts" exist, so "ByContactListContactsCount"
			// reads equally well as By<Field> or By<Edge>Count. Refuse to
			// guess; leave the selector untouched (it'll surface as a
			// compile error rather than silently picking one reading).
			name: "ambiguous_field_and_edge_both_match_refused",
			manifest: Manifest{"escrow": {
				Fields: map[string]bool{"ContactListContactsCount": true},
				Edges:  map[string]bool{"ContactListContacts": true},
			}},
			input: wrap(escrowImport, `escrow.ByContactListContactsCount(asc)`),
			want:  wrap(escrowImport, `escrow.ByContactListContactsCount(asc)`),
		},
		{
			// Control: same manifest as above, but a selector with no
			// competing reading still rewrites normally.
			name: "ambiguous_manifest_no_collision_still_rewrites",
			manifest: Manifest{"escrow": {
				Fields: map[string]bool{"ContactListContactsCount": true},
				Edges:  map[string]bool{"ContactListContacts": true},
			}},
			input: wrap(escrowImport, `escrow.HasContactListContacts()`),
			want:  wrap(escrowImport, `escrow.Edge.ContactListContacts.Has()`),
		},
		{
			name:     "value_position_method_value",
			manifest: escrowManifest,
			input: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/wherehelpers\"\n",
				`wherehelpers.AppendPtr(escrow.NameEQ)`),
			want: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/wherehelpers\"\n",
				`wherehelpers.AppendPtr(escrow.Field.Name.EQ)`),
		},
		{
			name: "aliased_import",
			manifest: Manifest{
				"escrow": escrowManifest["escrow"],
			},
			input: wrap("\tesc \"example.com/gen/escrow\"\n", `esc.NameEQ("x")`),
			want:  wrap("\tesc \"example.com/gen/escrow\"\n", `esc.Field.Name.EQ("x")`),
		},
		{
			name:     "non_manifest_package_untouched",
			manifest: escrowManifest,
			input: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/gen/other\"\n",
				"escrow.NameEQ(\"x\")\n\tother.NameEQ(\"x\")"),
			want: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/gen/other\"\n",
				"escrow.Field.Name.EQ(\"x\")\n\tother.NameEQ(\"x\")"),
		},
		{
			name:     "and_or_not_untouched_but_args_rewritten",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.And(escrow.NameEQ("a"), escrow.StatusEQ("b"))`),
			want:     wrap(escrowImport, `escrow.And(escrow.Field.Name.EQ("a"), escrow.Field.Status.EQ("b"))`),
		},
		{
			name:     "no_match_left_untouched",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.Frobnicate("x")`),
			want:     wrap(escrowImport, `escrow.Frobnicate("x")`),
		},
		{
			// Only a top-level selector whose X is a bare *ast.Ident
			// resolving to an import qualifies; a selector chain
			// (method/field access through another value) is untouched.
			name:     "method_chain_not_rewritten",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `x.escrow.NameEQ("x")`),
			want:     wrap(escrowImport, `x.escrow.NameEQ("x")`),
		},
		{
			// A dot import ". \"...\"" binds no local qualifier identifier,
			// so a bare "escrow.NameEQ(...)" selector (using the package's
			// base name as a literal identifier, not a real qualifier) has
			// nothing to resolve against and is left untouched.
			name:     "dot_import_excluded",
			manifest: escrowManifest,
			input:    wrap("\t. \"example.com/gen/escrow\"\n", `escrow.NameEQ("x")`),
			want:     wrap("\t. \"example.com/gen/escrow\"\n", `escrow.NameEQ("x")`),
		},
		shadowingCase(),
	}
	cases = append(cases, opSuffixCases()...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantFormatted, err := format.Source([]byte(tc.want))
			if err != nil {
				t.Fatalf("want does not gofmt: %v\n%s", err, tc.want)
			}
			inputFormatted, err := format.Source([]byte(tc.input))
			if err != nil {
				t.Fatalf("input does not gofmt: %v\n%s", err, tc.input)
			}
			wantChanged := string(inputFormatted) != string(wantFormatted)

			got, changed, err := RewriteSource(tc.name+".go", []byte(tc.input), tc.manifest, nil)
			if err != nil {
				t.Fatalf("RewriteSource error: %v", err)
			}
			if changed != wantChanged {
				t.Errorf("changed = %v, want %v", changed, wantChanged)
			}
			// Unchanged files are returned byte-for-byte as given (to
			// preserve original formatting); only a real rewrite goes
			// through gofmt-canonical output.
			wantBytes := wantFormatted
			if !wantChanged {
				wantBytes = []byte(tc.input)
			}
			if string(got) != string(wantBytes) {
				t.Errorf("got:\n%s\nwant:\n%s", got, wantBytes)
			}
		})
	}
}

func TestGeneratedFileSkipped(t *testing.T) {
	src := "// Code generated by entc, DO NOT EDIT.\n\n" + wrap(escrowImport, `escrow.NameEQ("x")`)
	got, changed, err := RewriteSource("gen.go", []byte(src), escrowManifest, nil)
	if err != nil {
		t.Fatalf("RewriteSource error: %v", err)
	}
	if changed {
		t.Errorf("changed = true, want false for generated file")
	}
	if string(got) != src {
		t.Errorf("generated file content modified:\ngot:\n%s\nwant (unchanged):\n%s", got, src)
	}
}

func TestIdempotent(t *testing.T) {
	first, changed, err := RewriteSource("idem.go", []byte(wrap(escrowImport, `escrow.NameEQ("x")`)), escrowManifest, nil)
	if err != nil {
		t.Fatalf("first RewriteSource error: %v", err)
	}
	if !changed {
		t.Fatalf("first pass should have changed the source")
	}

	second, changed, err := RewriteSource("idem.go", first, escrowManifest, nil)
	if err != nil {
		t.Fatalf("second RewriteSource error: %v", err)
	}
	if changed {
		t.Errorf("second pass reported changed = true, want false (idempotency)")
	}
	if string(second) != string(first) {
		t.Errorf("second pass altered output:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestPkgPrefixCollision covers the -pkgprefix guard: two different import
// paths share the base name "escrow" (a real entity package under
// example.com/gen/, and an unrelated same-basename package under
// example.com/other/). Without a prefix restriction both resolve by base
// name alone (the documented permissive default); with one, only the import
// path that actually starts with an allowed prefix resolves.
func TestPkgPrefixCollision(t *testing.T) {
	realImport := wrap("\t\"example.com/gen/escrow\"\n", `escrow.NameEQ("x")`)
	otherImport := wrap("\t\"example.com/other/escrow\"\n", `escrow.NameEQ("x")`)
	wantRewritten := wrap("\t\"example.com/gen/escrow\"\n", `escrow.Field.Name.EQ("x")`)
	otherRewritten := wrap("\t\"example.com/other/escrow\"\n", `escrow.Field.Name.EQ("x")`)

	t.Run("no_prefix_both_resolve_by_basename", func(t *testing.T) {
		got, changed, err := RewriteSource("real.go", []byte(realImport), escrowManifest, nil)
		if err != nil || !changed || string(got) != mustFormat(t, wantRewritten) {
			t.Fatalf("real import: changed=%v err=%v got=%s", changed, err, got)
		}
		got, changed, err = RewriteSource("other.go", []byte(otherImport), escrowManifest, nil)
		if err != nil || !changed || string(got) != mustFormat(t, otherRewritten) {
			t.Fatalf("other import (no prefix given): changed=%v err=%v got=%s", changed, err, got)
		}
	})

	t.Run("prefix_restricts_to_matching_path", func(t *testing.T) {
		prefixes := []string{"example.com/gen/"}
		got, changed, err := RewriteSource("real.go", []byte(realImport), escrowManifest, prefixes)
		if err != nil || !changed || string(got) != mustFormat(t, wantRewritten) {
			t.Fatalf("real import: changed=%v err=%v got=%s", changed, err, got)
		}
		got, changed, err = RewriteSource("other.go", []byte(otherImport), escrowManifest, prefixes)
		if err != nil || changed || string(got) != otherImport {
			t.Fatalf("other import (prefix given, should be untouched): changed=%v err=%v got=%s", changed, err, got)
		}
	})
}

func mustFormat(t *testing.T, src string) string {
	t.Helper()
	out, err := format.Source([]byte(src))
	if err != nil {
		t.Fatalf("does not gofmt: %v\n%s", err, src)
	}
	return string(out)
}
