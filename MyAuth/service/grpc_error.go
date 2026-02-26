package service

import (
	"context"
	"errors"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// authErrorCodeToGRPCCode maps AuthError codes to gRPC status codes.
func authErrorCodeToGRPCCode(code string) codes.Code {
	switch code {
	case ErrBadParameter:
		return codes.InvalidArgument
	case ErrInvalidUserOrPassword:
		return codes.PermissionDenied
	case ErrInternalServerError:
		return codes.Internal
	case ErrEntityNotFound:
		return codes.NotFound
	default:
		return codes.Unknown
	}
}

// AuthErrorToGRPC converts an error to a gRPC status error. AuthError is mapped to the
// corresponding gRPC code and message; other errors become codes.Unknown with "internal error".
func AuthErrorToGRPC(err error) error {
	if err == nil {
		return nil
	}
	var authErr AuthError
	if errors.As(err, &authErr) {
		return status.Error(authErrorCodeToGRPCCode(authErr.Code), authErr.Message)
	}
	return status.Error(codes.Unknown, "internal error")
}

// AuthErrorToGRPCInterceptor returns a unary server interceptor that converts handler
// errors to gRPC status errors and logs all errors for diagnostics.
func AuthErrorToGRPCInterceptor(logger log.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			var authErr AuthError
			if errors.As(err, &authErr) {
				level.Info(logger).Log(
					"msg", "gRPC handler error",
					"method", info.FullMethod,
					"error_code", authErr.Code,
					"error_message", authErr.Message,
					"error", err,
				)
			} else {
				level.Error(logger).Log(
					"msg", "gRPC handler error",
					"method", info.FullMethod,
					"err", err,
				)
			}
			err = AuthErrorToGRPC(err)
		}
		return resp, err
	}
}
