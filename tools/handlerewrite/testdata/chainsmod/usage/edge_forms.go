package usage

import (
	"context"

	"example.com/chainsmod/gen"
)

func edgeUniqueSet(client *gen.Client) {
	client.Escrow.UpdateOneID(1).SetStatusID(2)
}

func edgeUniqueSetNillable(client *gen.Client, statusID *int) {
	client.Escrow.Create().SetNillableStatusID(statusID)
}

func edgeNonUniqueAddRemove(client *gen.Client, ctx context.Context) {
	client.Escrow.Update().RemoveParcelsIDs(1, 2).Save(ctx)
}

func edgeClearAndFieldClear(client *gen.Client) {
	client.Escrow.UpdateOneID(1).ClearBio()
	client.Escrow.Update().ClearParcels()
}

func fieldAddAppend(client *gen.Client) {
	client.Escrow.Create().AddScore(5).AppendTags([]string{"x"})
}
