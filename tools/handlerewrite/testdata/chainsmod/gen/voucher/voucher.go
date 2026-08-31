// Package voucher stands in for a Task-1 F/E-handle entity subpackage —
// see gen/escrow/escrow.go's doc comment; same role, for the Voucher
// entity used by the propagation/deleted-setters fixtures.
package voucher

// Marker exists only so a consumer file can reference this package (Go
// requires imports be used); -chains mode never inspects it.
type Marker struct{}
