package main

import (
	"fmt"
	"go/format"
	"testing"
)

// escrowManifest is the common manifest used by most cases: fields Name,
// ID, Status, Count and edge Parcels.
var escrowManifest = Manifest{
	"escrow": {
		Fields: map[string]bool{"Name": true, "ID": true, "Status": true, "Count": true},
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
			want:     wrap(escrowImport, fmt.Sprintf("escrow.F.Name.%s(%s)", o.op, o.args)),
		})
	}
	return cases
}

func TestRewriteSource(t *testing.T) {
	cases := []rewriteCase{
		{
			name:     "bare_equality",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.Name("x")`),
			want:     wrap(escrowImport, `escrow.F.Name.EQ("x")`),
		},
		{
			name:     "id_bare",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.ID("x")`),
			want:     wrap(escrowImport, `escrow.F.ID.EQ("x")`),
		},
		{
			name:     "id_op",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.IDGT(1)`),
			want:     wrap(escrowImport, `escrow.F.ID.GT(1)`),
		},
		{
			name:     "has_edge",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.HasParcels()`),
			want:     wrap(escrowImport, `escrow.E.Parcels.Has()`),
		},
		{
			name:     "has_edge_with",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.HasParcelsWith(p1, p2)`),
			want:     wrap(escrowImport, `escrow.E.Parcels.HasWith(p1, p2)`),
		},
		{
			name:     "by_field",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.ByStatus(asc)`),
			want:     wrap(escrowImport, `escrow.F.Status.Order(asc)`),
		},
		{
			name:     "by_edge",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.ByParcels(t1, t2)`),
			want:     wrap(escrowImport, `escrow.E.Parcels.OrderBy(t1, t2)`),
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
			want:  wrap(escrowImport, `escrow.F.ParcelsCount.Order(asc)`),
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
			want:  wrap(escrowImport, `escrow.E.Parcels.OrderByCount(asc)`),
		},
		{
			name:     "value_position_method_value",
			manifest: escrowManifest,
			input: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/wherehelpers\"\n",
				`wherehelpers.AppendPtr(escrow.NameEQ)`),
			want: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/wherehelpers\"\n",
				`wherehelpers.AppendPtr(escrow.F.Name.EQ)`),
		},
		{
			name: "aliased_import",
			manifest: Manifest{
				"escrow": escrowManifest["escrow"],
			},
			input: wrap("\tesc \"example.com/gen/escrow\"\n", `esc.NameEQ("x")`),
			want:  wrap("\tesc \"example.com/gen/escrow\"\n", `esc.F.Name.EQ("x")`),
		},
		{
			name:     "non_manifest_package_untouched",
			manifest: escrowManifest,
			input: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/gen/other\"\n",
				"escrow.NameEQ(\"x\")\n\tother.NameEQ(\"x\")"),
			want: wrap("\t\"example.com/gen/escrow\"\n\t\"example.com/gen/other\"\n",
				"escrow.F.Name.EQ(\"x\")\n\tother.NameEQ(\"x\")"),
		},
		{
			name:     "and_or_not_untouched_but_args_rewritten",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.And(escrow.NameEQ("a"), escrow.StatusEQ("b"))`),
			want:     wrap(escrowImport, `escrow.And(escrow.F.Name.EQ("a"), escrow.F.Status.EQ("b"))`),
		},
		{
			name:     "no_match_left_untouched",
			manifest: escrowManifest,
			input:    wrap(escrowImport, `escrow.Frobnicate("x")`),
			want:     wrap(escrowImport, `escrow.Frobnicate("x")`),
		},
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

			got, changed, err := RewriteSource(tc.name+".go", []byte(tc.input), tc.manifest)
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
	got, changed, err := RewriteSource("gen.go", []byte(src), escrowManifest)
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
	first, changed, err := RewriteSource("idem.go", []byte(wrap(escrowImport, `escrow.NameEQ("x")`)), escrowManifest)
	if err != nil {
		t.Fatalf("first RewriteSource error: %v", err)
	}
	if !changed {
		t.Fatalf("first pass should have changed the source")
	}

	second, changed, err := RewriteSource("idem.go", first, escrowManifest)
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
