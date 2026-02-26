package service

import (
	"context"
	"errors"
	"fmt"

	"mygateway/domain"
	"mygateway/helpers"
	"mygateway/interfaces"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ErrGenericUnknownCluster is returned when route.Cluster is not found in staticConns or pools; proxy converts it to gRPC status.
var ErrGenericUnknownCluster = errors.New("unknown cluster")

// ErrStickyKeyRequired is returned when the route uses sticky_sessions but the configured header (e.g. session-id) is missing in metadata; proxy converts to gRPC status.
var ErrStickyKeyRequired = errors.New("sticky key required for this route")

// connectionResolverGeneric implements interfaces.ConnectionResolver. It resolves (route, headers) to a backend
// *grpc.ClientConn: for static clusters returns the pre-dialed connection; for dynamic clusters
// delegates to the corresponding ConnectionPool (GetConnRoundRobin or GetConnForKey using the balancer header).
// Also implements OnBackendFailure (delegate to pool) and Close (close all static conns and pools).
// Built in cmd/main from staticConns and dynamicPools maps.
type connectionResolverGeneric struct {
	staticConns map[domain.ClusterID]*grpc.ClientConn
	pools       map[domain.ClusterID]interfaces.ConnectionPool
}

// NewConnectionResolverGeneric creates a resolver from static connection and dynamic pool maps. Panics on nil staticConns or pools.
//
// Parameters: staticConns — cluster ID → single *grpc.ClientConn for static clusters; pools — cluster ID → ConnectionPool for dynamic. Empty maps allowed (static-only or dynamic-only).
//
// Returns: *connectionResolverGeneric implementing interfaces.ConnectionResolver.
//
// Called from cmd/main when building the gateway.
func NewConnectionResolverGeneric(
	staticConns map[domain.ClusterID]*grpc.ClientConn,
	pools map[domain.ClusterID]interfaces.ConnectionPool,
) *connectionResolverGeneric {
	return &connectionResolverGeneric{
		staticConns: helpers.NilPanic(staticConns, "service.connection_resolver_generic.go: staticConns is required"),
		pools:       helpers.NilPanic(pools, "service.connection_resolver_generic.go: pools is required"),
	}
}

// GetConnection returns a backend connection for the given route and headers: for static — pre-dialed conn from staticConns; for dynamic — from pool (round-robin or by sticky key from header).
//
// Parameters: ctx — request context; route — result of RouteMatcher.Match (Cluster, Balancer); headers — metadata after HeaderProcessor (for sticky header when sticky_sessions). Missing required header for sticky_sessions returns ErrStickyKeyRequired.
//
// Returns: (conn, stickyKey, instanceID, nil) on success (stickyKey empty for round-robin/static; instanceID — instance ID or cluster name for static); (nil, "", "", error) on unknown cluster (ErrGenericUnknownCluster), missing sticky header (ErrStickyKeyRequired) or pool error (ErrNoAvailableConnInstance, etc.).
//
// Called from service.TransparentProxy.Handler when opening the stream to the backend.
func (r *connectionResolverGeneric) GetConnection(
	ctx context.Context,
	route domain.Route,
	headers metadata.MD,
) (*grpc.ClientConn, string, string, error) {
	if conn := r.staticConns[route.Cluster]; conn != nil {
		return conn, "", string(route.Cluster), nil
	}
	p := r.pools[route.Cluster]
	if p == nil {
		return nil, "", "", fmt.Errorf("%w: %s", ErrGenericUnknownCluster, route.Cluster)
	}
	if route.Balancer.Type == domain.BalancerStickySession {
		header := route.Balancer.Header
		if header == "" {
			header = domain.StickySessionHeader
		}
		key, ok := helpers.GetHeaderValue(headers, header)
		if !ok {
			return nil, "", "", fmt.Errorf("%w: %s", ErrStickyKeyRequired, header)
		}
		conn, instanceID, err := p.GetConnectionForKey(ctx, key)
		if err != nil {
			return nil, "", "", err
		}
		return conn, key, instanceID, nil
	}
	conn, instanceID, err := p.GetConnectionRoundRobin(ctx)
	if err != nil {
		return nil, "", "", err
	}
	return conn, "", instanceID, nil
}

// OnBackendFailure delegates to the route's pool to unbind sticky key and close/unregister the instance. No-op for static cluster or when pool is missing.
//
// Parameters: route — route of the failed request; stickyKey — sticky key (from GetConnection); instanceID — identifier of the instance that failed.
//
// Called from service.TransparentProxy.Handler on backend stream creation error or message forward error.
func (r *connectionResolverGeneric) OnBackendFailure(route domain.Route, stickyKey, instanceID string) {
	p := r.pools[route.Cluster]
	if p == nil {
		return
	}
	p.OnBackendFailure(stickyKey, instanceID)
}

// Close closes all static connections and all pools. Errors from individual connections/pools are not aggregated; returns nil.
//
// Called from cmd/main via defer on graceful shutdown.
func (r *connectionResolverGeneric) Close() error {
	for _, conn := range r.staticConns {
		_ = conn.Close()
	}
	for _, p := range r.pools {
		_ = p.Close()
	}
	return nil
}
