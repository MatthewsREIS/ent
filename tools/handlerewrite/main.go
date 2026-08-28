package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// prefixList is a repeatable, comma-splitting flag.Value for -pkgprefix.
type prefixList []string

func (p *prefixList) String() string { return strings.Join(*p, ",") }

func (p *prefixList) Set(s string) error {
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*p = append(*p, part)
		}
	}
	return nil
}

// validateChainsFlags enforces that -chains is only used with -pkgprefix.
// Without a prefix guard, builderEntry would match any <X>Create/Update/
// UpdateOne-named type across the whole load, risking a rewrite of an
// unrelated same-named type in a mass migration — fail closed rather than
// rely on the flag's doc string.
func validateChainsFlags(chains bool, prefixes []string) error {
	if chains && len(prefixes) == 0 {
		return fmt.Errorf("-chains requires -pkgprefix (without it, any same-named " +
			"*<X>Create/*<X>Update/*<X>UpdateOne type across the whole load would be eligible)")
	}
	return nil
}

func main() {
	manifestPath := flag.String("manifest", "", "path to handle manifest JSON")
	chains := flag.Bool("chains", false, "types-aware setter-chain rewrite mode: decompose "+
		"old Set<F>/Add<F>/Set<E>ID/... calls on generated Create/Update builders into "+
		"F/E handle assignments, folding a chain's consecutive rewrites into one .With(...). "+
		"Implies types-aware package loading (go/packages); args are package patterns "+
		"(e.g. \"./...\"), not directories, and the manifest needs the v2 importPath/setters "+
		"fields. Requires -pkgprefix.")
	var prefixes prefixList
	flag.Var(&prefixes, "pkgprefix", "import path prefix eligible for rewriting; "+
		"repeatable and/or comma-separated. When given, only imports whose path "+
		"starts with one of these prefixes resolve to manifest keys (guards "+
		"against a same-basename import from an unrelated tree). Omit for the "+
		"permissive default: resolve by import base name alone.")
	flag.Parse()
	dirs := flag.Args()
	if *manifestPath == "" || len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: handlerewrite -manifest <manifest.json> [-pkgprefix <prefix>[,<prefix>...]] <pkg-dir>...")
		os.Exit(2)
	}
	if err := validateChainsFlags(*chains, prefixes); err != nil {
		fmt.Fprintln(os.Stderr, "handlerewrite:", err)
		os.Exit(2)
	}

	manifest, err := LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "handlerewrite: load manifest:", err)
		os.Exit(1)
	}

	var changed []string
	if *chains {
		changed, err = ProcessPackages("", dirs, manifest, prefixes)
	} else {
		changed, err = ProcessDirs(dirs, manifest, prefixes)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "handlerewrite:", err)
		os.Exit(1)
	}
	for _, f := range changed {
		fmt.Println(f)
	}
}
