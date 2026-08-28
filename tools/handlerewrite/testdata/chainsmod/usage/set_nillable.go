package usage

import "example.com/chainsmod/gen"

func setNillable(client *gen.Client, name *string) {
	client.Escrow.Create().SetNillableName(name)
}
