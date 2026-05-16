// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package bench measures the generation and compile cost of ent codegen
// fixtures. See cmd/bench-codegen for the CLI entrypoint.
package bench

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunFixture copies the given fixture's schema to a fresh temp module, runs
// codegen, runs go build, and returns a populated Run. The in-repo fixture is
// never modified. repoRoot must be an absolute path to the ent module root.
func RunFixture(repoRoot string, f Fixture) (Run, error) {
	if !filepath.IsAbs(repoRoot) {
		return Run{}, fmt.Errorf("bench: repoRoot must be absolute, got %q", repoRoot)
	}
	mod, err := os.MkdirTemp("", "bench-"+f.Name+"-*")
	if err != nil {
		return Run{}, fmt.Errorf("bench: tempdir: %w", err)
	}
	defer os.RemoveAll(mod)

	if err := stageFixture(repoRoot, f, mod); err != nil {
		return Run{}, fmt.Errorf("bench: stage fixture: %w", err)
	}

	// gen.go and schema/ live inside <mod>/ent/ so that entc.Generate uses
	// the correct module-relative package path for generated imports.
	entDir := filepath.Join(mod, "ent")

	if err := runTidy(mod); err != nil {
		return Run{}, fmt.Errorf("bench: go mod tidy: %w", err)
	}

	genWall, genRSS, err := runGen(entDir)
	if err != nil {
		return Run{}, fmt.Errorf("bench: gen: %w", err)
	}

	buildWall, buildRSS, err := runBuild(entDir)
	if err != nil {
		return Run{}, fmt.Errorf("bench: build: %w", err)
	}

	totalLOC, fileCount, top, err := CountLOC(entDir, 20)
	if err != nil {
		return Run{}, fmt.Errorf("bench: count LOC: %w", err)
	}

	return Run{
		Timestamp:         time.Now().UTC(),
		GitSHA:            gitSHA(repoRoot),
		Fixture:           f.Name,
		GenWallNS:         genWall.Nanoseconds(),
		GenPeakRSSBytes:   genRSS,
		BuildWallNS:       buildWall.Nanoseconds(),
		BuildPeakRSSBytes: buildRSS,
		TotalLOC:          totalLOC,
		FileCount:         fileCount,
		TopFiles:          top,
	}, nil
}

// stageFixture writes a fresh go.mod into mod, then creates <mod>/ent/ with
// the schema directory and a gen.go entrypoint. The module replaces
// entgo.io/ent and entgo.io/ent/entc/integration with the local repo so
// codegen runs against the worktree's templates. gen.go runs from <mod>/ent/
// so that entc.Generate uses the correct module-relative import paths for the
// generated packages.
func stageFixture(repoRoot string, f Fixture, mod string) error {
	goMod := fmt.Sprintf(`module benchfixture

go 1.23

require (
	entgo.io/ent v0.0.0
	entgo.io/ent/entc/integration v0.0.0
)

replace (
	entgo.io/ent => %s
	entgo.io/ent/entc/integration => %s/entc/integration
)
`, repoRoot, repoRoot)
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}

	entDir := filepath.Join(mod, "ent")
	srcSchema := f.SchemaDir
	if !filepath.IsAbs(srcSchema) {
		srcSchema = filepath.Join(repoRoot, srcSchema)
	}
	dstSchema := filepath.Join(entDir, "schema")
	if err := copyTree(srcSchema, dstSchema); err != nil {
		return err
	}

	entcGo := []byte(`//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{}); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
`)
	return os.WriteFile(filepath.Join(entDir, "gen.go"), entcGo, 0o644)
}

func runTidy(mod string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	return cmd.Run()
}

func runGen(entDir string) (time.Duration, int64, error) {
	cmd := exec.Command("go", "run", "-mod=mod", "./gen.go")
	cmd.Dir = entDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	wall, rss, err := MeasureCmd(cmd)
	return wall, rss, err
}

func runBuild(entDir string) (time.Duration, int64, error) {
	cmd := exec.Command("go", "build", "-mod=mod", "./...")
	cmd.Dir = entDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	wall, rss, err := MeasureCmd(cmd)
	return wall, rss, err
}

func gitSHA(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out[:len(out)-1])
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o644)
	})
}
