// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"database/sql/driver"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

// EdgeLoadDescriptor describes how to load edges for a query.
// This is used to reduce generated code by extracting common edge loading patterns.
type EdgeLoadDescriptor[N, E any, NID, EID comparable] struct {
	// EdgeSpec provides the sqlgraph edge specification.
	EdgeSpec func() *sqlgraph.EdgeSpec

	// ExtractNodeID gets the ID from a source node.
	ExtractNodeID func(*N) NID

	// ExtractEdgeID gets the ID from an edge node.
	ExtractEdgeID func(*E) EID

	// ExtractNodeFK gets the foreign key from source node (for M2O edges).
	// Returns nil if FK is not set.
	ExtractNodeFK func(*N) *EID

	// ExtractEdgeFK gets the foreign key from edge node (for O2M/O2O edges).
	// Returns nil if FK is not set.
	ExtractEdgeFK func(*E) *NID

	// ConvertNodeIDFromScan converts scanned value to node ID type (for M2M).
	ConvertNodeIDFromScan func(any) NID

	// ConvertEdgeIDFromScan converts scanned value to edge ID type (for M2M).
	ConvertEdgeIDFromScan func(any) EID

	// NewNodeIDScanner creates a new scanner for node ID type (for M2M).
	NewNodeIDScanner func() any

	// NewEdgeIDScanner creates a new scanner for edge ID type (for M2M).
	NewEdgeIDScanner func() any
}

// querierFunc is a function type that implements the querier interface.
type querierFunc func(context.Context, any) (any, error)

// Query calls the function.
func (f querierFunc) Query(ctx context.Context, q any) (any, error) {
	return f(ctx, q)
}

// LoadEdgeM2M loads many-to-many edges using a join table.
// This handles the complex case where edges go through an intermediate table.
// All query operations are provided as function parameters to avoid generic interface issues.
func LoadEdgeM2M[N, E any, NID, EID comparable](
	ctx context.Context,
	desc *EdgeLoadDescriptor[N, E, NID, EID],
	nodes []*N,
	init func(*N),
	assign func(*N, *E),
	pkIndices [2]int, // [sourceIdx, targetIdx] in primary key columns
	addWhere func(func(*sql.Selector)),
	prepareQuery func(context.Context) error,
	sqlAll func(context.Context, ...func(context.Context, *sqlgraph.QuerySpec)) ([]*E, error),
	withInterceptors func(context.Context, any, any, any) (any, error),
	query any,
	interceptors any,
) error {
	if len(nodes) == 0 {
		return nil
	}

	edgeIDs := make([]driver.Value, len(nodes))
	byID := make(map[NID]*N)
	nids := make(map[EID]map[*N]struct{})

	for i, node := range nodes {
		id := desc.ExtractNodeID(node)
		edgeIDs[i] = id
		byID[id] = node
		if init != nil {
			init(node)
		}
	}

	// Build the join query
	spec := desc.EdgeSpec()
	columns := spec.Columns
	addWhere(func(s *sql.Selector) {
		joinT := sql.Table(spec.Table)
		s.Join(joinT).On(s.C(spec.Target.IDSpec.Column), joinT.C(columns[pkIndices[1]]))
		s.Where(sql.InValues(joinT.C(columns[pkIndices[0]]), edgeIDs...))
		selectedCols := s.SelectedColumns()
		s.Select(joinT.C(columns[pkIndices[0]]))
		s.AppendSelect(selectedCols...)
		s.SetDistinct(false)
	})

	if err := prepareQuery(ctx); err != nil {
		return err
	}

	// Build a querier that executes with custom scan/assign to capture join table IDs
	type querier interface {
		Query(context.Context, any) (any, error)
	}
	qr := querierFunc(func(ctx context.Context, _ any) (any, error) {
		return sqlAll(ctx, func(_ context.Context, spec *sqlgraph.QuerySpec) {
			origAssign := spec.Assign
			origScanValues := spec.ScanValues

			spec.ScanValues = func(columns []string) ([]any, error) {
				values, err := origScanValues(columns[1:])
				if err != nil {
					return nil, err
				}
				// Prepend slot for source ID from join table
				return append([]any{desc.NewNodeIDScanner()}, values...), nil
			}

			spec.Assign = func(columns []string, values []any) error {
				// Extract source and target IDs from join table scan
				sourceID := desc.ConvertNodeIDFromScan(values[0])
				targetID := desc.ConvertEdgeIDFromScan(values[1])

				// Track which source nodes this target belongs to
				if nids[targetID] == nil {
					nids[targetID] = map[*N]struct{}{byID[sourceID]: {}}
					// First time seeing this edge - populate it
					return origAssign(columns[1:], values[1:])
				}
				// Already seen this edge, just track the additional source node
				nids[targetID][byID[sourceID]] = struct{}{}
				return nil
			}
		})
	})

	// Execute with interceptors
	result, err := withInterceptors(ctx, query, qr, interceptors)
	if err != nil {
		return err
	}
	neighbors := result.([]*E)

	// Map neighbors back to source nodes
	for _, edge := range neighbors {
		edgeID := desc.ExtractEdgeID(edge)
		sourceNodes, ok := nids[edgeID]
		if !ok {
			return fmt.Errorf(`unexpected edge node returned %v`, edgeID)
		}
		for node := range sourceNodes {
			assign(node, edge)
		}
	}

	return nil
}

// LoadEdgeO2M loads one-to-many edges where the target entity has the foreign key.
// All query operations are provided as function parameters.
func LoadEdgeO2M[N, E any, NID, EID comparable](
	ctx context.Context,
	desc *EdgeLoadDescriptor[N, E, NID, EID],
	nodes []*N,
	init func(*N),
	assign func(*N, *E),
	setWithFKs func(bool),
	addWhere func(func(*sql.Selector)),
	queryAll func(context.Context) ([]*E, error),
) error {
	if len(nodes) == 0 {
		return nil
	}

	fks := make([]driver.Value, 0, len(nodes))
	nodeids := make(map[NID]*N)

	for _, node := range nodes {
		id := desc.ExtractNodeID(node)
		fks = append(fks, id)
		nodeids[id] = node
		if init != nil {
			init(node)
		}
	}

	setWithFKs(true)
	spec := desc.EdgeSpec()
	column := spec.Columns[0]

	addWhere(func(s *sql.Selector) {
		s.Where(sql.InValues(s.C(column), fks...))
	})

	neighbors, err := queryAll(ctx)
	if err != nil {
		return err
	}

	for _, edge := range neighbors {
		fk := desc.ExtractEdgeFK(edge)
		if fk == nil {
			return fmt.Errorf(`foreign-key is nil for edge node`)
		}
		node, ok := nodeids[*fk]
		if !ok {
			return fmt.Errorf(`unexpected referenced foreign-key returned %v`, *fk)
		}
		assign(node, edge)
	}

	return nil
}

// LoadEdgeO2O loads one-to-one edges where the target entity has the foreign key.
// This is the same as O2M but without the init callback.
func LoadEdgeO2O[N, E any, NID, EID comparable](
	ctx context.Context,
	desc *EdgeLoadDescriptor[N, E, NID, EID],
	nodes []*N,
	assign func(*N, *E),
	setWithFKs func(bool),
	addWhere func(func(*sql.Selector)),
	queryAll func(context.Context) ([]*E, error),
) error {
	return LoadEdgeO2M(ctx, desc, nodes, nil, assign, setWithFKs, addWhere, queryAll)
}

// LoadEdgeM2O loads many-to-one edges where the source entity has the foreign key.
// All query operations are provided as function parameters.
func LoadEdgeM2O[N, E any, NID, EID comparable](
	ctx context.Context,
	desc *EdgeLoadDescriptor[N, E, NID, EID],
	nodes []*N,
	assign func(*N, *E),
	queryWhere func([]EID), // Callback to add ID filtering to query
	queryAll func(context.Context) ([]*E, error),
) error {
	if len(nodes) == 0 {
		return nil
	}

	ids := make([]EID, 0, len(nodes))
	nodeids := make(map[EID][]*N)

	for _, node := range nodes {
		fk := desc.ExtractNodeFK(node)
		if fk == nil {
			continue
		}
		if _, ok := nodeids[*fk]; !ok {
			ids = append(ids, *fk)
		}
		nodeids[*fk] = append(nodeids[*fk], node)
	}

	if len(ids) == 0 {
		return nil
	}

	// Let caller add ID filtering (entity-specific)
	queryWhere(ids)

	neighbors, err := queryAll(ctx)
	if err != nil {
		return err
	}

	for _, edge := range neighbors {
		id := desc.ExtractEdgeID(edge)
		nodes, ok := nodeids[id]
		if !ok {
			return fmt.Errorf(`unexpected foreign-key returned %v`, id)
		}
		for _, node := range nodes {
			assign(node, edge)
		}
	}

	return nil
}
