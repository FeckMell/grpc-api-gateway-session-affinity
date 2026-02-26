package interfaces

import "mygateway/domain"

// RouteMatcher resolves a gRPC full method name to a routing decision. Used by the transparent proxy
// to obtain the route (cluster, authorization, balancer) before header processing and connection
// resolution. Implemented by service.routeMatcherGeneric. Called from service.TransparentProxy.Handler.
//
//go:generate moq -stub -out mock/route_matcher.go -pkg mock . RouteMatcher
type RouteMatcher interface {
	// Match returns the route (cluster, authorization, balancer) by longest-prefix for the full gRPC method name; when no prefix matches, default (error or use_cluster) is used.
	// Parameter method — full method name, e.g. /package.Service/Method; empty string matches no prefix and result depends on default.
	// Returns: (domain.Route with fields filled, true) on prefix match or default use_cluster; (domain.Route{}, false) when no match and default.action=error.
	// Called from service.TransparentProxy.Handler at the start of each RPC.
	Match(method string) (domain.Route, bool)
}
