// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Command ent-codegen-migrate rewrites consumer source code to use the
// post-PR-5 mutation and predicate APIs.
//
// Usage:
//
//	ent-codegen-migrate -descriptors <generated-internal-pkg-path> <consumer-pkg-path>...
//
// The tool reads regenerated <entity>_mutation.go files in the descriptors
// path to learn field/edge names and types, then walks the consumer packages
// rewriting call sites.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var (
		descriptorsFlag = flag.String("descriptors", "", "path to the consumer's regenerated <pkg>/internal/ directory")
		genPackageFlag  = flag.String("gen-package", "", "full import path of the consumer's generated ent package (e.g. github.com/foo/bar/internal/ent/gen). Required: the rewriter emits cross-package facade calls (ent.Query<X><Y>FromQuery) and needs to qualify them with the correct local alias per consumer file. If the file imports the gen package as \"gen\", the call becomes gen.Query...; if aliased as \"myent\", it becomes myent.Query...")
		dryRunFlag      = flag.Bool("dry-run", false, "print changes without writing files")
	)
	flag.Parse()

	if *descriptorsFlag == "" {
		fmt.Fprintln(os.Stderr, "ent-codegen-migrate: -descriptors is required")
		flag.Usage()
		os.Exit(1)
	}
	if *genPackageFlag == "" {
		fmt.Fprintln(os.Stderr, "ent-codegen-migrate: -gen-package is required")
		flag.Usage()
		os.Exit(1)
	}
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "ent-codegen-migrate: at least one consumer package path required")
		flag.Usage()
		os.Exit(1)
	}

	descs, err := LoadDescriptors(*descriptorsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ent-codegen-migrate: load descriptors: %v\n", err)
		os.Exit(1)
	}
	// Populate pkgImports with the (alias → import path) bindings observed
	// in the descriptor source files. The add-missing-imports pass uses
	// this map to resolve "uuid.UUID" / "time.Time" references introduced
	// by earlier passes' type-arg emissions.
	if err := LoadDescriptorImports(*descriptorsFlag); err != nil {
		fmt.Fprintf(os.Stderr, "ent-codegen-migrate: load descriptor imports: %v\n", err)
		os.Exit(1)
	}

	// The generated root is the parent of the -descriptors directory
	// (i.e. the parent of the internal/ package). The walker skips this
	// subtree by absolute-path equality so consumer layouts where
	// src/ent/ holds both src/ent/gen/ (generated) and
	// src/ent/schema/ (hand-written) only skip the generated half.
	genRoot, err := filepath.Abs(filepath.Dir(*descriptorsFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ent-codegen-migrate: resolve generated root: %v\n", err)
		os.Exit(1)
	}

	for _, pkg := range flag.Args() {
		if err := RewritePackage(pkg, descs, genRoot, *genPackageFlag, *dryRunFlag); err != nil {
			fmt.Fprintf(os.Stderr, "ent-codegen-migrate: rewrite %s: %v\n", pkg, err)
			os.Exit(1)
		}
	}
}

// RewritePackage walks pkgPath for .go files and applies all rewriters in
// canonical order: edge-fk-setfield → typed-setter → mutation → predicate
// → edge-method → typed-edge-accessor → collection-method → has-all-fields
// → add-missing-imports.
//
// The genRoot argument is the absolute path of the generated package root
// (the parent of the -descriptors internal/ directory). The walker skips
// that subtree by absolute-path equality so it does not rewrite generated
// code while still descending into sibling hand-written packages (e.g.
// src/ent/schema/ in consumer layouts).
//
// genPackage is the full import path of the consumer's generated package
// (e.g. "github.com/foo/bar/internal/ent/gen"). The edge-method pass uses
// it to resolve the local alias for each file emitting facade calls.
//
// Each pass is idempotent; the chain is safe to re-run. Order matters
// because later passes may inspect shapes produced by earlier ones.
func RewritePackage(pkgPath string, descs Descriptors, genRoot, genPackage string, dryRun bool) error {
	// Adapt the three rewriters that don't care about the gen package
	// to the four-arg pass signature. Keeping their public signatures
	// unchanged keeps the existing per-pass tests stable.
	edgeMethodPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteEdgeMethodSource(filename, src, d, gp)
	}
	mutationPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteMutationSource(filename, src, d, gp)
	}
	predicatePass := func(filename, src string, d Descriptors, _ string) (string, error) {
		return RewritePredicateSource(filename, src, d)
	}
	typedEdgePass := func(filename, src string, d Descriptors, _ string) (string, error) {
		return RewriteTypedEdgeAccessorSource(filename, src, d)
	}
	collectionMethodPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteCollectionMethodSource(filename, src, d, gp)
	}
	hasAllFieldsPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteHasAllFieldsSource(filename, src, d, gp)
	}
	edgeFKSetFieldPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteEdgeFKSetFieldSource(filename, src, d, gp)
	}
	typedSetterPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteTypedSetterSource(filename, src, d, gp)
	}
	addMissingImportsPass := func(filename, src string, d Descriptors, gp string) (string, error) {
		return RewriteAddMissingImportsSource(filename, src, d, gp)
	}
	passes := []struct {
		name string
		fn   func(string, string, Descriptors, string) (string, error)
	}{
		// edge-fk-setfield runs before mutation because it rewrites
		// SetField / Field on ent.Mutation receivers using only the FK
		// column → edge map. mutation rewrites Set<Field> / <Field>()
		// shapes — disjoint from this pass — but ordering this first
		// keeps the new rewrite's "interface-receiver" reasoning isolated.
		{"edge-fk-setfield", edgeFKSetFieldPass},
		// typed-setter rewrites the SetDeletedAt typed-setter pattern
		// that mixin_base.go uses via an interface assertion. Runs early
		// because it transforms an interface-assertion + call group; later
		// passes that walk individual call sites are unaffected by it.
		{"typed-setter", typedSetterPass},
		{"mutation", mutationPass},
		{"predicate", predicatePass},
		{"edge-method", edgeMethodPass},
		{"typed-edge-accessor", typedEdgePass},
		{"collection-method", collectionMethodPass},
		// has-all-fields rewrites q.HasAllFields(fields...) →
		// gen.<Entity>HasAllFields(q, fields...). Sibling of collection-method
		// (same Query-receiver pattern) but emits a Query-less facade name —
		// gqlgen's collection template predates the Query-infix convention.
		{"has-all-fields", hasAllFieldsPass},
		// add-missing-imports runs last so it sees every type-arg
		// emitted by earlier passes (GetField[time.Time], EdgeIDAs[uuid.UUID])
		// and adds the matching import when missing.
		{"add-missing-imports", addMissingImportsPass},
	}
	return filepath.WalkDir(pkgPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if absPath == genRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out := string(src)
		for _, p := range passes {
			out, err = p.fn(path, out, descs, genPackage)
			if err != nil {
				return fmt.Errorf("%s: %s rewrite: %w", path, p.name, err)
			}
		}
		if out == string(src) {
			return nil
		}
		if dryRun {
			fmt.Printf("--- %s (would rewrite) ---\n", path)
			return nil
		}
		return os.WriteFile(path, []byte(out), 0o644)
	})
}
