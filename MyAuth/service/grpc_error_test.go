package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuthErrorToGRPC_Nil(t *testing.T) {
	got := AuthErrorToGRPC(nil)
	assert.NoError(t, got)
}

func TestAuthErrorToGRPC_BadParameter(t *testing.T) {
	err := NewBadParameterError("username is required", nil)
	got := AuthErrorToGRPC(err)
	require.Error(t, got)
	st, ok := status.FromError(got)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "username is required", st.Message())
}

func TestAuthErrorToGRPC_InvalidUserOrPassword(t *testing.T) {
	err := NewInvalidUserOrPasswordError("invalid username or password", nil)
	got := AuthErrorToGRPC(err)
	require.Error(t, got)
	st, ok := status.FromError(got)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
	assert.Equal(t, "invalid username or password", st.Message())
}

func TestAuthErrorToGRPC_InternalServerError(t *testing.T) {
	err := NewInternalServerError("failed to get user", nil)
	got := AuthErrorToGRPC(err)
	require.Error(t, got)
	st, ok := status.FromError(got)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "failed to get user", st.Message())
}

func TestAuthErrorToGRPC_EntityNotFound(t *testing.T) {
	err := NewEntityNotFoundError("user not found", nil)
	got := AuthErrorToGRPC(err)
	require.Error(t, got)
	st, ok := status.FromError(got)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, "user not found", st.Message())
}

func TestAuthErrorToGRPC_UnknownError(t *testing.T) {
	err := errors.New("any")
	got := AuthErrorToGRPC(err)
	require.Error(t, got)
	st, ok := status.FromError(got)
	require.True(t, ok)
	assert.Equal(t, codes.Unknown, st.Code())
	assert.Equal(t, "internal error", st.Message())
}
