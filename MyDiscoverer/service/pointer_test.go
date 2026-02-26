package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPtr(t *testing.T) {
	s := "hello"
	p := Ptr(s)
	require.NotNil(t, p)
	assert.Equal(t, s, *p)
}

func TestValue(t *testing.T) {
	x := 42
	assert.Equal(t, 42, Value(&x))
}

func TestValue_Nil(t *testing.T) {
	assert.Equal(t, 0, Value[int](nil))
	assert.Equal(t, "", Value[string](nil))
}
