package usage

import "example.com/chainsmod/gen"

// spreadIDs exercises call.Ellipsis preservation: the old
// AddParcelIDs(ids...) spread must survive as E.Parcels.AddIDs(ids...),
// not silently drop the "..." (which would pass a []int where the
// variadic AddIDs(vs ...ID) wants ...int).
func spreadIDs(client *gen.Client, ids []int) {
	client.Escrow.Create().AddParcelIDs(ids...)
}
