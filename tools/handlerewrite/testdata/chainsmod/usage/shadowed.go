package usage

import "example.com/chainsmod/gen"

type notEscrow struct{}

// paramShadow's param is named "escrow" but has nothing to do with the
// tracked entity — proves -chains mode doesn't key off identifier names at
// all (unlike v1's shadow-heuristic), so it can't be confused by this.
func paramShadow(escrow *notEscrow) {
	_ = escrow
}

// clean sits in the same file as paramShadow's shadowing declaration; its
// legitimate chain on an actual *gen.EscrowCreate still rewrites normally.
func clean(client *gen.Client) {
	client.Escrow.Create().SetName("x")
}
