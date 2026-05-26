// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
)

// generatedSymbolRE matches identifiers that suggest the failing symbol
// belongs to ent-generated code. The patterns are intentionally permissive:
// false positives (suggesting the build tag when it's actually a typo)
// are benign because `go build ./...` will still surface the real bug.
var generatedSymbolRE = regexp.MustCompile(`\b(hook|intercept|ent|gen)\.[A-Z]`)

// wrapLoadError annotates a load failure with the failing import location
// and, when the failure looks like a reference to ent-generated code,
// a hint about the //go:build !entcodegen escape hatch.
//
// Two error shapes are handled:
//
//  1. packages.Error — produced by packages.Load when the schema package fails
//     to typecheck (missing import, etc.). Extracted via errors.As.
//
//  2. Plain errors — produced by gorun/gocmd when go run fails to compile the
//     schema loader (e.g. "undefined: hook.UserFunc" surfaces at go build time).
//     Detected by checking err.Error() directly.
func wrapLoadError(err error, schemaPath string) error {
	var pkgErr packages.Error
	if errors.As(err, &pkgErr) {
		return formatLoadError(pkgErr.Pos, pkgErr.Msg)
	}

	// Fallback: the error came from gorun (go build stderr). The message
	// may still contain a generated-code reference; annotate it too.
	msg := err.Error()
	if generatedSymbolRE.MatchString(msg) {
		return formatLoadError("", msg)
	}

	return err
}

// formatLoadError builds the annotated error string for a schema load failure.
// pos may be empty when the error origin has no position information.
func formatLoadError(pos, msg string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "entc/load: schema package failed to typecheck under -tags=entcodegen:\n")
	if pos != "" {
		fmt.Fprintf(&b, "  %s: %s\n", pos, msg)
	} else {
		fmt.Fprintf(&b, "  %s\n", msg)
	}

	if generatedSymbolRE.MatchString(msg) {
		fmt.Fprintf(&b, "\n")
		fmt.Fprintf(&b, "  This file appears to reference generated code. If it contains hook,\n")
		fmt.Fprintf(&b, "  interceptor, or runtime helpers, add this on the first line:\n\n")
		fmt.Fprintf(&b, "      //go:build !entcodegen\n\n")
		fmt.Fprintf(&b, "  See docs/codegen-isolation.md for details.\n")
	}

	return errors.New(b.String())
}
