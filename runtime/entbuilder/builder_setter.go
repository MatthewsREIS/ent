package entbuilder

// BSet is the generic shape for a generated *XxxCreate / *XxxUpdate setter
// that wraps a call into the mutation and returns the builder. Generated
// code of the form
//
//	func (uc *UserCreate) SetName(v string) *UserCreate {
//	    uc.mutation.SetName(v)
//	    return uc
//	}
//
// collapses to
//
//	func (uc *UserCreate) SetName(v string) *UserCreate { return entbuilder.BSet(uc, uc.mutation.SetName, v) }
//
// The type parameter B is the builder's struct type (call site passes *B);
// V is the value type being set. Instantiation cost scales with
// (# distinct builder types) × (# distinct field types).
func BSet[B any, V any](b *B, set func(V), v V) *B {
	set(v)
	return b
}
