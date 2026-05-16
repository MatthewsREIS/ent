// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

// CountLOC walks the directory tree rooted at root, counting newlines in every
// regular file whose name ends with ".go". Returns total LOC, file count, and
// the top-N files by LOC (largest first). N=0 returns no top files.
func CountLOC(root string, topN int) (totalLOC, fileCount int, top []FileStats, err error) {
	panic("not implemented")
}
