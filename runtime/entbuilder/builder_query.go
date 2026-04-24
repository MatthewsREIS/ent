package entbuilder

// Must is the generic shape for a generated *XxxQuery XxxX wrapper that
// panics on error and returns the value. Generated code of the form
//
//	func (uq *UserQuery) AllX(ctx context.Context) []*User {
//	    nodes, err := uq.All(ctx)
//	    if err != nil {
//	        panic(err)
//	    }
//	    return nodes
//	}
//
// collapses to
//
//	func (uq *UserQuery) AllX(ctx context.Context) []*User { return entbuilder.Must(uq.All(ctx)) }
//
// NOTE: the NotFound-tolerant wrappers (FirstX, FirstIDX) cannot use Must;
// they are emitted single-line-collapsed but still multi-statement because
// IsNotFound lives in the generated per-schema package and can't be
// referenced from runtime/entbuilder without an import cycle.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
