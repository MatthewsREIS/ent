// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadOnly_SuppressesUpdateAndDeleteForOneEntity verifies the per-entity
// ReadOnly annotation suppresses update/delete builders for ONLY the
// annotated entity, leaving other entities untouched.
//
// Note on output layout: entc.Generate defaults the target to the parent of
// schemaPath (i.e. the module root), so generated files land at mod/ in the
// sub-package layout (mod/auditlog/update.go, mod/user/update.go, etc.).
func TestReadOnly_SuppressesUpdateAndDeleteForOneEntity(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	mod := writeFeatureFlagsModule(t, "ffreadonly")
	schemaPath := filepath.Join(mod, "schema")
	require.NoError(t, os.MkdirAll(schemaPath, 0o755))

	// AuditLog: read-only.
	require.NoError(t, os.WriteFile(filepath.Join(schemaPath, "auditlog.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/entc"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type AuditLog struct {
	ent.Schema
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{field.String("message")}
}

func (AuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{entc.ReadOnly()}
}
`), 0o644))

	// User: normal entity for control.
	require.NoError(t, os.WriteFile(filepath.Join(schemaPath, "user.go"), []byte(`package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}
`), 0o644))

	target := filepath.Join(mod, "ent")
	// No feature flags -- only per-entity annotation.
	runCodegenWithFeatures(t, mod, target, schemaPath, nil)

	// Codegen now writes to the explicit target (mod/ent) with sub-package layout.
	// Generated sub-package files land at mod/ent/auditlog/ and mod/ent/user/.
	genRoot := target

	// AuditLog has ReadOnly: no update or delete sub-package files.
	auditUpdate, _ := filepath.Glob(filepath.Join(genRoot, "auditlog", "update.go"))
	require.Empty(t, auditUpdate, "AuditLog has ReadOnly annotation; auditlog/update.go must not be generated")
	auditDelete, _ := filepath.Glob(filepath.Join(genRoot, "auditlog", "delete.go"))
	require.Empty(t, auditDelete, "AuditLog has ReadOnly annotation; auditlog/delete.go must not be generated")

	// Also check flat-layout filenames in case the codegen switches style.
	auditUpdateFlat, _ := filepath.Glob(filepath.Join(genRoot, "auditlog_update.go"))
	require.Empty(t, auditUpdateFlat, "AuditLog has ReadOnly annotation; auditlog_update.go must not be generated")
	auditDeleteFlat, _ := filepath.Glob(filepath.Join(genRoot, "auditlog_delete.go"))
	require.Empty(t, auditDeleteFlat, "AuditLog has ReadOnly annotation; auditlog_delete.go must not be generated")

	// AuditLog create builder must still exist (ReadOnly only suppresses write-back operations).
	auditCreate, _ := filepath.Glob(filepath.Join(genRoot, "auditlog", "create.go"))
	auditCreateFlat, _ := filepath.Glob(filepath.Join(genRoot, "auditlog_create.go"))
	require.NotEmpty(t, append(auditCreate, auditCreateFlat...), "AuditLog ReadOnly must not suppress create builder")

	// User has no ReadOnly: both update and delete builders must exist.
	userUpdate, _ := filepath.Glob(filepath.Join(genRoot, "user", "update.go"))
	userUpdateFlat, _ := filepath.Glob(filepath.Join(genRoot, "user_update.go"))
	require.NotEmpty(t, append(userUpdate, userUpdateFlat...), "User has no ReadOnly annotation; update builder must be generated")

	userDelete, _ := filepath.Glob(filepath.Join(genRoot, "user", "delete.go"))
	userDeleteFlat, _ := filepath.Glob(filepath.Join(genRoot, "user_delete.go"))
	require.NotEmpty(t, append(userDelete, userDeleteFlat...), "User has no ReadOnly annotation; delete builder must be generated")
}
