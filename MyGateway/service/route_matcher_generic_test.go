package service

import (
	"testing"

	"mygateway/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouteMatcherGeneric(t *testing.T) {
	t.Run("invalid_config_returns_error", func(t *testing.T) {
		_, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{Prefix: "", Cluster: "c1"},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prefix must be non-empty")
	})

	t.Run("valid_empty_routes_returns_router", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes:  []domain.Route{},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		require.NotNil(t, r)
		_, ok := r.Match("/any/method")
		assert.False(t, ok)
	})

	t.Run("valid_nil_routes_returns_router", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes:  nil,
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		require.NotNil(t, r)
		_, ok := r.Match("/any/method")
		assert.False(t, ok)
	})

	t.Run("routes_sorted_by_prefix_length_desc", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{Prefix: "/a", Cluster: "short"},
				{Prefix: "/a/b/c", Cluster: "long"},
				{Prefix: "/a/b", Cluster: "medium"},
			},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		// Match /a/b/c/foo should hit longest prefix /a/b/c
		route, ok := r.Match("/a/b/c/foo")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("long"), route.Cluster)
		// Match /a/b/bar should hit /a/b
		route, ok = r.Match("/a/b/bar")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("medium"), route.Cluster)
		// Match /a/qux should hit /a
		route, ok = r.Match("/a/qux")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("short"), route.Cluster)
	})
}

func TestRouteMatcherGeneric_Match(t *testing.T) {
	t.Run("no_match_default_error_returns_false", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes:  []domain.Route{{Prefix: "/api", Cluster: "c1"}},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		route, ok := r.Match("/other/method")
		assert.False(t, ok)
		assert.Equal(t, domain.Route{}, route)
	})

	t.Run("no_match_default_use_cluster_returns_default_cluster", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes:  []domain.Route{{Prefix: "/api", Cluster: "c1"}},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteUseCluster, Cluster: "default_cluster"},
		})
		require.NoError(t, err)
		route, ok := r.Match("/nomatch/method")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("default_cluster"), route.Cluster)
		assert.Equal(t, domain.AuthorizationNone, route.Authorization)
		assert.Equal(t, domain.BalancerRoundRobin, route.Balancer.Type)
	})

	t.Run("prefix_match_returns_route_with_defaults", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{Prefix: "/svc", Cluster: "cluster1"},
			},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		route, ok := r.Match("/svc/Method")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("cluster1"), route.Cluster)
		assert.Equal(t, domain.AuthorizationNone, route.Authorization)
		assert.Equal(t, domain.BalancerRoundRobin, route.Balancer.Type)
	})

	t.Run("longest_prefix_wins", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{Prefix: "/pkg.Service/A", Cluster: "short"},
				{Prefix: "/pkg.Service/AB", Cluster: "long"},
			},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		route, ok := r.Match("/pkg.Service/ABMethod")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("long"), route.Cluster)
	})

	t.Run("authorization_required_preserved", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{Prefix: "/auth", Cluster: "c1", Authorization: domain.AuthorizationRequired},
			},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		route, ok := r.Match("/auth/Login")
		require.True(t, ok)
		assert.Equal(t, domain.AuthorizationRequired, route.Authorization)
	})

	t.Run("sticky_sessions_with_header_preserved", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{
					Prefix:  "/sticky",
					Cluster: "c1",
					Balancer: domain.BalancerConfig{
						Type:   domain.BalancerStickySession,
						Header: "session-id",
					},
				},
			},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		route, ok := r.Match("/sticky/Call")
		require.True(t, ok)
		assert.Equal(t, domain.BalancerStickySession, route.Balancer.Type)
		assert.Equal(t, "session-id", route.Balancer.Header)
	})

	// Sticky with empty header: config validation rejects it, but withRouteDefaults fills
	// default header when that branch is reached (e.g. if router were built without validation).
	t.Run("sticky_sessions_empty_header_gets_default", func(t *testing.T) {
		r := &routeMatcherGeneric{
			routes: []domain.Route{{
				Prefix:  "/sticky",
				Cluster: "c1",
				Balancer: domain.BalancerConfig{
					Type:   domain.BalancerStickySession,
					Header: "",
				},
			}},
			def: domain.DefaultRoute{Action: domain.DefaultRouteError},
		}
		route, ok := r.Match("/sticky/Call")
		require.True(t, ok)
		assert.Equal(t, domain.StickySessionHeader, route.Balancer.Header)
	})

	t.Run("exact_prefix_match", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes: []domain.Route{
				{Prefix: "/exact", Cluster: "c1"},
			},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		route, ok := r.Match("/exact")
		require.True(t, ok)
		assert.Equal(t, domain.ClusterID("c1"), route.Cluster)
	})

	t.Run("no_match_empty_routes_default_error", func(t *testing.T) {
		r, err := NewRouteMatcherGeneric(domain.RouteConfig{
			Routes:  []domain.Route{},
			Default: domain.DefaultRoute{Action: domain.DefaultRouteError},
		})
		require.NoError(t, err)
		_, ok := r.Match("/anything")
		assert.False(t, ok)
	})
}
