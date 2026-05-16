// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

// Fixture is one in-repo schema that the bench can measure end-to-end.
// SchemaDir is the path to the directory containing the ent.Schema-implementing
// Go files (relative to the repo root).
type Fixture struct {
	Name      string
	SchemaDir string
}

// InRepoFixtures returns the list of integration fixtures the bench knows about.
// Wired up in Task 10 once subprocess measurement is in place.
func InRepoFixtures() []Fixture {
	return []Fixture{
		{Name: "privacy", SchemaDir: "entc/integration/privacy/ent/schema"},
		{Name: "hooks", SchemaDir: "entc/integration/hooks/ent/schema"},
		{Name: "edgeschema", SchemaDir: "entc/integration/edgeschema/ent/schema"},
	}
}
