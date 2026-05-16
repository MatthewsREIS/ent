// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package bench measures the generation and compile cost of ent codegen
// fixtures. See cmd/bench-codegen for the CLI entrypoint.
package bench

import "errors"

// RunFixture copies the given fixture's schema to a temp module, runs codegen,
// runs go build, and returns a populated Run. Implemented in Task 9.
func RunFixture(repoRoot string, f Fixture) (Run, error) {
	return Run{}, errors.New("not implemented")
}
