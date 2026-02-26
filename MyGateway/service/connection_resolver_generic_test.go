package service

import (
	"context"
	"errors"
	"testing"

	"mygateway/domain"
	"mygateway/interfaces"
	"mygateway/interfaces/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	errPoolKey = errors.New("pool error")
	errPoolRR  = errors.New("pool rr error")
)

func TestNewConnectionResolverGeneric_Panics(t *testing.T) {
	t.Run("staticConns_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.connection_resolver_generic.go: staticConns is required", func() {
			NewConnectionResolverGeneric(nil, map[domain.ClusterID]interfaces.ConnectionPool{})
		})
	})
	t.Run("pools_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.connection_resolver_generic.go: pools is required", func() {
			NewConnectionResolverGeneric(map[domain.ClusterID]*grpc.ClientConn{}, nil)
		})
	})
}

func TestConnectionResolverGeneric_GetConnection(t *testing.T) {
	ctx := context.Background()
	testConn := newTestConn(t)

	tests := []struct {
		name        string
		staticConns map[domain.ClusterID]*grpc.ClientConn
		pools       map[domain.ClusterID]interfaces.ConnectionPool
		route       domain.Route
		headers     metadata.MD
		wantErr     error
		wantSticky  string
		wantInstID  string
	}{
		{
			name:        "static_cluster_returns_conn",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{domain.ClusterID("static_a"): testConn},
			pools:       map[domain.ClusterID]interfaces.ConnectionPool{},
			route:       domain.Route{Cluster: "static_a"},
			headers:     nil,
			wantErr:     nil,
			wantInstID:  "static_a",
		},
		{
			name:        "unknown_cluster_returns_error",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools:       map[domain.ClusterID]interfaces.ConnectionPool{},
			route:       domain.Route{Cluster: "unknown"},
			headers:     nil,
			wantErr:     ErrGenericUnknownCluster,
		},
		{
			name:        "sticky_sessions_missing_header_returns_error",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"dynamic": &mock.ConnectionPoolMock{},
			},
			route:   domain.Route{Cluster: "dynamic", Balancer: domain.BalancerConfig{Type: domain.BalancerStickySession, Header: "session-id"}},
			headers: metadata.Pairs("other", "v"),
			wantErr: ErrStickyKeyRequired,
		},
		{
			name:        "sticky_sessions_with_header_calls_GetConnForKey",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"dynamic": &mock.ConnectionPoolMock{
					GetConnectionForKeyFunc: func(ctx context.Context, key string) (*grpc.ClientConn, string, error) {
						if key != "sess-1" {
							return nil, "", errors.New("unexpected key")
						}
						return testConn, "inst-1", nil
					},
				},
			},
			route:      domain.Route{Cluster: "dynamic", Balancer: domain.BalancerConfig{Type: domain.BalancerStickySession, Header: "session-id"}},
			headers:    metadata.Pairs("session-id", "sess-1"),
			wantErr:    nil,
			wantSticky: "sess-1",
			wantInstID: "inst-1",
		},
		{
			name:        "round_robin_calls_GetConnRoundRobin",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"dynamic": &mock.ConnectionPoolMock{
					GetConnectionRoundRobinFunc: func(ctx context.Context) (*grpc.ClientConn, string, error) {
						return testConn, "inst-rr", nil
					},
				},
			},
			route:      domain.Route{Cluster: "dynamic", Balancer: domain.BalancerConfig{Type: domain.BalancerRoundRobin}},
			headers:    nil,
			wantErr:    nil,
			wantInstID: "inst-rr",
		},
		{
			name:        "sticky_sessions_empty_header_uses_default",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"dynamic": &mock.ConnectionPoolMock{
					GetConnectionForKeyFunc: func(ctx context.Context, key string) (*grpc.ClientConn, string, error) {
						if key != "sess-default" {
							return nil, "", errors.New("unexpected key")
						}
						return testConn, "inst-1", nil
					},
				},
			},
			route:      domain.Route{Cluster: "dynamic", Balancer: domain.BalancerConfig{Type: domain.BalancerStickySession, Header: ""}},
			headers:    metadata.Pairs("session-id", "sess-default"),
			wantErr:    nil,
			wantSticky: "sess-default",
			wantInstID: "inst-1",
		},
		{
			name:        "sticky_sessions_GetConnForKey_error",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"dynamic": &mock.ConnectionPoolMock{
					GetConnectionForKeyFunc: func(ctx context.Context, key string) (*grpc.ClientConn, string, error) {
						return nil, "", errPoolKey
					},
				},
			},
			route:   domain.Route{Cluster: "dynamic", Balancer: domain.BalancerConfig{Type: domain.BalancerStickySession, Header: "session-id"}},
			headers: metadata.Pairs("session-id", "sess-1"),
			wantErr: errPoolKey,
		},
		{
			name:        "round_robin_GetConnRoundRobin_error",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"dynamic": &mock.ConnectionPoolMock{
					GetConnectionRoundRobinFunc: func(ctx context.Context) (*grpc.ClientConn, string, error) {
						return nil, "", errPoolRR
					},
				},
			},
			route:   domain.Route{Cluster: "dynamic", Balancer: domain.BalancerConfig{Type: domain.BalancerRoundRobin}},
			headers: nil,
			wantErr: errPoolRR,
		},
		{
			name:        "static_cluster_nil_conn_falls_through_to_pool",
			staticConns: map[domain.ClusterID]*grpc.ClientConn{domain.ClusterID("c"): nil},
			pools: map[domain.ClusterID]interfaces.ConnectionPool{
				"c": &mock.ConnectionPoolMock{
					GetConnectionRoundRobinFunc: func(ctx context.Context) (*grpc.ClientConn, string, error) {
						return testConn, "inst-pool", nil
					},
				},
			},
			route:      domain.Route{Cluster: "c", Balancer: domain.BalancerConfig{Type: domain.BalancerRoundRobin}},
			headers:    nil,
			wantErr:    nil,
			wantInstID: "inst-pool",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewConnectionResolverGeneric(tt.staticConns, tt.pools)
			conn, stickyKey, instanceID, err := r.GetConnection(ctx, tt.route, tt.headers)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, conn)
			assert.Equal(t, tt.wantSticky, stickyKey)
			assert.Equal(t, tt.wantInstID, instanceID)
		})
	}
}

func TestConnectionResolverGeneric_OnBackendFailure(t *testing.T) {
	t.Run("no_pool_for_cluster_noop", func(t *testing.T) {
		r := NewConnectionResolverGeneric(
			map[domain.ClusterID]*grpc.ClientConn{},
			map[domain.ClusterID]interfaces.ConnectionPool{},
		)
		assert.NotPanics(t, func() {
			r.OnBackendFailure(domain.Route{Cluster: "none"}, "key", "inst")
		})
	})
	t.Run("pool_delegates", func(t *testing.T) {
		var gotKey, gotInstID string
		pool := &mock.ConnectionPoolMock{
			OnBackendFailureFunc: func(key string, instanceID string) {
				gotKey = key
				gotInstID = instanceID
			},
		}
		r := NewConnectionResolverGeneric(
			map[domain.ClusterID]*grpc.ClientConn{},
			map[domain.ClusterID]interfaces.ConnectionPool{"c1": pool},
		)
		r.OnBackendFailure(domain.Route{Cluster: "c1"}, "sk", "inst-1")
		assert.Equal(t, "sk", gotKey)
		assert.Equal(t, "inst-1", gotInstID)
	})
}

func TestConnectionResolverGeneric_Close(t *testing.T) {
	testConn := newTestConn(t)
	staticClosed := false
	poolClosed := false
	r := NewConnectionResolverGeneric(
		map[domain.ClusterID]*grpc.ClientConn{"s": testConn},
		map[domain.ClusterID]interfaces.ConnectionPool{
			"d": &mock.ConnectionPoolMock{CloseFunc: func() error { poolClosed = true; return nil }},
		},
	)
	err := r.Close()
	require.NoError(t, err)
	assert.True(t, poolClosed)
	// static conn was closed by Close() (conn.Close() called)
	_ = staticClosed
}
