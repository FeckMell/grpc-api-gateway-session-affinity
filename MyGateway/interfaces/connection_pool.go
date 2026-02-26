package interfaces

import (
	"context"

	"google.golang.org/grpc"
)

// ConnectionPool provides backend gRPC connections for one dynamic cluster, with either
// round-robin or sticky-by-key selection.
//
// GetConnRoundRobin returns a connection to the next instance in round-robin order;
// used when the route does not require sticky sessions.
// GetConnForKey returns a connection bound to the given key (e.g. session-id value);
// the same key always gets the same instance until OnBackendFailure or instance removal.
// OnBackendFailure unbinds the key from the instance, closes the connection to that
// instance, and notifies the discoverer (e.g. UnregisterInstance).
// Close closes all connections and stops the pool; idempotent.
//
// Called by service.connectionResolverGeneric (GetConnection delegates to GetConnectionRoundRobin or
// GetConnectionForKey; OnBackendFailure and Close are called by the resolver on behalf of the proxy).
//
//go:generate moq -stub -out mock/connection_pool.go -pkg mock . ConnectionPool
type ConnectionPool interface {
	// GetConnectionRoundRobin returns a connection to the next instance in round-robin order; used for routes without sticky sessions.
	// Parameter ctx — context for dial when creating a new connection; cancel/timeout lead to factory error.
	// Returns: (conn, instanceID, nil) on success; (nil, "", err) when pool is closed (ErrConnPoolClosed), no instances or dial error (ErrNoAvailableConnInstance, etc.).
	// Called from service.connectionResolverGeneric.GetConnection when route.Balancer.Type != sticky_sessions.
	GetConnectionRoundRobin(ctx context.Context) (conn *grpc.ClientConn, instanceID string, err error)

	// GetConnectionForKey returns a connection bound to the sticky key (e.g. session-id); the same key gets the same instance until OnBackendFailure or instance removal.
	// Parameters: ctx — for dial when creating connection; key — sticky header value (e.g. session-id). Empty key usually yields ErrNoAvailableConnInstance.
	// Returns: (conn, instanceID, nil) on success; (nil, "", err) when pool closed, empty key, no free instance or dial error.
	// Called from service.connectionResolverGeneric.GetConnection when route.Balancer.Type == sticky_sessions.
	GetConnectionForKey(ctx context.Context, key string) (conn *grpc.ClientConn, instanceID string, err error)

	// OnBackendFailure unbinds the key from the instance (if key non-empty), closes the connection to instanceID, removes the instance from the list and calls discoverer.UnregisterInstance(instanceID).
	// Parameters: key — sticky key of the failed request (empty string allowed, then only close/unregister); instanceID — identifier of the instance that failed.
	// Called from service.connectionResolverGeneric.OnBackendFailure on stream or dial failure to the backend.
	OnBackendFailure(key string, instanceID string)

	// Close closes all pool connections and marks the pool closed; idempotent. Subsequent GetConnection* return ErrConnPoolClosed.
	// Returns: nil (errors from closing individual connections are not aggregated).
	// Called from service.connectionResolverGeneric.Close on shutdown (cmd/main defer).
	Close() error
}
