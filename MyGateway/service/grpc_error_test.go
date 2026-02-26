package service

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGatewayErrorToGRPC_nil(t *testing.T) {
	assert.NoError(t, gatewayErrorToGRPC(nil))
}

func TestGatewayErrorToGRPC_ErrNoAvailableConnInstance(t *testing.T) {
	err := gatewayErrorToGRPC(ErrNoAvailableConnInstance)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, s.Code())
	assert.Equal(t, msgAllInstancesBusy, s.Message())
}

func TestGatewayErrorToGRPC_ErrNoAvailableConnInstanceWrapped(t *testing.T) {
	wrapped := errors.Join(ErrNoAvailableConnInstance, errors.New("extra"))
	err := gatewayErrorToGRPC(wrapped)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, s.Code())
	assert.Equal(t, msgAllInstancesBusy, s.Message())
}

func TestGatewayErrorToGRPC_ErrStickyKeyRequired(t *testing.T) {
	err := gatewayErrorToGRPC(ErrStickyKeyRequired)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, s.Code())
	assert.Equal(t, msgMissingOrInvalidToken, s.Message())
}

func TestGatewayErrorToGRPC_ErrConnPoolClosed(t *testing.T) {
	err := gatewayErrorToGRPC(ErrConnPoolClosed)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
	assert.Equal(t, msgBackendUnavailable, s.Message())
}

func TestGatewayErrorToGRPC_ErrGenericUnknownCluster(t *testing.T) {
	err := gatewayErrorToGRPC(errors.Join(ErrGenericUnknownCluster, errors.New("cluster x")))
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
	assert.Equal(t, msgBackendUnavailable, s.Message())
}

func TestGatewayErrorToGRPC_arbitraryError(t *testing.T) {
	plain := errors.New("some backend failure")
	err := gatewayErrorToGRPC(plain)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
	assert.Equal(t, msgBackendUnavailable, s.Message())
}

func TestGatewayErrorToGRPC_existingStatusPreserved(t *testing.T) {
	orig := status.Error(codes.Unauthenticated, "missing session-id")
	err := gatewayErrorToGRPC(orig)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, s.Code())
	assert.Equal(t, "missing session-id", s.Message())
}

func TestGatewayErrorToGRPC_existingStatusInternalPreserved(t *testing.T) {
	orig := status.Error(codes.Internal, "missing grpc method in stream context")
	err := gatewayErrorToGRPC(orig)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
}

func TestGatewayErrorToGRPC_existingStatusUnavailableNormalized(t *testing.T) {
	orig := status.Error(codes.Unavailable, "connection error: desc = \"transport: Error while dialing: dial tcp 192.168.176.5:5000: connect: no route to host\"")
	err := gatewayErrorToGRPC(orig)
	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
	assert.Equal(t, msgBackendUnavailable, s.Message(), "Unavailable from backend must be normalized to FR-MGW-5 message")
}

// fakeServerStream is a minimal grpc.ServerStream for testing the interceptor.
type fakeServerStream struct {
	ctx context.Context
}

func (f *fakeServerStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeServerStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeServerStream) SetTrailer(metadata.MD)       {}
func (f *fakeServerStream) Context() context.Context     { return f.ctx }
func (f *fakeServerStream) SendMsg(interface{}) error    { return nil }
func (f *fakeServerStream) RecvMsg(interface{}) error    { return io.EOF }

func TestGatewayErrorToGRPCStreamInterceptor_handlerReturnsNil(t *testing.T) {
	interceptor := GatewayErrorToGRPCStreamInterceptor(log.NewNopLogger())
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Method"}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}
	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
}

func TestGatewayErrorToGRPCStreamInterceptor_handlerReturnsErrNoAvailableConnInstance(t *testing.T) {
	interceptor := GatewayErrorToGRPCStreamInterceptor(log.NewNopLogger())
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Method"}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return ErrNoAvailableConnInstance
	}
	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, s.Code())
	assert.Equal(t, msgAllInstancesBusy, s.Message())
}

func TestGatewayErrorToGRPCStreamInterceptor_handlerReturnsErrStickyKeyRequired(t *testing.T) {
	interceptor := GatewayErrorToGRPCStreamInterceptor(log.NewNopLogger())
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Method"}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return ErrStickyKeyRequired
	}
	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, s.Code())
	assert.Equal(t, msgMissingOrInvalidToken, s.Message())
}

func TestGatewayErrorToGRPCStreamInterceptor_handlerReturnsArbitraryError(t *testing.T) {
	interceptor := GatewayErrorToGRPCStreamInterceptor(log.NewNopLogger())
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Method"}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return errors.New("backend dial failed")
	}
	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
	assert.Equal(t, msgBackendUnavailable, s.Message())
}

func TestGatewayErrorToGRPCStreamInterceptor_handlerReturnsExistingStatus(t *testing.T) {
	interceptor := GatewayErrorToGRPCStreamInterceptor(log.NewNopLogger())
	ss := &fakeServerStream{ctx: context.Background()}
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Method"}
	orig := status.Error(codes.Unimplemented, "method not routed")
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return orig
	}
	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, s.Code())
	assert.Equal(t, "method not routed", s.Message())
}
