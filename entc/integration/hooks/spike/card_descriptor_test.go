// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package spike

import (
	"testing"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/runtime/entbuilder"
)

func TestCardDescriptor_Valid(t *testing.T) {
	if err := entbuilder.ValidateSchema[ent.Card](cardSchema); err != nil {
		t.Fatalf("Card descriptor fails validation: %v", err)
	}
}
