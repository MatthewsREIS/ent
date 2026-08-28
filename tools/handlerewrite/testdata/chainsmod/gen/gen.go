// Package gen holds hand-written stand-ins for entc-generated builder
// types — just enough surface (method names, receiver/return types) for
// handlerewrite's -chains mode to type-check against via go/packages. Not
// real ent output; no runtime behavior is exercised here.
package gen

import "context"

type Escrow struct{}

type Client struct {
	Escrow  *EscrowClient
	Voucher *VoucherClient
}

// Voucher and VoucherCreate stand in for what a real Task-3-regenerated
// builder actually looks like post-migration: no old Set<F> methods at
// all (unlike EscrowCreate above, which keeps every old setter so its
// receiver's static type always resolves cleanly — never exercising the
// case this type exists for). A chain of 2+ decomposed setters against
// VoucherCreate forces every link after the first to hit an unresolvable
// receiver type (the method it calls doesn't exist), which is exactly the
// state real regenerated code is in: processExpr must recognize the
// builder from a *propagated* manifest entry (see exprResult.entry), not
// from resolving each link's original static type.
type Voucher struct{}

type VoucherClient struct{}

func (c *VoucherClient) Create() *VoucherCreate { return &VoucherCreate{} }

type VoucherCreate struct{}

type EscrowClient struct{}

func (c *EscrowClient) Create() *EscrowCreate               { return &EscrowCreate{} }
func (c *EscrowClient) Update() *EscrowUpdate               { return &EscrowUpdate{} }
func (c *EscrowClient) UpdateOneID(id int) *EscrowUpdateOne { return &EscrowUpdateOne{} }

type EscrowCreate struct{}

func (c *EscrowCreate) SetName(v string) *EscrowCreate            { return c }
func (c *EscrowCreate) SetNillableName(v *string) *EscrowCreate   { return c }
func (c *EscrowCreate) SetBio(v string) *EscrowCreate             { return c }
func (c *EscrowCreate) SetScore(v int) *EscrowCreate              { return c }
func (c *EscrowCreate) AddScore(v int) *EscrowCreate              { return c }
func (c *EscrowCreate) SetTags(v []string) *EscrowCreate          { return c }
func (c *EscrowCreate) AppendTags(v []string) *EscrowCreate       { return c }
func (c *EscrowCreate) SetStatusID(v int) *EscrowCreate           { return c }
func (c *EscrowCreate) SetNillableStatusID(v *int) *EscrowCreate  { return c }
func (c *EscrowCreate) AddParcelIDs(vs ...int) *EscrowCreate      { return c }
func (c *EscrowCreate) Save(ctx context.Context) (*Escrow, error) { return nil, nil }

type EscrowUpdate struct{}

func (u *EscrowUpdate) SetName(v string) *EscrowUpdate          { return u }
func (u *EscrowUpdate) SetBio(v string) *EscrowUpdate           { return u }
func (u *EscrowUpdate) Where(ps ...func()) *EscrowUpdate        { return u }
func (u *EscrowUpdate) RemoveParcelIDs(vs ...int) *EscrowUpdate { return u }
func (u *EscrowUpdate) ClearParcels() *EscrowUpdate             { return u }
func (u *EscrowUpdate) Save(ctx context.Context) (int, error)   { return 0, nil }

type EscrowUpdateOne struct{}

func (u *EscrowUpdateOne) SetStatusID(v int) *EscrowUpdateOne        { return u }
func (u *EscrowUpdateOne) ClearBio() *EscrowUpdateOne                { return u }
func (u *EscrowUpdateOne) Save(ctx context.Context) (*Escrow, error) { return nil, nil }
