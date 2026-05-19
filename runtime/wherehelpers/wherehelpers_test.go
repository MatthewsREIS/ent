package wherehelpers_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"entgo.io/ent/runtime/wherehelpers"
)

type pred func(int)

func TestAppendPtr_NilValue(t *testing.T) {
	var preds []pred
	var v *int
	got := wherehelpers.AppendPtr(preds, v, func(int) pred { return func(int) {} })
	require.Len(t, got, 0, "nil value must not append")
}

func TestAppendPtr_NonNilValue(t *testing.T) {
	var preds []pred
	v := 42
	got := wherehelpers.AppendPtr(preds, &v, func(int) pred { return func(int) {} })
	require.Len(t, got, 1, "non-nil value must append once")
}

func TestAppendPtr_PreservesExisting(t *testing.T) {
	existing := []pred{func(int) {}, func(int) {}}
	v := 1
	got := wherehelpers.AppendPtr(existing, &v, func(int) pred { return func(int) {} })
	require.Len(t, got, 3, "must preserve existing preds and append new one")
}

func TestAppendSlice_Empty(t *testing.T) {
	var preds []pred
	got := wherehelpers.AppendSlice(preds, []int{}, func(...int) pred { return func(int) {} })
	require.Len(t, got, 0, "empty slice must not append")
}

func TestAppendSlice_NonEmpty(t *testing.T) {
	var preds []pred
	got := wherehelpers.AppendSlice(preds, []int{1, 2, 3}, func(...int) pred { return func(int) {} })
	require.Len(t, got, 1, "non-empty slice must append once")
}

func TestAppendSlice_NilSlice(t *testing.T) {
	var preds []pred
	got := wherehelpers.AppendSlice(preds, ([]int)(nil), func(...int) pred { return func(int) {} })
	require.Len(t, got, 0, "nil slice must not append (len==0)")
}

func TestAppendBool_False(t *testing.T) {
	var preds []pred
	got := wherehelpers.AppendBool(preds, false, func() pred { return func(int) {} })
	require.Len(t, got, 0, "false must not append")
}

func TestAppendBool_True(t *testing.T) {
	var preds []pred
	got := wherehelpers.AppendBool(preds, true, func() pred { return func(int) {} })
	require.Len(t, got, 1, "true must append once")
}
