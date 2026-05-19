// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CountLOC walks the directory tree rooted at root, counting newlines in every
// regular file whose name ends with ".go". Returns total LOC, file count, and
// the top-N files by LOC (largest first). N=0 returns no top files.
func CountLOC(root string, topN int) (totalLOC, fileCount int, top []FileStats, err error) {
	if _, statErr := os.Stat(root); statErr != nil {
		return 0, 0, nil, fmt.Errorf("loc: stat root: %w", statErr)
	}
	var all []FileStats
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		n, lerr := countLines(path)
		if lerr != nil {
			return fmt.Errorf("loc: %s: %w", path, lerr)
		}
		all = append(all, FileStats{Path: path, LOC: n})
		totalLOC += n
		fileCount++
		return nil
	})
	if walkErr != nil {
		return 0, 0, nil, walkErr
	}
	if topN > 0 && len(all) > 0 {
		sort.Slice(all, func(i, j int) bool { return all[i].LOC > all[j].LOC })
		if topN > len(all) {
			topN = len(all)
		}
		top = all[:topN]
	}
	return totalLOC, fileCount, top, nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // allow long lines (snapshot files)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
