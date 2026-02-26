package interfaces

import (
	"context"

	"mygateway/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ConnectionResolver provides a backend gRPC connection for a (route, headers) and reports backend failures.
// GetConnection returns a *grpc.ClientConn, the sticky key (if any), and the instance ID; OnBackendFailure
// notifies the resolver that a backend stream failed so it can unbind sticky sessions and close/unregister
// the instance. Implemented by service.connectionResolverGeneric. Called from service.TransparentProxy.Handler for every request and on stream errors.
//
//go:generate moq -stub -out mock/connection_resolver.go -pkg mock . ConnectionResolver
type ConnectionResolver interface {
	// GetConnection returns a gRPC connection to the backend for the given route and processed headers; for static — single connection, for dynamic — from pool (round-robin or by sticky key).
	// Parameters: ctx — request context; route — result of RouteMatcher.Match (cluster, balancer type); headers — metadata after HeaderProcessor.Process (for sticky header when sticky_sessions).
	// Returns: (conn, stickyKey, instanceID, nil) on success (stickyKey empty for round-robin/static; instanceID — instance identifier or cluster name for static); (nil, "", "", error) on unknown cluster (ErrGenericUnknownCluster), missing sticky header (ErrStickyKeyRequired) or pool error (ErrNoAvailableConnInstance, etc.).
	// Called from service.TransparentProxy.Handler when opening the stream to the backend (and on retries).
	GetConnection(ctx context.Context, route domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error)

	// OnBackendFailure notifies the resolver of stream/dial failure to the backend so it can unbind sticky session and close/unregister the instance.
	// Parameters: route — request route; stickyKey — sticky key from the failed request (may be empty); instanceID — identifier of the instance that failed.
	// Called from service.TransparentProxy.Handler on NewStream error or stream message forward error.
	OnBackendFailure(route domain.Route, stickyKey, instanceID string)
}
