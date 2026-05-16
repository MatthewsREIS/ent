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
		dryRunFlag      = flag.Bool("dry-run", false, "print changes without writing files")
	)
	flag.Parse()

	if *descriptorsFlag == "" {
		fmt.Fprintln(os.Stderr, "ent-codegen-migrate: -descriptors is required")
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

	for _, pkg := range flag.Args() {
		if err := RewritePackage(pkg, descs, *dryRunFlag); err != nil {
			fmt.Fprintf(os.Stderr, "ent-codegen-migrate: rewrite %s: %v\n", pkg, err)
			os.Exit(1)
		}
	}
}

// RewritePackage walks pkgPath for .go files (excluding _test.go and any
// path matching */ent/* generated trees) and applies both the mutation
// and predicate rewriters.
func RewritePackage(pkgPath string, descs Descriptors, dryRun bool) error {
	return filepath.WalkDir(pkgPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip generated trees.
			if d.Name() == "gen" || d.Name() == "ent" || d.Name() == "internal" {
				if strings.Contains(path, "/ent/") || strings.HasSuffix(path, "/ent") {
					return filepath.SkipDir
				}
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
		// Mutation rewrite first, then predicate (so AST node identities
		// stay stable through each pass).
		out, err := RewriteMutationSource(path, string(src), descs)
		if err != nil {
			return fmt.Errorf("%s: mutation rewrite: %w", path, err)
		}
		out, err = RewritePredicateSource(path, out, descs)
		if err != nil {
			return fmt.Errorf("%s: predicate rewrite: %w", path, err)
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
