package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	manifestPath := flag.String("manifest", "", "path to handle manifest JSON")
	flag.Parse()
	dirs := flag.Args()
	if *manifestPath == "" || len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: handlerewrite -manifest <manifest.json> <pkg-dir>...")
		os.Exit(2)
	}

	manifest, err := LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "handlerewrite: load manifest:", err)
		os.Exit(1)
	}

	changed, err := ProcessDirs(dirs, manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "handlerewrite:", err)
		os.Exit(1)
	}
	for _, f := range changed {
		fmt.Println(f)
	}
}
