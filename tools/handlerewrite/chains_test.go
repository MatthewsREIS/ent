package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyFixture copies testdata/chainsmod into a fresh temp dir — each test
// gets its own copy since ProcessPackages rewrites files in place — and
// returns the temp dir's path.
func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir("testdata/chainsmod", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("testdata/chainsmod", p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

// runChains runs ProcessPackages over a fresh copy of the fixture module
// and returns the (possibly rewritten) content of usage/<name> plus the
// list of changed files, relative to the copy's root, slash-separated.
func runChains(t *testing.T, manifest Manifest, prefixes []string, name string) (content string, changed []string) {
	t.Helper()
	dir := copyFixture(t)
	changedAbs, err := ProcessPackages(dir, []string{"./..."}, manifest, prefixes)
	if err != nil {
		t.Fatalf("ProcessPackages: %v", err)
	}
	for _, c := range changedAbs {
		rel, err := filepath.Rel(dir, c)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}
		changed = append(changed, filepath.ToSlash(rel))
	}
	b, err := os.ReadFile(filepath.Join(dir, "usage", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b), changed
}

// chainsManifest is the common manifest for most chains-mode cases: an
// "escrow" entity with fields Name (nillable), Bio, Score (canAdd), Tags
// (canAppend), a unique edge Status (nillable, StructField used as-is —
// entc never singularizes Set<E>ID/SetNillable<E>ID), and a non-unique
// edge Parcels whose old Add/RemoveIDs methods use the singularized
// "Parcel" (entc's Edge.MutationAdd/MutationRemove; see MethodBase's
// doc comment on SetterEntry in rewrite.go).
var chainsManifest = Manifest{
	"escrow": {
		ImportPath: "example.com/chainsmod/gen/escrow",
		Setters: map[string]SetterEntry{
			"Name":    {Kind: "field", Nillable: true},
			"Bio":     {Kind: "field"},
			"Score":   {Kind: "field", CanAdd: true},
			"Tags":    {Kind: "field", CanAppend: true},
			"Status":  {Kind: "edge", Unique: true, Nillable: true},
			"Parcels": {Kind: "edge", Unique: false, MethodBase: "Parcel"},
		},
	},
}

var chainsPrefixes = []string{"example.com/chainsmod/gen"}

// voucherManifest describes the Voucher entity used by the deleted-setters
// (Tests:true + propagation) fixture — see VoucherCreate's doc comment in
// gen/gen.go for why it's a separate stand-in from escrow.
var voucherManifest = Manifest{
	"voucher": {
		ImportPath: "example.com/chainsmod/gen/voucher",
		Setters: map[string]SetterEntry{
			"Title": {Kind: "field"},
			"Desc":  {Kind: "field"},
			"Price": {Kind: "field", CanAdd: true},
		},
	},
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output does not contain %q\ngot:\n%s", want, got)
	}
}

func mustNotContain(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Errorf("output unexpectedly contains %q\ngot:\n%s", want, got)
	}
}

func TestChainsSingleSetter(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "single_setter.go")
	mustContainChanged(t, changed, "usage/single_setter.go")
	mustContain(t, got, `client.Escrow.Create().With(escrow.Field.Name.Set("x"))`)
	mustContain(t, got, `"example.com/chainsmod/gen/escrow"`)
	mustNotContain(t, got, `.SetName(`)
}

func TestChainsLongChainFoldsToOneWith(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "long_chain.go")
	mustContainChanged(t, changed, "usage/long_chain.go")
	mustContain(t, got, `client.Escrow.Create().With(escrow.Field.Name.Set("x"), escrow.Field.Bio.Set("y"), escrow.Field.Score.Set(5))`)
	if strings.Count(got, ".With(") != 1 {
		t.Errorf("want exactly one .With( call in a folded chain, got:\n%s", got)
	}
}

func TestChainsInterruptedByWhereKeepsTwoWiths(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "chain_where.go")
	mustContainChanged(t, changed, "usage/chain_where.go")
	mustContain(t, got, `client.Escrow.Update().With(escrow.Field.Name.Set("x")).Where().With(escrow.Field.Bio.Set("y")).Save(ctx)`)
	if strings.Count(got, ".With(") != 2 {
		t.Errorf("want exactly two .With( calls (fold broken by Where), got:\n%s", got)
	}
}

func TestChainsSetNillable(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "set_nillable.go")
	mustContainChanged(t, changed, "usage/set_nillable.go")
	mustContain(t, got, `client.Escrow.Create().With(escrow.Field.Name.SetNillable(name))`)
	mustNotContain(t, got, `.SetNillableName(`)
}

func TestChainsEdgeForms(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "edge_forms.go")
	mustContainChanged(t, changed, "usage/edge_forms.go")
	mustContain(t, got, `client.Escrow.UpdateOneID(1).With(escrow.Edge.Status.SetID(2))`)
	mustContain(t, got, `client.Escrow.Create().With(escrow.Edge.Status.SetNillableID(statusID))`)
	// Old methods AddParcelIDs/RemoveParcelIDs use the singularized
	// "Parcel" (matching real entc codegen); the manifest's "Parcels" key
	// (and E.Parcels handle) stays plural throughout.
	mustContain(t, got, `client.Escrow.Create().With(escrow.Edge.Parcels.AddIDs(1, 2))`)
	mustContain(t, got, `client.Escrow.Update().With(escrow.Edge.Parcels.RemoveIDs(1, 2)).Save(ctx)`)
	mustContain(t, got, `client.Escrow.UpdateOneID(1).With(escrow.Field.Bio.Clear())`)
	mustContain(t, got, `client.Escrow.Update().With(escrow.Edge.Parcels.Clear())`)
	mustContain(t, got, `client.Escrow.Create().With(escrow.Field.Score.Add(5), escrow.Field.Tags.Append([]string{"x"}))`)
}

func TestChainsNonManifestBuilderUntouched(t *testing.T) {
	// The "other" package sits outside chainsPrefixes, so its WidgetCreate
	// (whose SetName shape would otherwise decompose) must be left alone.
	original, err := os.ReadFile("testdata/chainsmod/usage/non_manifest_untouched.go")
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "non_manifest_untouched.go")
	for _, c := range changed {
		if c == "usage/non_manifest_untouched.go" {
			t.Fatalf("non_manifest_untouched.go should not be reported as changed; changed=%v", changed)
		}
	}
	if got != string(original) {
		t.Errorf("non_manifest_untouched.go was modified:\ngot:\n%s\nwant (unchanged):\n%s", got, original)
	}
}

func TestChainsShadowedVariableHandledCorrectly(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "shadowed.go")
	mustContainChanged(t, changed, "usage/shadowed.go")
	// The param named "escrow" (an unrelated type) is untouched...
	mustContain(t, got, `func paramShadow(escrow *notEscrow) {`)
	// ...and the sibling function's legitimate chain still rewrites,
	// despite the file-wide name collision — types mode doesn't care.
	mustContain(t, got, `client.Escrow.Create().With(escrow.Field.Name.Set("x"))`)
}

// ambiguousManifest gives "escrow" both a field "StatusID" and a unique
// edge "Status", so the single method "SetStatusID" decomposes two ways:
// F.StatusID.Set(v) and E.Status.SetID(v).
var ambiguousManifest = Manifest{
	"escrow": {
		ImportPath: "example.com/chainsmod/gen/escrow",
		Setters: map[string]SetterEntry{
			"StatusID": {Kind: "field"},
			"Status":   {Kind: "edge", Unique: true},
		},
	},
}

func TestChainsAmbiguityRefused(t *testing.T) {
	original, err := os.ReadFile("testdata/chainsmod/usage/ambiguous.go")
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	got, changed := runChains(t, ambiguousManifest, chainsPrefixes, "ambiguous.go")
	for _, c := range changed {
		if c == "usage/ambiguous.go" {
			t.Fatalf("ambiguous.go should not be reported as changed; changed=%v", changed)
		}
	}
	if got != string(original) {
		t.Errorf("ambiguous.go was modified:\ngot:\n%s\nwant (unchanged):\n%s", got, original)
	}
}

// importAliasManifest points at the same real (fixture) import path used by
// usage/import_alias.go's existing "esc" aliased import, so the
// alias-reuse path is exercised end to end.
var importAliasManifest = Manifest{
	"escrow": {
		ImportPath: "example.com/chainsmod/gen/escrow",
		Setters: map[string]SetterEntry{
			"Name": {Kind: "field", Nillable: true},
		},
	},
}

func TestChainsImportInsertionReusesExistingAlias(t *testing.T) {
	got, changed := runChains(t, importAliasManifest, chainsPrefixes, "import_alias.go")
	mustContainChanged(t, changed, "usage/import_alias.go")
	mustContain(t, got, `client.Escrow.Create().With(esc.Field.Name.Set("x"))`)
	// Only one import of the escrow path — no duplicate added alongside
	// the existing aliased one.
	if n := strings.Count(got, `"example.com/chainsmod/gen/escrow"`); n != 1 {
		t.Errorf("want exactly one import of the escrow path, got %d:\n%s", n, got)
	}
}

// TestChainsLoadsTestFiles_AndFoldsThroughDeletedSetters is the regression
// test for c049aca1a's two most consequential fixes at once: without
// packages.Config.Tests: true this _test.go fixture is never even loaded
// (changed would come back empty); without exprResult.entry/hasEntry
// propagation, only SetTitle (the link right after Create(), whose
// receiver type Create() itself still resolves) would decompose — SetDesc
// and SetPrice would be left untouched, since VoucherCreate has no old
// setter methods at all (their own receiver, the previous link's call, is
// itself unresolvable).
func TestChainsLoadsTestFiles_AndFoldsThroughDeletedSetters(t *testing.T) {
	got, changed := runChains(t, voucherManifest, chainsPrefixes, "deleted_setters_test.go")
	mustContainChanged(t, changed, "usage/deleted_setters_test.go")
	mustContain(t, got, `client.Voucher.Create().With(voucher.Field.Title.Set("x"), voucher.Field.Desc.Set("y"), voucher.Field.Price.Set(5))`)
	if strings.Count(got, ".With(") != 1 {
		t.Errorf("want exactly one .With( call (all three links folded), got:\n%s", got)
	}
	mustNotContain(t, got, `.SetTitle(`)
	mustNotContain(t, got, `.SetDesc(`)
	mustNotContain(t, got, `.SetPrice(`)
}

// TestChainsVariadicSpreadPreserved guards buildHandleCall carrying
// call.Ellipsis over: AddParcelIDs(ids...) must rewrite to
// E.Parcels.AddIDs(ids...), not AddIDs(ids) (a []int where the handle's
// variadic AddIDs(vs ...ID) wants ...int — a compile error the old text
// output wouldn't have caught since these fixtures aren't compiled).
func TestChainsVariadicSpreadPreserved(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "variadic_spread.go")
	mustContainChanged(t, changed, "usage/variadic_spread.go")
	mustContain(t, got, `client.Escrow.Create().With(escrow.Edge.Parcels.AddIDs(ids...))`)
	mustNotContain(t, got, `AddIDs(ids)`)
}

// TestChainsFuncLiteralArgumentRewritten guards the func-literal fallback
// in processExpr (walkForChains): a chain nested inside a func-literal
// argument (the common t.Run(name, func(t *testing.T) { ... }) shape) must
// still be found and rewritten, not silently left untouched because
// processExpr's own receiver/args recursion doesn't see into a literal's
// body on its own.
func TestChainsFuncLiteralArgumentRewritten(t *testing.T) {
	got, changed := runChains(t, chainsManifest, chainsPrefixes, "func_literal.go")
	mustContainChanged(t, changed, "usage/func_literal.go")
	mustContain(t, got, `client.Escrow.Create().With(escrow.Field.Name.Set("x"))`)
	mustNotContain(t, got, `.SetName(`)
}

func mustContainChanged(t *testing.T, changed []string, want string) {
	t.Helper()
	for _, c := range changed {
		if c == want {
			return
		}
	}
	t.Errorf("want %q in changed files, got %v", want, changed)
}
