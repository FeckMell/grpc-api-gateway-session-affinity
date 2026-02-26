package service

import (
	"errors"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const msgAllInstancesBusy = "all instances are busy"
const msgBackendUnavailable = "backend service unavailable"
const msgMissingOrInvalidToken = "missing or invalid token"

// GatewayErrorToGRPCStreamInterceptor returns a stream server interceptor: runs the handler and maps the returned error via gatewayErrorToGRPC (table 4.1.4), logs the error for diagnostics.
//
// Parameter logger — logger for "stream handler error" with method and err.
//
// Returns: grpc.StreamServerInterceptor. The error it returns is already a gRPC status (Unavailable, Unauthenticated, ResourceExhausted, etc.).
//
// Called from cmd/main when creating the gRPC server (grpc.ChainStreamInterceptor).
func GatewayErrorToGRPCStreamInterceptor(logger log.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		err := handler(srv, ss)
		if err != nil {
			level.Info(logger).Log(
				"msg", "stream handler error",
				"method", info.FullMethod,
				"err", err,
			)
			err = gatewayErrorToGRPC(err)
		}
		return err
	}
}

// gatewayErrorToGRPC maps handler errors to gRPC status per FR-MGW-5 (table 4.1.4): nil → nil; ErrNoAvailableConnInstance → ResourceExhausted "all instances are busy"; ErrStickyKeyRequired → Unauthenticated "missing or invalid token"; ErrConnPoolClosed/ErrGenericUnknownCluster and any Unavailable → Unavailable "backend service unavailable"; other gRPC status with code != Unknown returned as-is; rest → Unavailable "backend service unavailable".
//
// Parameter err — error returned by handler; nil is allowed.
//
// Returns: nil if err == nil; otherwise *status.Error with the appropriate code and message.
//
// Called from GatewayErrorToGRPCStreamInterceptor after calling the handler.
func gatewayErrorToGRPC(err error) error {
	if err == nil {
		return nil
	}
	// Normalize any Unavailable (e.g. connection/dial errors from backend client, possibly wrapped).
	if status.Code(err) == codes.Unavailable {
		return status.Error(codes.Unavailable, msgBackendUnavailable)
	}
	if s, ok := status.FromError(err); ok && s.Code() != codes.Unknown {
		return s.Err()
	}
	switch {
	case errors.Is(err, ErrNoAvailableConnInstance):
		return status.Error(codes.ResourceExhausted, msgAllInstancesBusy)
	case errors.Is(err, ErrStickyKeyRequired):
		return status.Error(codes.Unauthenticated, msgMissingOrInvalidToken)
	case errors.Is(err, ErrConnPoolClosed), errors.Is(err, ErrGenericUnknownCluster):
		return status.Error(codes.Unavailable, msgBackendUnavailable)
	default:
		return status.Error(codes.Unavailable, msgBackendUnavailable)
	}
}
