// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package load

import "strings"

// codegenTag is the build tag entc sets on every schema load so that files
// gated with `//go:build !entcodegen` are excluded during code generation.
// This lets schema authors put hook bodies, interceptor bodies, and any
// helper code that imports generated symbols into tagged files without
// blocking codegen when those symbols don't exist yet.
const codegenTag = "entcodegen"

// mergeCodegenTag returns buildFlags with codegenTag merged into the LAST
// existing -tags flag, or appended as a new "-tags=entcodegen" flag if
// none exists. The function is idempotent: passing flags that already
// contain entcodegen returns them unchanged.
//
// Go's -tags flag is last-wins, not additive, so we cannot simply append
// "-tags=entcodegen", and we must merge into the last occurrence rather
// than the first. This helper handles both flag forms:
//   - "-tags=foo,bar" (single arg, equals-form)
//   - "-tags" "foo,bar" (two args, space-form)
func mergeCodegenTag(buildFlags []string) []string {
	out := append([]string(nil), buildFlags...)
	// Go's -tags flag is last-wins, so find the LAST occurrence to merge into.
	lastTagIdx := -1
	lastTagForm := "" // "equals" or "twoarg"
	for i := 0; i < len(out); i++ {
		switch {
		case out[i] == "-tags" && i+1 < len(out):
			lastTagIdx = i
			lastTagForm = "twoarg"
		case strings.HasPrefix(out[i], "-tags="):
			lastTagIdx = i
			lastTagForm = "equals"
		}
	}
	if lastTagIdx < 0 {
		return append(out, "-tags="+codegenTag)
	}
	switch lastTagForm {
	case "twoarg":
		if !hasTag(out[lastTagIdx+1], codegenTag) {
			out[lastTagIdx+1] = appendTag(out[lastTagIdx+1], codegenTag)
		}
	case "equals":
		tags := strings.TrimPrefix(out[lastTagIdx], "-tags=")
		if !hasTag(tags, codegenTag) {
			out[lastTagIdx] = "-tags=" + appendTag(tags, codegenTag)
		}
	}
	return out
}

// hasTag reports whether the comma-separated tag list contains the given tag.
func hasTag(tagList, tag string) bool {
	for _, t := range strings.Split(tagList, ",") {
		if t == tag {
			return true
		}
	}
	return false
}

// appendTag adds tag to the comma-separated tag list.
func appendTag(tagList, tag string) string {
	if tagList == "" {
		return tag
	}
	return tagList + "," + tag
}
