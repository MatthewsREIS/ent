package entbuilder

import (
	"errors"
	"testing"
)

func TestMust_ReturnsValueOnNilErr(t *testing.T) {
	got := Must(42, nil)
	if got != 42 {
		t.Fatalf("Must should return v when err is nil; got %d", got)
	}
}

func TestMust_PanicsOnErr(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Must must panic when err is non-nil")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("Must must panic with the err value; got %T", r)
		}
		if err.Error() != "boom" {
			t.Fatalf("panic payload must be the err; got %v", err)
		}
	}()
	_ = Must(0, errors.New("boom"))
}

func TestMust_ZeroValueTypeInference(t *testing.T) {
	// Ensure T is inferred from the first arg across return types that matter
	// for query builders: slices, pointers, primitives.
	type node struct{ ID int }
	var zeroSlice []*node
	got := Must(zeroSlice, nil)
	if got != nil {
		t.Fatalf("zero slice must round-trip through Must; got %v", got)
	}

	var zeroPtr *node
	gotPtr := Must(zeroPtr, nil)
	if gotPtr != nil {
		t.Fatalf("nil *node must round-trip through Must; got %v", gotPtr)
	}
}
