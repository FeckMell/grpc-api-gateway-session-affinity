package service

import (
	"sort"
	"strings"

	"mygateway/domain"
	"mygateway/helpers"
)

// routeMatcherGeneric implements interfaces.RouteMatcher. It maps a gRPC full method name to a domain.Route
// using longest-prefix match: routes are stored sorted by prefix length (descending) and the first
// matching prefix wins. Holds a copy of routes and the default route; provides Match(method) used by the proxy. Built from domain.RouteConfig in cmd/main.
type routeMatcherGeneric struct {
	routes []domain.Route
	def    domain.DefaultRoute
}

// NewRouteMatcherGeneric validates config via ValidateRouteConfig, copies routes, sorts by descending prefix length (longest-prefix) and creates the router. After validation panics on nil routes/default (helpers.NilPanic).
//
// Parameter cfg — route config (from YAML via LoadConfig). Must contain Routes and Default.
//
// Returns: (*routeMatcherGeneric, nil) on success; (nil, error) on ValidateRouteConfig error (*RouteConfigError).
//
// Called from cmd/main at startup.
func NewRouteMatcherGeneric(cfg domain.RouteConfig) (*routeMatcherGeneric, error) {
	if err := domain.ValidateRouteConfig(cfg); err != nil {
		return nil, err
	}
	routes := make([]domain.Route, len(cfg.Routes))
	copy(routes, cfg.Routes)
	sort.Slice(routes, func(i, j int) bool {
		return len(routes[i].Prefix) > len(routes[j].Prefix)
	})

	return &routeMatcherGeneric{
		routes: helpers.NilPanic(routes, "service.route_matcher_generic.go: routes is required"),
		def:    helpers.NilPanic(cfg.Default, "service.route_matcher_generic.go: default is required"),
	}, nil
}

// Match returns the route by longest-prefix for the full gRPC method name; withRouteDefaults (authorization, balancer, header) is applied to the route. When no prefix matches, default (use_cluster or error) is used.
//
// Parameter method — full method name, e.g. /package.Service/Method. Empty string matches no prefix; result depends on default.
//
// Returns: (domain.Route with fields filled, true) on prefix match or default action use_cluster (then only Cluster is set in Route); (domain.Route{}, false) when no match and default.action=error.
//
// Called from service.TransparentProxy.Handler at the start of each RPC.
func (r *routeMatcherGeneric) Match(method string) (domain.Route, bool) {
	for _, route := range r.routes {
		if strings.HasPrefix(method, route.Prefix) {
			return withRouteDefaults(route), true
		}
	}
	if r.def.Action == domain.DefaultRouteUseCluster {
		return withRouteDefaults(domain.Route{Cluster: r.def.Cluster}), true
	}
	return domain.Route{}, false
}

// withRouteDefaults fills default values for empty route fields: authorization=none, balancer.type=round_robin, for sticky_sessions — header=session-id.
//
// Parameter route — route from config or default (may have empty fields).
//
// Returns: copy of route with empty fields filled (does not mutate the original slice/struct if passed by value).
//
// Called only from routeMatcherGeneric.Match.
func withRouteDefaults(route domain.Route) domain.Route {
	if route.Authorization == "" {
		route.Authorization = domain.AuthorizationNone
	}
	if route.Balancer.Type == "" {
		route.Balancer.Type = domain.BalancerRoundRobin
	}
	if route.Balancer.Type == domain.BalancerStickySession && route.Balancer.Header == "" {
		route.Balancer.Header = domain.StickySessionHeader
	}
	return route
}
