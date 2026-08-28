package usage

import "example.com/chainsmod/gen"

// literalWrap exercises a chain nested inside a func-literal argument (the
// common t.Run(name, func(t *testing.T) { ...chain... }) shape). processExpr's
// own receiver/args recursion doesn't see into a func literal's body on its
// own; only walkForChains's full-subtree walk does.
func literalWrap(client *gen.Client) {
	run(func() {
		client.Escrow.Create().SetName("x")
	})
}

func run(f func()) { f() }
