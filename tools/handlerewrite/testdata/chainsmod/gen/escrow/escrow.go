// Package escrow stands in for a Task-1 F/E-handle entity subpackage: a
// real, importable package at the import path the -chains fixture tests
// reference via manifest "importPath", so the import-insertion and
// alias-reuse tests exercise a resolvable import. handlerewrite's -chains
// mode never inspects this package's contents — it only needs the import
// path to exist and be resolvable by go/packages.
package escrow

// Marker exists only so a consumer file can reference this package (Go
// requires imports be used); -chains mode never inspects it.
type Marker struct{}
