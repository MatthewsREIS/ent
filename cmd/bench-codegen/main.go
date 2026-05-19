// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Command bench-codegen measures ent codegen cost across fixtures.
// See internal/bench/README.md for usage.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"entgo.io/ent/internal/bench"
)

func main() {
	var (
		fixtureFlag = flag.String("fixture", "all", "fixture name to run, or 'all'")
		outFlag     = flag.String("out", "-", "output path; '-' means stdout")
		schemaFlag  = flag.String("schema", "", "absolute path to an external schema dir (overrides -fixture)")
		labelFlag   = flag.String("label", "external", "fixture label to record when using -schema")
	)
	flag.Parse()

	repoRoot, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bench-codegen: %v\n", err)
		os.Exit(1)
	}

	var fixtures []bench.Fixture
	switch {
	case *schemaFlag != "":
		if !filepath.IsAbs(*schemaFlag) {
			fmt.Fprintln(os.Stderr, "bench-codegen: -schema must be an absolute path")
			os.Exit(1)
		}
		fixtures = []bench.Fixture{{Name: *labelFlag, SchemaDir: *schemaFlag}}
	case *fixtureFlag == "all":
		fixtures = bench.InRepoFixtures()
	default:
		for _, f := range bench.InRepoFixtures() {
			if f.Name == *fixtureFlag {
				fixtures = append(fixtures, f)
			}
		}
		if len(fixtures) == 0 {
			fmt.Fprintf(os.Stderr, "bench-codegen: unknown fixture %q (known: %v)\n", *fixtureFlag, fixtureNames(bench.InRepoFixtures()))
			os.Exit(1)
		}
	}

	out := io.Writer(os.Stdout)
	if *outFlag != "-" {
		f, err := os.Create(*outFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bench-codegen: open out: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)

	exitCode := 0
	for _, f := range fixtures {
		fmt.Fprintf(os.Stderr, "bench-codegen: running %s...\n", f.Name)
		run, err := bench.RunFixture(repoRoot, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bench-codegen: %s: %v\n", f.Name, err)
			exitCode = 1
			continue
		}
		if err := enc.Encode(run); err != nil {
			fmt.Fprintf(os.Stderr, "bench-codegen: encode %s: %v\n", f.Name, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	root := strings.TrimRight(string(out), "\n")
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("abs: %w", err)
	}
	return abs, nil
}

func fixtureNames(fs []bench.Fixture) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Name
	}
	return out
}
