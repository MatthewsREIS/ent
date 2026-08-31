package usage

import "example.com/chainsmod/gen"

// ambiguousSetStatusID calls a method whose name decomposes two ways
// against a manifest carrying both a field "StatusID" and a unique edge
// "Status": Field.StatusID.Set(v) and Edge.Status.SetID(v) both read validly.
// Refuse to guess; leave it untouched.
func ambiguousSetStatusID(client *gen.Client) {
	client.Escrow.Create().SetStatusID(2)
}
