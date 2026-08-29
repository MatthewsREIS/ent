// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	stdsql "database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/schema/field"
)

// fixtureEnum mirrors a schema enum field without a custom GoType (e.g.
// FieldTypeState): a locally-defined string type implementing Stringer.
type fixtureEnum string

func (e fixtureEnum) String() string { return string(e) }

// fixtureScanner mirrors a schema.Link-shaped GoType: implements
// sql.Scanner (pointer receiver) and driver.Valuer (value receiver), but
// isn't one of the standard sql.Null* types.
type fixtureScanner struct{ V string }

func (f *fixtureScanner) Scan(v any) error {
	switch x := v.(type) {
	case nil:
	case string:
		f.V = x
	case []byte:
		f.V = string(x)
	}
	return nil
}

func (f fixtureScanner) Value() (driver.Value, error) { return f.V, nil }

type fixtureJSON struct {
	A int
	B string
}

// fixtureExternal mirrors a field declared with schema/field's external
// ValueScanner option (HasValueScanner): the entity struct field is this
// exact type, but ScanTargets/AssignRow never construct a cell for it
// themselves — they always delegate to FieldSpec.ScanValue/FromValue.
type fixtureExternal string

// fixtureEntity is a hand-built stand-in for a generated internal model
// struct. Field indices mirror internal_model.tmpl's struct emission:
// index 0 is always the embedded Config, index 1 is the ID (when present),
// then fields in declaration order, then unexported FK fields.
type fixtureEntity struct {
	_                struct{} // 0: Config stand-in
	ID               int      // 1
	IntField         int      // 2
	StrField         string   // 3
	BoolField        bool     // 4
	TimeField        time.Time
	NillableIntField *int
	EnumField        fixtureEnum
	BytesField       []byte
	JSONField        fixtureJSON
	ScannerField     fixtureScanner
	NillableScanner  *fixtureScanner
	SensitiveField   string
	ExternalField    fixtureExternal
	fkField          *int
}

func (e *fixtureEntity) SetFK(v int) { e.fkField = &v }

// fixtureDescriptor builds the Descriptor for fixtureEntity, one FieldSpec
// per rule-table row under test.
func fixtureDescriptor() *Descriptor {
	return &Descriptor{
		Name:      "Fixture",
		IDColumn:  "id",
		IDSQLType: field.TypeInt,
		IDType:    reflect.TypeOf(0),
		ScanFields: []FieldSpec{
			{Column: "int_field", Name: "int_field", SQLType: field.TypeInt, Type: reflect.TypeOf(0), StructIndex: 2},
			{Column: "str_field", Name: "str_field", SQLType: field.TypeString, Type: reflect.TypeOf(""), StructIndex: 3},
			{Column: "bool_field", Name: "bool_field", SQLType: field.TypeBool, Type: reflect.TypeOf(false), StructIndex: 4},
			{Column: "time_field", Name: "time_field", SQLType: field.TypeTime, Type: reflect.TypeOf(time.Time{}), StructIndex: 5},
			{Column: "nillable_int", Name: "nillable_int", SQLType: field.TypeInt, Type: reflect.TypeOf(0), Nillable: true, StructIndex: 6},
			{Column: "enum_field", Name: "enum_field", SQLType: field.TypeEnum, Type: reflect.TypeOf(fixtureEnum("")), StructIndex: 7},
			{Column: "bytes_field", Name: "bytes_field", SQLType: field.TypeBytes, Type: reflect.TypeOf([]byte(nil)), StructIndex: 8},
			{Column: "json_field", Name: "json_field", SQLType: field.TypeJSON, Type: reflect.TypeOf(fixtureJSON{}), StructIndex: 9},
			{Column: "scanner_field", Name: "scanner_field", SQLType: field.TypeString, Type: reflect.TypeOf(fixtureScanner{}), StructIndex: 10},
			{Column: "nillable_scanner", Name: "nillable_scanner", SQLType: field.TypeString, Type: reflect.TypeOf(fixtureScanner{}), Nillable: true, StructIndex: 11},
			{Column: "sensitive_field", Name: "sensitive_field", Sensitive: true, StructIndex: 12},
			{
				Column: "external_field", Name: "external_field", SQLType: field.TypeString,
				Type: reflect.TypeOf(fixtureExternal("")), StructIndex: 13, HasValueScanner: true,
				ScanValue: func() any { return &stdsql.NullString{} },
				FromValue: func(v any) (any, error) {
					ns := v.(*stdsql.NullString)
					if !ns.Valid {
						return fixtureExternal(""), nil
					}
					return fixtureExternal(ns.String), nil
				},
			},
		},
		FKColumns: []FieldSpec{
			{Column: "fk_col", SQLType: field.TypeInt, Type: reflect.TypeOf(0), GoName: "SetFK"},
		},
	}
}

func TestScanTargets_CellTypes(t *testing.T) {
	desc := fixtureDescriptor()
	cols := []string{
		"id", "int_field", "str_field", "bool_field", "time_field", "nillable_int",
		"enum_field", "bytes_field", "json_field", "scanner_field", "nillable_scanner",
		"external_field", "fk_col", "unknown_col",
	}
	values, err := ScanTargets(desc, cols)
	if err != nil {
		t.Fatalf("ScanTargets: %v", err)
	}
	want := []any{
		new(stdsql.NullInt64),  // id
		new(stdsql.NullInt64),  // int_field
		new(stdsql.NullString), // str_field
		new(stdsql.NullBool),   // bool_field
		new(stdsql.NullTime),   // time_field
		new(stdsql.NullInt64),  // nillable_int
		new(stdsql.NullString), // enum_field
		new([]byte),            // bytes_field
		new([]byte),            // json_field
	}
	for i, w := range want {
		if got := reflect.TypeOf(values[i]); got != reflect.TypeOf(w) {
			t.Errorf("col %q: got cell type %s, want %s", cols[i], got, reflect.TypeOf(w))
		}
	}
	// scanner_field: direct *fixtureScanner (not Nillable -> no wrap).
	if _, ok := values[9].(*fixtureScanner); !ok {
		t.Errorf("scanner_field: got %T, want *fixtureScanner", values[9])
	}
	// nillable_scanner: Nillable + non-standard-null GoType -> wrapped.
	ns, ok := values[10].(*sql.NullScanner)
	if !ok {
		t.Fatalf("nillable_scanner: got %T, want *sql.NullScanner", values[10])
	}
	if _, ok := ns.S.(*fixtureScanner); !ok {
		t.Errorf("nillable_scanner: NullScanner.S is %T, want *fixtureScanner", ns.S)
	}
	// external_field: exactly what ScanValue() returns.
	if _, ok := values[11].(*stdsql.NullString); !ok {
		t.Errorf("external_field: got %T, want *sql.NullString", values[11])
	}
	// fk_col: plain int FK -> sql.NullInt64.
	if _, ok := values[12].(*stdsql.NullInt64); !ok {
		t.Errorf("fk_col: got %T, want *sql.NullInt64", values[12])
	}
	// unknown_col: escape hatch passthrough.
	if _, ok := values[13].(*sql.UnknownType); !ok {
		t.Errorf("unknown_col: got %T, want *sql.UnknownType", values[13])
	}
}

// scanAndAssign is the test helper mirroring what a real ScanValues/
// AssignValues pair does: build cells via ScanTargets, populate them as
// if rows.Scan had run, then AssignRow them into entity.
func scanAndAssign(t *testing.T, desc *Descriptor, entity any, cols []string, populate func(cells []any)) []struct {
	col string
	val any
} {
	t.Helper()
	cells, err := ScanTargets(desc, cols)
	if err != nil {
		t.Fatalf("ScanTargets: %v", err)
	}
	populate(cells)
	var unknowns []struct {
		col string
		val any
	}
	err = AssignRow(desc, entity, cols, cells, func(c string, v any) {
		unknowns = append(unknowns, struct {
			col string
			val any
		}{c, v})
	})
	if err != nil {
		t.Fatalf("AssignRow: %v", err)
	}
	return unknowns
}

func TestAssignRow_PlainAndNillableInt(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	cols := []string{"id", "int_field", "nillable_int"}
	scanAndAssign(t, desc, e, cols, func(cells []any) {
		cells[0].(*stdsql.NullInt64).Int64, cells[0].(*stdsql.NullInt64).Valid = 1, true
		cells[1].(*stdsql.NullInt64).Int64, cells[1].(*stdsql.NullInt64).Valid = 42, true
		cells[2].(*stdsql.NullInt64).Valid = false // NULL -> leave nil
	})
	if e.ID != 1 || e.IntField != 42 {
		t.Errorf("got ID=%d IntField=%d, want 1, 42", e.ID, e.IntField)
	}
	if e.NillableIntField != nil {
		t.Errorf("NillableIntField = %v, want nil", e.NillableIntField)
	}

	e2 := &fixtureEntity{}
	scanAndAssign(t, desc, e2, []string{"nillable_int"}, func(cells []any) {
		cells[0].(*stdsql.NullInt64).Int64, cells[0].(*stdsql.NullInt64).Valid = 7, true
	})
	if e2.NillableIntField == nil || *e2.NillableIntField != 7 {
		t.Errorf("NillableIntField = %v, want *7", e2.NillableIntField)
	}
}

func TestAssignRow_StringBoolTime(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	scanAndAssign(t, desc, e, []string{"str_field", "bool_field", "time_field"}, func(cells []any) {
		cells[0].(*stdsql.NullString).String, cells[0].(*stdsql.NullString).Valid = "hi", true
		cells[1].(*stdsql.NullBool).Bool, cells[1].(*stdsql.NullBool).Valid = true, true
		cells[2].(*stdsql.NullTime).Time, cells[2].(*stdsql.NullTime).Valid = now, true
	})
	if e.StrField != "hi" || !e.BoolField || !e.TimeField.Equal(now) {
		t.Errorf("got %+v", e)
	}
}

func TestAssignRow_Enum(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"enum_field"}, func(cells []any) {
		cells[0].(*stdsql.NullString).String, cells[0].(*stdsql.NullString).Valid = "active", true
	})
	if e.EnumField != fixtureEnum("active") {
		t.Errorf("EnumField = %q, want %q", e.EnumField, "active")
	}
}

func TestAssignRow_Bytes(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"bytes_field"}, func(cells []any) {
		*cells[0].(*[]byte) = []byte("raw")
	})
	if string(e.BytesField) != "raw" {
		t.Errorf("BytesField = %q, want %q", e.BytesField, "raw")
	}
}

func TestAssignRow_JSON(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"json_field"}, func(cells []any) {
		*cells[0].(*[]byte) = []byte(`{"A":9,"B":"z"}`)
	})
	if e.JSONField != (fixtureJSON{A: 9, B: "z"}) {
		t.Errorf("JSONField = %+v, want {9 z}", e.JSONField)
	}

	// Empty/absent JSON leaves the zero value (matches the generated
	// `value != nil && len(*value) > 0` guard).
	e2 := &fixtureEntity{}
	scanAndAssign(t, desc, e2, []string{"json_field"}, func(cells []any) {})
	if e2.JSONField != (fixtureJSON{}) {
		t.Errorf("JSONField = %+v, want zero value", e2.JSONField)
	}
}

func TestAssignRow_CustomScannerGoType(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"scanner_field"}, func(cells []any) {
		if err := cells[0].(*fixtureScanner).Scan("hello"); err != nil {
			t.Fatal(err)
		}
	})
	if e.ScannerField.V != "hello" {
		t.Errorf("ScannerField.V = %q, want %q", e.ScannerField.V, "hello")
	}
}

func TestAssignRow_NillableScannerGoType(t *testing.T) {
	desc := fixtureDescriptor()

	// NULL column -> stays nil.
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"nillable_scanner"}, func(cells []any) {
		// Valid stays false: simulates a NULL column.
	})
	if e.NillableScanner != nil {
		t.Errorf("NillableScanner = %v, want nil", e.NillableScanner)
	}

	// Non-NULL -> populated.
	e2 := &fixtureEntity{}
	scanAndAssign(t, desc, e2, []string{"nillable_scanner"}, func(cells []any) {
		ns := cells[0].(*sql.NullScanner)
		if err := ns.S.Scan("world"); err != nil {
			t.Fatal(err)
		}
		ns.Valid = true
	})
	if e2.NillableScanner == nil || e2.NillableScanner.V != "world" {
		t.Errorf("NillableScanner = %+v, want &{world}", e2.NillableScanner)
	}
}

func TestAssignRow_ExternalValueScannerRoundTrip(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"external_field"}, func(cells []any) {
		cells[0].(*stdsql.NullString).String, cells[0].(*stdsql.NullString).Valid = "ext", true
	})
	if e.ExternalField != "ext" {
		t.Errorf("ExternalField = %q, want %q", e.ExternalField, "ext")
	}
}

func TestAssignRow_ForeignKeyColumn(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	scanAndAssign(t, desc, e, []string{"fk_col"}, func(cells []any) {
		cells[0].(*stdsql.NullInt64).Int64, cells[0].(*stdsql.NullInt64).Valid = 99, true
	})
	if e.fkField == nil || *e.fkField != 99 {
		t.Errorf("fkField = %v, want *99", e.fkField)
	}
}

func TestAssignRow_UnknownColumnPassthrough(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{}
	unknowns := scanAndAssign(t, desc, e, []string{"mystery"}, func(cells []any) {
		*cells[0].(*sql.UnknownType) = "surprise"
	})
	if len(unknowns) != 1 || unknowns[0].col != "mystery" {
		t.Fatalf("unknowns = %+v, want one entry for %q", unknowns, "mystery")
	}
}

func TestFormatEntity_MatchesGeneratedStringerFormat(t *testing.T) {
	desc := fixtureDescriptor()
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	nInt := 5
	e := &fixtureEntity{
		ID:               1,
		IntField:         42,
		StrField:         "hi",
		BoolField:        true,
		TimeField:        now,
		NillableIntField: &nInt,
		EnumField:        "active",
		BytesField:       []byte("raw"),
		JSONField:        fixtureJSON{A: 9, B: "z"},
		ScannerField:     fixtureScanner{V: "hello"},
		NillableScanner:  &fixtureScanner{V: "world"},
		SensitiveField:   "secret",
		ExternalField:    "ext",
	}
	got := FormatEntity(desc, e)
	want := "Fixture(id=1, " +
		"int_field=42, " +
		"str_field=hi, " +
		"bool_field=true, " +
		"time_field=" + now.Format(time.ANSIC) + ", " +
		"nillable_int=5, " +
		"enum_field=active, " +
		"bytes_field=" + fmtBytes([]byte("raw")) + ", " +
		"json_field=" + fmtJSON(fixtureJSON{A: 9, B: "z"}) + ", " +
		"scanner_field=" + fmtJSON(fixtureScanner{V: "hello"}) + ", " +
		"nillable_scanner=" + fmtJSON(fixtureScanner{V: "world"}) + ", " +
		"sensitive_field=<sensitive>, " +
		"external_field=ext)"
	if got != want {
		t.Errorf("FormatEntity =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatEntity_NillableNilFieldSkipsSegmentNotSeparator(t *testing.T) {
	desc := fixtureDescriptor()
	e := &fixtureEntity{ID: 1, StrField: "x"}
	got := FormatEntity(desc, e)
	// nillable_int has no value ("nillable_int=" is omitted entirely) but
	// its positional ", " separators on both sides remain, matching the
	// generated stringer's unconditional (index-based) separator.
	want := "Fixture(id=1, int_field=0, str_field=x, bool_field=false, " +
		"time_field=" + (time.Time{}).Format(time.ANSIC) + ", , enum_field=, " +
		"bytes_field=[], json_field=" + fmtJSON(fixtureJSON{}) + ", " +
		"scanner_field=" + fmtJSON(fixtureScanner{}) + ", , " +
		"sensitive_field=<sensitive>, external_field=)"
	if got != want {
		t.Errorf("FormatEntity =\n%q\nwant\n%q", got, want)
	}
}

func fmtBytes(b []byte) string { return fmt.Sprintf("%v", b) }
func fmtJSON(v any) string     { return fmt.Sprintf("%v", v) }
