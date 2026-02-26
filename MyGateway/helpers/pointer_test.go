package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrPanic(t *testing.T) {
	t.Run("empty_panics", func(t *testing.T) {
		assert.PanicsWithValue(t, "str is required", func() {
			StrPanic("", "str is required")
		})
	})
	t.Run("non_empty_returns_value", func(t *testing.T) {
		got := StrPanic("hello", "str is required")
		require.Equal(t, "hello", got)
	})
}

func TestNilPanic(t *testing.T) {
	t.Run("nil_interface_panics", func(t *testing.T) {
		var v interface{} = nil
		assert.PanicsWithValue(t, "interface is required", func() {
			NilPanic(v, "interface is required")
		})
	})
	t.Run("nil_slice_panics", func(t *testing.T) {
		var s []byte = nil
		assert.PanicsWithValue(t, "slice is required", func() {
			NilPanic(s, "slice is required")
		})
	})
	t.Run("nil_map_panics", func(t *testing.T) {
		var m map[string]int = nil
		assert.PanicsWithValue(t, "map is required", func() {
			NilPanic(m, "map is required")
		})
	})
	t.Run("nil_pointer_panics", func(t *testing.T) {
		var p *int = nil
		assert.PanicsWithValue(t, "pointer is required", func() {
			NilPanic(p, "pointer is required")
		})
	})
	t.Run("non_nil_returns_value", func(t *testing.T) {
		s := []byte("ok")
		got := NilPanic(s, "slice is required")
		require.Equal(t, []byte("ok"), got)
	})
	t.Run("non_nil_string_returns_value", func(t *testing.T) {
		got := NilPanic("hello", "str is required")
		require.Equal(t, "hello", got)
	})
}
