// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	stdsql "database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/schema/field"
)

// This file implements the reflection-driven scan/assign/format machinery
// that backs every entity's generated ScanValues/AssignValues/String method
// bodies (see internal_model.tmpl). Read the task-4 report's conversion rule
// table before changing this file — every branch below exists to reproduce
// one specific row of that table.

// scanKind classifies how a single column is scanned from sql.Rows and
// converted back into the entity struct.
type scanKind uint8

const (
	// scanBasic covers every field without a Scanner-implementing GoType:
	// scan into the sql.Null* (or []byte, for Bytes) cell matching the
	// field's SQLType, then reflect.Convert the underlying primitive to
	// the field's actual Go type (handles named numeric/string/bool/time
	// types, e.g. enums, schema.Int, time.Duration, http.Dir, generically).
	scanBasic scanKind = iota
	// scanJSON covers field.TypeJSON fields without a Scanner GoType:
	// scan into []byte, json.Unmarshal into the field's address.
	scanJSON
	// scanCustom covers fields whose GoType itself implements
	// sql.Scanner+driver.Valuer (schema.Link, uuid.UUID, role.Priority,
	// ...): scan into a fresh *ElemType (optionally sql.NullScanner-
	// wrapped when the field is Nillable and ElemType isn't already one
	// of the standard sql.Null* types), then assign the pointer (or its
	// dereferenced elem) into the struct field.
	scanCustom
	// scanExternal covers fields declared with an external ValueScanner
	// (schema/field's Field.ValueScanner option): dispatch entirely
	// through the field's ScanValue/FromValue closures.
	scanExternal
)

// fieldPlan is the precomputed, entity-independent scan/assign strategy for
// one column. Built once per Descriptor (see Descriptor.scanOnce) and reused
// across every query — only the final reflect.Value writes touch the
// concrete entity.
type fieldPlan struct {
	spec FieldSpec
	kind scanKind

	// scanCustom only.
	elemType reflect.Type // concrete (non-pointer) Scanner-implementing type
	wrapNull bool         // wrap the cell in a *sql.NullScanner

	// isFK marks a plan built from Descriptor.FKColumns: the target struct
	// field is unexported, so assignment goes through the entity's
	// generated exported setter (FieldSpec.GoName holds the setter's
	// method name, e.g. "SetUserSpouse") via reflect method Call instead
	// of a direct field Set.
	isFK bool
}

var (
	scannerType = reflect.TypeOf((*stdsql.Scanner)(nil)).Elem()
	valuerType  = reflect.TypeOf((*driver.Valuer)(nil)).Elem()
	timeType    = reflect.TypeOf(time.Time{})

	standardNullTypes = []reflect.Type{
		reflect.TypeOf(stdsql.NullBool{}),
		reflect.TypeOf(stdsql.NullFloat64{}),
		reflect.TypeOf(stdsql.NullInt32{}),
		reflect.TypeOf(stdsql.NullInt64{}),
		reflect.TypeOf(stdsql.NullTime{}),
		reflect.TypeOf(stdsql.NullString{}),
	}
)

// isValueScanner reports whether t (or *t, for the common case of a
// pointer-receiver Scan method) implements both sql.Scanner and
// driver.Valuer — mirrors entc/gen's Field.Type.ValueScanner(), evaluated
// here against a real reflect.Type instead of the serializable RType.
func isValueScanner(t reflect.Type) bool {
	if t == nil {
		return false
	}
	pt := t
	if pt.Kind() != reflect.Ptr {
		pt = reflect.PointerTo(t)
	}
	return pt.Implements(scannerType) && (t.Implements(valuerType) || pt.Implements(valuerType))
}

// concreteElem returns t's pointee when t is a pointer type, else t itself.
func concreteElem(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}

// isStandardNullType reports if t (after a single pointer indirection) is
// one of the sql package's built-in Null* wrapper types.
func isStandardNullType(t reflect.Type) bool {
	t = concreteElem(t)
	for _, nt := range standardNullTypes {
		if t == nt {
			return true
		}
	}
	return false
}

// buildFieldPlan classifies a single FieldSpec, matching the precedence
// entc/gen's decode template used: an external ValueScanner first, then
// IsJSON, then a GoType implementing Scanner/Valuer, else the basic-kind
// fallback. See the task-4 report's rule table for the full matrix.
func buildFieldPlan(spec FieldSpec, isFK bool) *fieldPlan {
	p := &fieldPlan{spec: spec, isFK: isFK}
	switch {
	case spec.HasValueScanner:
		p.kind = scanExternal
	case spec.SQLType == field.TypeJSON:
		p.kind = scanJSON
	case isValueScanner(spec.Type):
		p.kind = scanCustom
		p.elemType = concreteElem(spec.Type)
		p.wrapNull = spec.Nillable && !isStandardNullType(p.elemType)
	default:
		p.kind = scanBasic
	}
	return p
}

// plansFor returns (building and caching on first use) the by-column plan
// map for desc, covering the ID column, every entry in ScanFields, and
// every entry in FKColumns.
func plansFor(desc *Descriptor) map[string]*fieldPlan {
	desc.scanOnce.Do(func() {
		m := make(map[string]*fieldPlan, len(desc.ScanFields)+len(desc.FKColumns)+1)
		if desc.IDColumn != "" {
			// Struct index 1: every entity struct embeds Config at index
			// 0, and a single-field ID (when present) is always the very
			// next field — see internal_model.tmpl's struct emission.
			m[desc.IDColumn] = buildFieldPlan(FieldSpec{
				Column:      desc.IDColumn,
				SQLType:     desc.IDSQLType,
				Type:        desc.IDType,
				StructIndex: 1,
			}, false)
		}
		for _, f := range desc.ScanFields {
			m[f.Column] = buildFieldPlan(f, false)
		}
		for _, f := range desc.FKColumns {
			m[f.Column] = buildFieldPlan(f, true)
		}
		desc.scanPlans = m
	})
	return desc.scanPlans
}

// newScanCell returns the destination object rows.Scan should populate for
// this column.
func (p *fieldPlan) newScanCell() any {
	switch p.kind {
	case scanExternal:
		return p.spec.ScanValue()
	case scanJSON:
		return new([]byte)
	case scanCustom:
		cell := reflect.New(p.elemType).Interface()
		if p.wrapNull {
			return &sql.NullScanner{S: cell.(stdsql.Scanner)}
		}
		return cell
	default:
		return newBasicCell(p.spec.SQLType)
	}
}

// newBasicCell returns the sql.Null*/[]byte scan destination for a plain
// (non-Scanner, non-JSON) field, keyed by its logical SQL kind.
func newBasicCell(t field.Type) any {
	switch {
	case t == field.TypeBytes:
		return new([]byte)
	case t == field.TypeString || t == field.TypeEnum:
		return new(stdsql.NullString)
	case t == field.TypeBool:
		return new(stdsql.NullBool)
	case t == field.TypeTime:
		return new(stdsql.NullTime)
	case t.Numeric() && t.Float():
		return new(stdsql.NullFloat64)
	case t.Numeric():
		return new(stdsql.NullInt64)
	default:
		// TypeOther/TypeUUID without a Scanner GoType, or any other kind
		// this table can't express generically — the escape hatch: treat
		// exactly like an unrecognized column (default: selectValues.Set).
		return new(sql.UnknownType)
	}
}

// ScanTargets returns the []any that sql.Rows.Scan should populate for the
// given columns. Unrecognized columns (computed/select-only columns, or a
// field this table can't classify) fall back to sql.UnknownType.
func ScanTargets(desc *Descriptor, columns []string) ([]any, error) {
	plans := plansFor(desc)
	values := make([]any, len(columns))
	for i, c := range columns {
		p, ok := plans[c]
		if !ok {
			values[i] = new(sql.UnknownType)
			continue
		}
		values[i] = p.newScanCell()
	}
	return values, nil
}

// AssignRow assigns values (as populated by ScanTargets + rows.Scan) into
// entity's fields, keyed by columns. entity must be a non-nil pointer to
// the generated entity struct. unknown, if non-nil, receives every column
// AssignRow doesn't recognize (mirrors the generated method's previous
// `default: _m.selectValues.Set(...)` case) — deliberately a callback
// rather than reflection into the entity's unexported selectValues field,
// which entbuilder (a different package) can't reach directly.
func AssignRow(desc *Descriptor, entity any, columns []string, values []any, unknown func(column string, value any)) error {
	if m, n := len(values), len(columns); m < n {
		return fmt.Errorf("entbuilder: mismatch number of scan values: %d != %d", m, n)
	}
	ev := reflect.ValueOf(entity)
	if ev.Kind() != reflect.Ptr || ev.IsNil() {
		return fmt.Errorf("entbuilder: AssignRow requires a non-nil entity pointer, got %T", entity)
	}
	plans := plansFor(desc)
	for i, c := range columns {
		p, ok := plans[c]
		if !ok {
			if unknown != nil {
				unknown(c, values[i])
			}
			continue
		}
		if err := p.assign(ev, values[i]); err != nil {
			return err
		}
	}
	return nil
}

// assign converts and writes one scanned value into entity (ev, the
// original pointer Value — needed as-is for method calls on FK setters,
// and dereferenced for direct struct-field writes).
func (p *fieldPlan) assign(ev reflect.Value, value any) error {
	switch p.kind {
	case scanExternal:
		v, err := p.spec.FromValue(value)
		if err != nil {
			return err
		}
		return p.writeDirect(ev, reflect.ValueOf(v))
	case scanJSON:
		cell, ok := value.(*[]byte)
		if !ok {
			return fmt.Errorf("entbuilder: unexpected type %T for field %s", value, p.spec.Column)
		}
		if cell == nil || len(*cell) == 0 {
			return nil
		}
		if p.isFK {
			// Not reachable in practice (FK targets are never JSON), but
			// keep the error explicit rather than silently doing nothing.
			return fmt.Errorf("entbuilder: JSON scan unsupported for foreign-key field %s", p.spec.Column)
		}
		return json.Unmarshal(*cell, ev.Elem().Field(p.spec.StructIndex).Addr().Interface())
	case scanCustom:
		return p.assignCustom(ev, value)
	default:
		return p.assignBasic(ev, value)
	}
}

// assignCustom handles the scanCustom cell shapes: either a direct
// *ElemType (already a sql.Scanner) or a *sql.NullScanner wrapping one.
func (p *fieldPlan) assignCustom(ev reflect.Value, value any) error {
	var raw reflect.Value
	if p.wrapNull {
		ns, ok := value.(*sql.NullScanner)
		if !ok {
			return fmt.Errorf("entbuilder: unexpected type %T for field %s", value, p.spec.Column)
		}
		if !ns.Valid {
			return nil
		}
		raw = reflect.ValueOf(ns.S)
	} else {
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Ptr || rv.IsNil() {
			return nil
		}
		raw = rv
	}
	// write() re-wraps into a pointer itself when the target struct field
	// (or FK setter — never applicable here) needs one, so always hand it
	// the dereferenced concrete value.
	return p.writeDirect(ev, raw.Elem())
}

// assignBasic handles every field.Type that isn't JSON or backed by a
// Scanner GoType: extract the underlying primitive from the sql.Null* (or
// []byte) cell, then reflect.Convert it to the field's actual Go type.
func (p *fieldPlan) assignBasic(ev reflect.Value, value any) error {
	switch v := value.(type) {
	case *[]byte:
		if v == nil || *v == nil {
			return nil
		}
		return p.writeDirect(ev, reflect.ValueOf(*v))
	case *stdsql.NullString:
		if !v.Valid {
			return nil
		}
		return p.writeDirect(ev, reflect.ValueOf(v.String))
	case *stdsql.NullBool:
		if !v.Valid {
			return nil
		}
		return p.writeDirect(ev, reflect.ValueOf(v.Bool))
	case *stdsql.NullTime:
		if !v.Valid {
			return nil
		}
		return p.writeDirect(ev, reflect.ValueOf(v.Time))
	case *stdsql.NullInt64:
		if !v.Valid {
			return nil
		}
		return p.writeDirect(ev, reflect.ValueOf(v.Int64))
	case *stdsql.NullFloat64:
		if !v.Valid {
			return nil
		}
		return p.writeDirect(ev, reflect.ValueOf(v.Float64))
	default:
		// sql.UnknownType or some other passthrough cell landed on a
		// recognized column (e.g. a modifier overriding it). Unreachable in
		// practice — ScanTargets always pairs this column with one of the
		// sql.Null*/[]byte cells above, per newBasicCell, so AssignRow only
		// ever sees what it asked for. Note this is NOT what the old
		// generated decode block did: it returned
		// fmt.Errorf("unexpected type %T for field %s", ...) for exactly
		// this case (see the deleted internal_model.tmpl decode block).
		// Ignoring here rather than erroring is a deliberate call for an
		// unreachable path, not a claim of behavioral parity.
		return nil
	}
}

// writeDirect stores value into the field: a method Call for an FK's
// exported setter, or a struct-field Set otherwise. value's type may
// already be an exact (or convertible) match for the target — including
// itself being a pointer, e.g. a field.TypeValueScanner[*url.URL]'s
// FromValue returning a *url.URL directly for a GoType(&url.URL{}) field —
// in which case it's used as-is; otherwise, for a Nillable value-kind
// target, a fresh pointer is allocated and value converted into it.
// Trying the direct (non-pointer-target) match first, before ever
// considering a pointer-alloc, is what makes this safe for both shapes.
func (p *fieldPlan) writeDirect(ev reflect.Value, value reflect.Value) error {
	if p.isFK {
		target := p.spec.Type
		converted, err := convertTo(value, target)
		if err != nil {
			return fmt.Errorf("entbuilder: %w for field %s", err, p.spec.Column)
		}
		method := ev.MethodByName(p.spec.GoName)
		if !method.IsValid() {
			return fmt.Errorf("entbuilder: missing setter %s for foreign-key field %s", p.spec.GoName, p.spec.Column)
		}
		method.Call([]reflect.Value{converted})
		return nil
	}
	fv := ev.Elem().Field(p.spec.StructIndex)
	if converted, err := convertTo(value, fv.Type()); err == nil {
		fv.Set(converted)
		return nil
	}
	if fv.Kind() == reflect.Ptr {
		if converted, err := convertTo(value, fv.Type().Elem()); err == nil {
			ptr := reflect.New(fv.Type().Elem())
			ptr.Elem().Set(converted)
			fv.Set(ptr)
			return nil
		}
	}
	return fmt.Errorf("entbuilder: cannot convert %s to %s for field %s", value.Type(), fv.Type(), p.spec.Column)
}

// convertTo converts value to target, preferring a direct (identity or
// implicit) assignment over reflect.Convert — matters for e.g. a named
// []byte-kind GoType, which reflect.Convert would also handle, but where
// staying on the assignable path avoids any ambiguity.
func convertTo(value reflect.Value, target reflect.Type) (reflect.Value, error) {
	switch {
	case value.Type().AssignableTo(target):
		return value, nil
	case value.Type().ConvertibleTo(target):
		return value.Convert(target), nil
	default:
		return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", value.Type(), target)
	}
}

// isNilableKind reports whether k is a kind reflect.Value.IsNil accepts
// (Chan/Func/Interface/Map/Ptr/Slice/UnsafePointer) — calling IsNil on any
// other kind panics.
func isNilableKind(k reflect.Kind) bool {
	switch k {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

// FormatEntity reproduces the generated String() method's output exactly:
// "Name(id=1, field=value, ...)", redacting Sensitive fields as
// "<sensitive>", formatting a plain time.Time field with time.ANSIC, and
// skipping a Nillable field's "name=value" segment entirely (but not its
// positional ", " separator) when its pointer is nil.
func FormatEntity(desc *Descriptor, entity any) string {
	rv := reflect.ValueOf(entity)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	var b strings.Builder
	b.WriteString(desc.Name)
	b.WriteByte('(')
	if desc.IDColumn != "" {
		if len(desc.ScanFields) > 0 {
			fmt.Fprintf(&b, "id=%v, ", rv.Field(1).Interface())
		} else {
			fmt.Fprintf(&b, "id=%v", rv.Field(1).Interface())
		}
	}
	for i, f := range desc.ScanFields {
		if i > 0 {
			b.WriteString(", ")
		}
		if f.Sensitive {
			b.WriteString(f.Name)
			b.WriteString("=<sensitive>")
			continue
		}
		fv := rv.Field(f.StructIndex)
		// f.Nillable means "pointer struct field" in this (ScanFields)
		// collection — see FieldSpec.Nillable's doc comment. IsNil panics
		// on a non-nilable kind, so guard by kind rather than trusting the
		// bit: this is the one place a Fields-shaped spec (where Nillable
		// means "clearable" and doesn't imply a pointer struct field) could
		// otherwise reach IsNil and panic.
		if f.Nillable && isNilableKind(fv.Kind()) {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		b.WriteString(f.Name)
		b.WriteByte('=')
		if fv.Type() == timeType {
			b.WriteString(fv.Interface().(time.Time).Format(time.ANSIC))
		} else {
			fmt.Fprintf(&b, "%v", fv.Interface())
		}
	}
	b.WriteByte(')')
	return b.String()
}
