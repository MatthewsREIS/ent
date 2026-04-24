package entbuilder

import "testing"

// fakeBuilder stands in for a generated *XxxCreate / *XxxUpdate type.
// The real builders hold config + hooks + mutation; for the helper tests
// we only need an addressable struct and a method-value-like setter.
type fakeBuilder struct {
	setCalls int
	lastInt  int
	lastStr  string
}

func (b *fakeBuilder) setInt(v int) {
	b.setCalls++
	b.lastInt = v
}

func (b *fakeBuilder) setStr(v string) {
	b.setCalls++
	b.lastStr = v
}

func TestBSet_ReturnsSameBuilder(t *testing.T) {
	b := &fakeBuilder{}
	got := BSet(b, b.setInt, 42)
	if got != b {
		t.Fatalf("BSet must return the builder pointer it was given")
	}
	if b.setCalls != 1 {
		t.Fatalf("setter must be called exactly once; got %d", b.setCalls)
	}
	if b.lastInt != 42 {
		t.Fatalf("setter must receive v=42; got %d", b.lastInt)
	}
}

func TestBSet_Str(t *testing.T) {
	b := &fakeBuilder{}
	got := BSet(b, b.setStr, "hello")
	if got != b || b.lastStr != "hello" {
		t.Fatalf("BSet[string] failed: got=%p lastStr=%q", got, b.lastStr)
	}
}

func TestBSet_InstantiatesForMultipleBuilderTypes(t *testing.T) {
	// Ensure generic is instantiable for distinct builder types, matching
	// the 111-schemas-per-consumer pattern.
	type builderA struct{ n int }
	type builderB struct{ n int }
	a := &builderA{}
	b := &builderB{}
	_ = BSet(a, func(v int) { a.n = v }, 1)
	_ = BSet(b, func(v int) { b.n = v }, 2)
	if a.n != 1 || b.n != 2 {
		t.Fatalf("separate instantiations interfered: a.n=%d b.n=%d", a.n, b.n)
	}
}
