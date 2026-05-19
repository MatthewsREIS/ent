// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Mutation hot-path bench gate. Fails the build if Field/SetField mean
// exceeds the budget (≤100 ns / ≤200 ns).
package bench_test

import (
	"reflect"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
)

type benchEntity struct {
	ID    int
	Title string
}

func benchDescriptor() *entbuilder.Descriptor {
	return &entbuilder.Descriptor{
		Name:   "BenchEntity",
		IDType: reflect.TypeFor[int](),
		Fields: map[string]entbuilder.FieldSpec{
			"title": {Type: reflect.TypeFor[string](), GoName: "Title"},
		},
	}
}

func BenchmarkMutationField(b *testing.B) {
	m := entbuilder.NewMutation[benchEntity](nil, ent.OpUpdate, benchDescriptor())
	_ = m.SetField("title", "hello")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Field("title")
	}
}

func BenchmarkMutationSetField(b *testing.B) {
	m := entbuilder.NewMutation[benchEntity](nil, ent.OpUpdate, benchDescriptor())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.SetField("title", "hello")
	}
}

// TestMutationHotPath_Budget runs the benchmarks programmatically and
// fails if mean per-op exceeds the budget. Runs in the normal `go test`
// flow; no `-bench` flag needed for the gate.
func TestMutationHotPath_Budget(t *testing.T) {
	const fieldBudgetNS = 100
	const setFieldBudgetNS = 200

	r := testing.Benchmark(BenchmarkMutationField)
	if got := r.NsPerOp(); got > fieldBudgetNS {
		t.Errorf("Field hot path: %d ns/op (budget %d ns)", got, fieldBudgetNS)
	}
	r = testing.Benchmark(BenchmarkMutationSetField)
	if got := r.NsPerOp(); got > setFieldBudgetNS {
		t.Errorf("SetField hot path: %d ns/op (budget %d ns)", got, setFieldBudgetNS)
	}
}
