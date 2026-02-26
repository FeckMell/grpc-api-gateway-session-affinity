package domain

import (
	"strconv"
	"strings"
)

// ClusterID identifies a backend cluster (e.g. "myauth", "my_service").
type ClusterID string

// AuthorizationMode is the per-route auth policy: none (pass through) or required (JWT + session-id).
type AuthorizationMode string

const (
	AuthorizationNone     AuthorizationMode = "none"
	AuthorizationRequired AuthorizationMode = "required"
)

// BalancerType selects how a backend instance is chosen: round_robin or sticky_sessions.
type BalancerType string

const (
	BalancerRoundRobin    BalancerType = "round_robin"
	BalancerStickySession BalancerType = "sticky_sessions"
)

// BalancerConfig holds balancer type and, for sticky_sessions, the metadata header name (e.g. session-id).
type BalancerConfig struct {
	Type   BalancerType
	Header string
}

// Route maps a path prefix to a cluster.
// Prefix must start with "/" and is matched with strings.HasPrefix(fullMethod, Prefix).
type Route struct {
	Prefix        string
	Cluster       ClusterID
	Authorization AuthorizationMode
	Balancer      BalancerConfig
}

// DefaultRouteAction is the behavior when no route prefix matches: error (return Unimplemented) or use_cluster.
type DefaultRouteAction string

const (
	DefaultRouteError      DefaultRouteAction = "error"
	DefaultRouteUseCluster DefaultRouteAction = "use_cluster"
)

// DefaultRoute defines the action and optional default cluster when no route matches.
type DefaultRoute struct {
	Action  DefaultRouteAction
	Cluster ClusterID
}

// RouteConfig is an ordered list of routes and the default route.
// Longer prefixes should be ordered first so that longest-prefix match works.
type RouteConfig struct {
	Routes  []Route
	Default DefaultRoute
}

// ValidateRouteConfig validates route and default config: each route has non-empty Prefix starting with "/", authorization none|required, balancer.type round_robin|sticky_sessions; for sticky_sessions balancer.header is set; default.action error|use_cluster; for use_cluster default.cluster is non-empty.
//
// Parameter cfg — route config (usually from YAML via cmd.LoadConfig). Routes may be in any order; validation does not check cluster references (LoadConfig does that).
//
// Returns: nil when config is valid; *RouteConfigError with Index (0-based route index or -1 for default section) and Reason (error text) on first error found.
//
// Called from service.NewRouteMatcherGeneric and cmd.LoadConfig before using the config.
func ValidateRouteConfig(cfg RouteConfig) error {
	for i, r := range cfg.Routes {
		if r.Prefix == "" {
			return &RouteConfigError{Index: i, Reason: "prefix must be non-empty"}
		}
		if len(r.Prefix) > 0 && r.Prefix[0] != '/' {
			return &RouteConfigError{Index: i, Reason: "prefix must start with /"}
		}
		switch r.Authorization {
		case "", AuthorizationNone, AuthorizationRequired:
		default:
			return &RouteConfigError{Index: i, Reason: "authorization must be none|required"}
		}
		switch r.Balancer.Type {
		case "", BalancerRoundRobin, BalancerStickySession:
		default:
			return &RouteConfigError{Index: i, Reason: "balancer.type must be round_robin|sticky_sessions"}
		}
		if r.Balancer.Type == BalancerStickySession && strings.TrimSpace(r.Balancer.Header) == "" {
			return &RouteConfigError{Index: i, Reason: "balancer.header is required for sticky_sessions"}
		}
	}
	switch cfg.Default.Action {
	case "", DefaultRouteError:
	case DefaultRouteUseCluster:
		if cfg.Default.Cluster == "" {
			return &RouteConfigError{Index: -1, Reason: "default.cluster is required when default.action=use_cluster"}
		}
	default:
		return &RouteConfigError{Index: -1, Reason: "default.action must be error|use_cluster"}
	}
	return nil
}

// RouteConfigError is returned by ValidateRouteConfig when a route or the default is invalid.
// Index is the route index (0-based) or -1 for the default section; Reason is a human-readable message.
type RouteConfigError struct {
	Index  int
	Reason string
}

// Error implements error; returns a string like "route[0]: prefix must be non-empty" for logging and user output.
// For e: Index — route index (0-based) or -1 for default; Reason — validation message.
// Returns: single string "route[N]: " + Reason.
// Used when logging or os.Exit(1) in main on invalid config (after ValidateRouteConfig).
func (e *RouteConfigError) Error() string {
	return "route[" + strconv.Itoa(e.Index) + "]: " + e.Reason
}
