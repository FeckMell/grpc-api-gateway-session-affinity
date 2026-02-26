package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEntityNotFoundError(t *testing.T) {
	e := NewEntityNotFoundError("not found", nil)
	assert.Equal(t, ErrEntityNotFound, e.Code)
	assert.Equal(t, "not found", e.Message)
	assert.Nil(t, e.Inner)
}

func TestNewEntityNotFoundError_WithInner(t *testing.T) {
	inner := errors.New("cause")
	e := NewEntityNotFoundError("wrap", inner)
	assert.Equal(t, ErrEntityNotFound, e.Code)
	assert.Equal(t, "wrap", e.Message)
	assert.Same(t, inner, e.Inner)
}

func TestNewInvalidUserOrPasswordError(t *testing.T) {
	e := NewInvalidUserOrPasswordError("invalid", nil)
	assert.Equal(t, ErrInvalidUserOrPassword, e.Code)
	assert.Equal(t, "invalid", e.Message)
}

func TestNewInternalServerError(t *testing.T) {
	e := NewInternalServerError("db failed", nil)
	assert.Equal(t, ErrInternalServerError, e.Code)
	assert.Equal(t, "db failed", e.Message)
}

func TestNewInternalServerError_WithInner(t *testing.T) {
	inner := NewBadParameterError("bad", nil)
	e := NewInternalServerError("wrap", inner)
	assert.Equal(t, ErrInternalServerError, e.Code)
	assert.Equal(t, "wrap", e.Message)
	assert.NotNil(t, e.Inner)
	assert.True(t, IsBadParameter(e.Inner))
}

func TestNewBadParameterError(t *testing.T) {
	e := NewBadParameterError("invalid body", nil)
	assert.Equal(t, ErrBadParameter, e.Code)
	assert.Equal(t, "invalid body", e.Message)
}

func TestAuthError_Error(t *testing.T) {
	e := AuthError{Code: "x", Message: "msg", Inner: nil}
	assert.Equal(t, "x: msg", e.Error())
}

func TestAuthError_Error_WithInner(t *testing.T) {
	inner := errors.New("cause")
	e := AuthError{Code: "x", Message: "msg", Inner: inner}
	assert.Equal(t, "x: msg: cause", e.Error())
}

func TestAuthError_Unwrap(t *testing.T) {
	inner := errors.New("cause")
	e := AuthError{Inner: inner}
	assert.Same(t, inner, e.Unwrap())
}

func TestAuthError_Unwrap_Nil(t *testing.T) {
	e := AuthError{}
	assert.Nil(t, e.Unwrap())
}

func TestIsEntityNotFound(t *testing.T) {
	e := NewEntityNotFoundError("gone", nil)
	assert.True(t, IsEntityNotFound(e))
	assert.False(t, IsEntityNotFound(NewBadParameterError("x", nil)))
	assert.False(t, IsEntityNotFound(errors.New("plain")))
}

func TestIsInvalidUserOrPassword(t *testing.T) {
	e := NewInvalidUserOrPasswordError("invalid", nil)
	assert.True(t, IsInvalidUserOrPassword(e))
	assert.False(t, IsInvalidUserOrPassword(NewEntityNotFoundError("x", nil)))
}

func TestIsInternalServerError(t *testing.T) {
	e := NewInternalServerError("fail", nil)
	assert.True(t, IsInternalServerError(e))
	assert.False(t, IsInternalServerError(NewBadParameterError("x", nil)))
}

func TestIsBadParameter(t *testing.T) {
	e := NewBadParameterError("invalid", nil)
	assert.True(t, IsBadParameter(e))
	assert.False(t, IsBadParameter(NewEntityNotFoundError("x", nil)))
}

func TestErrorsAs_AuthError(t *testing.T) {
	e := NewBadParameterError("bad", nil)
	var authErr AuthError
	require.True(t, errors.As(e, &authErr))
	assert.Equal(t, ErrBadParameter, authErr.Code)
}
