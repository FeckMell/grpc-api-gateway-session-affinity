package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMyError(t *testing.T) {
	inner := errors.New("underlying")
	e := NewMyError(ErrBadParameter, "invalid input", inner)
	require.NotNil(t, e)
	assert.Equal(t, ErrBadParameter, e.Code)
	assert.Equal(t, "invalid input", e.Message)
	assert.Same(t, inner, e.Inner)
}

func TestNewInternalServerError(t *testing.T) {
	e := NewInternalServerError("db failed", nil)
	require.NotNil(t, e)
	assert.Equal(t, ErrInternalServerError, e.Code)
	assert.Equal(t, "db failed", e.Message)
}

func TestNewBadParameterError(t *testing.T) {
	e := NewBadParameterError("invalid body", nil)
	require.NotNil(t, e)
	assert.Equal(t, ErrBadParameter, e.Code)
	assert.Equal(t, "invalid body", e.Message)
}

func TestToMyError_WithMyError(t *testing.T) {
	e := NewBadParameterError("bad", nil)
	got := ToMyError(e)
	require.NotNil(t, got)
	assert.Same(t, e, got)
}

func TestToMyError_WithOrdinaryError(t *testing.T) {
	e := errors.New("plain")
	got := ToMyError(e)
	assert.Nil(t, got)
}

func TestIsEntityNotFoundError(t *testing.T) {
	e := NewEntityNotFoundError("gone", nil)
	assert.True(t, IsEntityNotFoundError(e))
}
