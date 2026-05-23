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
// If err is not a packages.Error (the typical schema-typecheck failure),
// it is returned unchanged.
func wrapLoadError(err error, schemaPath string) error {
	var pkgErr packages.Error
	if !errors.As(err, &pkgErr) {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "entc/load: schema package failed to typecheck under -tags=entcodegen:\n")
	fmt.Fprintf(&b, "  %s: %s\n", pkgErr.Pos, pkgErr.Msg)

	if generatedSymbolRE.MatchString(pkgErr.Msg) {
		fmt.Fprintf(&b, "\n")
		fmt.Fprintf(&b, "  This file appears to reference generated code. If it contains hook,\n")
		fmt.Fprintf(&b, "  interceptor, or runtime helpers, add this on the first line:\n\n")
		fmt.Fprintf(&b, "      //go:build !entcodegen\n\n")
		fmt.Fprintf(&b, "  See docs/codegen-isolation.md for details.\n")
	}

	return errors.New(b.String())
}
