package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRouteConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         RouteConfig
		wantErr     bool
		wantIndex   int
		wantContain string
	}{
		{
			name:    "valid_nil_routes",
			cfg:     RouteConfig{Routes: nil},
			wantErr: false,
		},
		{
			name:    "valid_empty_routes",
			cfg:     RouteConfig{Routes: []Route{}},
			wantErr: false,
		},
		{
			name: "valid_simple_routes",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/api/Login", Cluster: ClusterID("myauth")},
					{Prefix: "/api/MyService", Cluster: ClusterID("my_service")},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_authorization_none",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Authorization: AuthorizationNone},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_authorization_required",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Authorization: AuthorizationRequired},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_authorization_empty",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Authorization: ""},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_balancer_round_robin",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Balancer: BalancerConfig{Type: BalancerRoundRobin}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_balancer_empty_type",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Balancer: BalancerConfig{Type: ""}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_balancer_sticky_with_header",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Balancer: BalancerConfig{Type: BalancerStickySession, Header: "session-id"}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_default_action_empty",
			cfg: RouteConfig{
				Routes:  []Route{{Prefix: "/x", Cluster: "c1"}},
				Default: DefaultRoute{Action: ""},
			},
			wantErr: false,
		},
		{
			name: "valid_default_action_error",
			cfg: RouteConfig{
				Routes:  []Route{{Prefix: "/x", Cluster: "c1"}},
				Default: DefaultRoute{Action: DefaultRouteError},
			},
			wantErr: false,
		},
		{
			name: "valid_default_use_cluster_with_cluster",
			cfg: RouteConfig{
				Routes:  []Route{{Prefix: "/x", Cluster: "c1"}},
				Default: DefaultRoute{Action: DefaultRouteUseCluster, Cluster: "fallback"},
			},
			wantErr: false,
		},
		{
			name: "err_empty_prefix",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/ok", Cluster: "c1"},
					{Prefix: "", Cluster: "c2"},
				},
			},
			wantErr:     true,
			wantIndex:   1,
			wantContain: "non-empty",
		},
		{
			name: "err_prefix_no_leading_slash",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "no-slash", Cluster: "c1"},
				},
			},
			wantErr:     true,
			wantIndex:   0,
			wantContain: "start with /",
		},
		{
			name: "err_invalid_authorization",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Authorization: AuthorizationMode("invalid")},
				},
			},
			wantErr:     true,
			wantIndex:   0,
			wantContain: "authorization must be none|required",
		},
		{
			name: "err_invalid_balancer_type",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Balancer: BalancerConfig{Type: BalancerType("random")}},
				},
			},
			wantErr:     true,
			wantIndex:   0,
			wantContain: "balancer.type must be round_robin|sticky_sessions",
		},
		{
			name: "err_sticky_sessions_empty_header",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Balancer: BalancerConfig{Type: BalancerStickySession, Header: ""}},
				},
			},
			wantErr:     true,
			wantIndex:   0,
			wantContain: "balancer.header is required for sticky_sessions",
		},
		{
			name: "err_sticky_sessions_whitespace_header",
			cfg: RouteConfig{
				Routes: []Route{
					{Prefix: "/x", Cluster: "c1", Balancer: BalancerConfig{Type: BalancerStickySession, Header: "  \t "}},
				},
			},
			wantErr:     true,
			wantIndex:   0,
			wantContain: "balancer.header is required for sticky_sessions",
		},
		{
			name: "err_default_use_cluster_empty_cluster",
			cfg: RouteConfig{
				Routes:  []Route{{Prefix: "/x", Cluster: "c1"}},
				Default: DefaultRoute{Action: DefaultRouteUseCluster, Cluster: ""},
			},
			wantErr:     true,
			wantIndex:   -1,
			wantContain: "default.cluster is required when default.action=use_cluster",
		},
		{
			name: "err_default_invalid_action",
			cfg: RouteConfig{
				Routes:  []Route{{Prefix: "/x", Cluster: "c1"}},
				Default: DefaultRoute{Action: DefaultRouteAction("invalid")},
			},
			wantErr:     true,
			wantIndex:   -1,
			wantContain: "default.action must be error|use_cluster",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRouteConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				var routeErr *RouteConfigError
				require.ErrorAs(t, err, &routeErr)
				assert.Equal(t, tt.wantIndex, routeErr.Index, "Index")
				assert.Contains(t, routeErr.Reason, tt.wantContain, "Reason")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRouteConfigError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *RouteConfigError
		wantMsg string
	}{
		{
			name:    "index_zero",
			err:     &RouteConfigError{Index: 0, Reason: "prefix must be non-empty"},
			wantMsg: "route[0]: prefix must be non-empty",
		},
		{
			name:    "index_negative",
			err:     &RouteConfigError{Index: -1, Reason: "default.action must be error|use_cluster"},
			wantMsg: "route[-1]: default.action must be error|use_cluster",
		},
		{
			name:    "index_positive",
			err:     &RouteConfigError{Index: 5, Reason: "custom reason"},
			wantMsg: "route[5]: custom reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}
