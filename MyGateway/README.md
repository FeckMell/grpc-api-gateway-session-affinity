# MyGateway — code and product documentation

This document describes MyGateway as a product and as a codebase: functionality, scenarios (success and error), architecture, and important technical decisions. Aimed at system analysts and developers (including leads).

---

## 1. Purpose and system boundaries

**MyGateway** is an API-agnostic gRPC proxy and load balancer. The service is not tied to any specific `.proto` or service contract: it routes and proxies **any** gRPC traffic by full method name and metadata.

**Idea:** The proxy operates only at the transport level (route, headers, backend selection). Protobuf payload is forwarded transparently between client and backend without parsing in application code.

**Role in the ecosystem:** Single entry point for clients; behind MyGateway there can be different backend clusters (auth, training, etc.).

---

## 2. Functional capabilities

### 2.1 gRPC proxying

- Handles **any** gRPC calls (unary and streaming) via `grpc.UnknownServiceHandler`.
- Full method name is taken from stream context (`grpc.MethodFromServerStream`).
- Payload is forwarded via `emptypb.Empty` (recv/send without deserialization into app types), keeping the proxy transparent for any gRPC API.

### 2.2 Routing

- **Input:** Full method name (e.g. `/package.Service/Method`).
- **Algorithm:** Longest-prefix match over configured routes (`strings.HasPrefix(method, route.Prefix)`); routes are sorted by decreasing prefix length at init.
- **Output:** `domain.Route` (cluster, authorization, balancer).
- **Default route:**
  - `error` — when no route matches, no route is returned (proxy then returns Unimplemented).
  - `use_cluster` — when no route matches, a route with the specified default cluster is returned.

### 2.3 Authorization (per-route)

- **none** — Headers pass through without checks.
- **required** — Required:
  - metadata `session-id`;
  - metadata `authorization` (value is the JWT itself, no "Bearer" prefix);
  - Valid JWT: HMAC-SHA256 signature, expiry, and `session_id` in claims must match `session-id` in header.

Policy is chosen by longest-prefix of the method against rules built from routes.

### 2.4 Balancing (per-route)

- **round_robin** — Select instance in round-robin order (for dynamic cluster).
- **sticky_sessions** — Bind value of a configurable header (e.g. `session-id`) to instance ID; if header is missing the request fails with `ErrStickyKeyRequired`.

### 2.5 Backend clusters

- **static** — Single fixed address, one persistent `*grpc.ClientConn` per cluster.
- **dynamic** — Instance list from HTTP Discoverer; connection pool (ConnectionPool), periodic refresh, round_robin or sticky by key.

### 2.6 Backend failure handling

- On error creating a stream to the backend or on proxy error (s2c/c2s), `OnBackendFailure(route, stickyKey, instanceID)` is called.
- Pool: unbind sticky key, close connection to that instance, call `Discoverer.UnregisterInstance(instanceID)`.
- The next request gets a new connection (round_robin or new sticky).
- **Retry:** For dynamic clusters on NewStream error — up to RETRY_COUNT attempts with RETRY_TIMEOUT_MS per attempt; on each failure OnBackendFailure, next attempt on another instance.

---

## 3. Success scenarios (happy paths)

### 3.1 Method without authorization (e.g. Login)

1. Client calls a method whose route has `authorization: none`.
2. Router: `Match(method)` → Route (cluster, balancer, authorization=none).
3. HeaderProcessorChain: ConfigurableAuthProcessor for this prefix skips (returns headers unchanged).
4. Resolver: For static cluster returns the single conn; for dynamic — GetConnectionRoundRobin or GetConnectionForKey (if sticky).
5. Proxy creates client stream to backend and transparently forwards traffic server↔client.
6. Client receives response/stream from backend.

### 3.2 Method with authorization and sticky sessions

1. Client sends metadata: `session-id`, `authorization: <JWT>`.
2. Router: Match → Route with `authorization: required`, `balancer: sticky_sessions`, `balancer.header: session-id`.
3. ConfigurableAuthProcessor: Checks session-id and authorization, validates JWT (signature, expiry, session_id).
4. Resolver: GetConnectionForKey(session-id) — returns cached or new connection to instance.
5. Transparent proxying until end of stream.
6. Client receives response/stream.

### 3.3 Default route use_cluster

1. Client calls a method that does not match any prefix.
2. Router: Match finds no route by prefix; default.action = use_cluster → returns Route with default.cluster.
3. Then as in 3.1/3.2 depending on that cluster and default route settings (if they were defined for default — in the current model default only specifies cluster).

### 3.4 Instance list refresh (dynamic cluster)

1. Background ticker at `discoverer_interval_ms` calls `Discoverer.GetInstances()`.
2. Pool updates instance list; connections to instances that disappeared are closed, sticky bindings for them removed.
3. New requests get connections only to current instances.

---

## 4. Error scenarios and error codes

### 4.1 Proxy (service/transparent.go)

| Condition | Behavior | gRPC code / message |
|-----------|----------|----------------------|
| No method in stream context | `grpc.MethodFromServerStream` → false | `Internal`: "missing grpc method in stream context" |
| No route match (and default not use_cluster) | Router.Match → false | `Unimplemented`: "method not routed" |
| Header chain error (auth etc.) | HeaderProcessor.Process returns err | err returned as-is (often `Unauthenticated`, `Internal`) |
| Resolver error (no cluster, no sticky key etc.) | GetConnection returns err | Handler returns err; stream interceptor maps to status (see 4.4) |
| Error creating client stream to backend | NewStream error | Handler returns err; interceptor → UNAVAILABLE "backend service unavailable"; OnBackendFailure |
| Error proxying server→client | forwardServerToClient | Handler returns err; interceptor → UNAVAILABLE; OnBackendFailure |
| Error proxying client→server | forwardClientToServer | Handler returns err; interceptor → UNAVAILABLE; OnBackendFailure |
| Both channels ended without explicit error | Should not happen on correct EOF | `Internal`: "unexpected proxy termination" |

### 4.2 Resolver (service/connection_resolver_generic.go)

| Condition | Error |
|-----------|--------|
| Cluster not static and not found in pools | `ErrGenericUnknownCluster` (wrapped with cluster name) → status.Convert gives client the corresponding gRPC status |
| sticky_sessions but no value in metadata for balancer.header | `ErrStickyKeyRequired` (wrapped with header name) |
| Pool.GetConnectionRoundRobin / GetConnectionForKey return error | Propagated unwrapped (e.g. `ErrNoAvailableConnInstance`, `ErrConnPoolClosed`) |

### 4.3 Pool (service/connection_pool.go)

| Condition | Error |
|-----------|--------|
| Pool already closed | `ErrConnPoolClosed` |
| GetConnectionRoundRobin: instance list empty or all factory dials failed | `ErrNoAvailableConnInstance` |
| GetConnectionForKey: empty key | `ErrNoAvailableConnInstance` |
| GetConnectionForKey: no free instance (all occupied by other session-ids) | `ErrNoAvailableConnInstance` |

### 4.4 Error mapping to gRPC status

Mapping is in **service/grpc_error.go**: function `GatewayErrorToGRPC` and stream server interceptor `GatewayErrorToGRPCStreamInterceptor`. Handler returns "raw" errors (from GetConnection, NewStream, forward s2c/c2s); interceptor after handler maps them to gRPC status per the table below.

| Condition in code | gRPC code | Message |
|-------------------|-----------|---------|
| `ErrNoAvailableConnInstance` | `RESOURCE_EXHAUSTED` (8) | "all instances are busy" |
| `ErrStickyKeyRequired` | `UNAUTHENTICATED` (16) | "missing or invalid token" |
| `ErrConnPoolClosed`, `ErrGenericUnknownCluster` and others (NewStream, s2c/c2s) | `UNAVAILABLE` (14) | "backend service unavailable" |

Status errors already produced by the handler (Internal, Unimplemented, Unauthenticated from auth) are left unchanged (interceptor returns them as-is).

### 4.5 ConfigurableAuthProcessor (helpers)

| Condition | gRPC code / message |
|-----------|----------------------|
| authorization: required, no session-id in metadata | `Unauthenticated`: "missing session-id" |
| authorization: required, no/empty authorization in metadata | `Unauthenticated`: "missing or invalid token" |
| JwtService.ValidateToken returns error (internal) | `Internal`: err.Error() |
| ValidateToken returns (false, nil) | `Unauthenticated`: "missing or invalid token" |

### 4.6 Configuration (cmd/config.go)

- Missing or invalid `SERVICE_PORT_GRPC` → exit 1 with message.
- Missing `CONFIG_PATH` → exit 1.
- YAML read/parse error → "load config ... : ...".
- Invalid default.action → "default.action must be error|use_cluster".
- For static cluster missing address → "cluster %s: address is required for static cluster".
- For dynamic: missing discoverer_url or discoverer_interval_ms ≤ 0 → corresponding messages.
- Route references unknown cluster → "route prefix ... references unknown cluster ...".
- default use_cluster points to undefined cluster → "default cluster ... is not defined".
- At least one route has authorization=required but JWT_SECRET empty → "JWT_SECRET is required when at least one route has authorization=required".
- Missing RETRY_COUNT or RETRY_TIMEOUT_MS → corresponding "... is required" messages.

### 4.7 Router and domain

- `NewRouteMatcherGeneric`: After validation, routes or default nil → panic "service.route_matcher_generic.go: routes is required" / "default is required".
- `ValidateRouteConfig`: Empty prefix, prefix without "/", invalid authorization/balancer.type, sticky_sessions without header, invalid default.action/default.cluster → `*domain.RouteConfigError` with Index and Reason.

### 4.8 Constructors (fail-fast)

All of the following panic on nil for a critical parameter at application startup (NRE happens at startup, not at runtime):

- **service.NewTransparentProxy:** router, resolver, headers, logger — "service.transparent.go: ... is required".
- **service.NewConnectionResolverGeneric:** staticConns nil, pools nil — "service.connection_resolver_generic.go: staticConns/pools is required".
- **service.NewConnectionPool:** discoverer, factory, logger — "service.connection_pool.go: ... is required".
- **service.NewRouteMatcherGeneric:** After validation routes/default nil — "service.route_matcher_generic.go: routes/default is required".
- **helpers.NewConfigurableAuthProcessor:** jwt nil — "helpers.configurable_auth_processor.go: JwtService is required".
- **helpers.NewHeaderProcessorChain:** any processor nil — "helpers.header_chain.go: processor at index N is required".
- **service.NewJWTValidator:** secret nil, timeProvider nil — "service.validator.go: secret is required" / "time provider is required".
- **adapters.DiscovererHTTP:** baseURL empty, client nil — "adapters.discoverer.go: baseURL/http client is required".
- **service.NewTimeProvider:** now nil — "service.time_provider.go: now is required".

---

## 5. Architecture

### 5.1 Overview

```
Client (gRPC)
    → grpc.Server (UnknownServiceHandler)
        → TransparentProxy.Handler
            → RouteMatcher.Match(method)        → domain.Route
            → HeaderProcessor.Process(md, method) → metadata.MD / error
            → ConnectionResolver.GetConnection(ctx, route, outMD) → *grpc.ClientConn, stickyKey, instanceID / error
            → backend.NewStream(...); forwardServerToClient || forwardClientToServer
            [on error] → ConnectionResolver.OnBackendFailure(route, stickyKey, instanceID)
```

### 5.2 Components and packages

| Component | Package | Purpose |
|-----------|---------|---------|
| Entry point, config | cmd | main, LoadConfig (env + YAML), route matcher, pools, resolver, auth, proxy, grpc.Server creation |
| Domain | domain | Route, RouteConfig, DefaultRoute, ClusterID, ClusterConfig, ServiceInstance, ValidateRouteConfig, StickySessionHeader |
| Proxy, router, resolver, pool | service | TransparentProxy, routeMatcherGeneric (NewRouteMatcherGeneric, Match), connectionResolverGeneric (NewConnectionResolverGeneric, GetConnection, OnBackendFailure, Close), connectionPool (NewConnectionPool, GetConnectionRoundRobin, GetConnectionForKey), timeProvider (NewTimeProvider) |
| Header chain and auth | helpers | HeaderProcessorChain, ConfigurableAuthProcessor (JWT per route); GetSessionID, GetAuthToken, GetHeaderValue |
| JWT tokens | auth | TokenClaims, CreateToken, ParseAndVerify (token.go) |
| JWT validator | service | JWTValidator, NewJWTValidator (validator.go) — implements interfaces.JwtService |
| Adapters | adapters | DiscovererHTTP: GET /v1/instances, POST /v1/unregister/{id} |
| Interfaces | interfaces | Discoverer, ConnectionPool, ConnectionResolver, RouteMatcher, HeaderProcessor, JwtService, TimeProvider; mocks in interfaces/mock |

### 5.3 Data flow

- **Route:** method string → RouteMatcher.Match → Route (cluster, authorization, balancer).
- **Headers:** incoming metadata + method → HeaderProcessorChain → outgoing metadata or gRPC error.
- **Connection:** (route, outgoing MD) → ConnectionResolver.GetConnection → static conn or pool.GetConnectionRoundRobin/GetConnectionForKey → *grpc.ClientConn.
- **Proxying:** serverStream ↔ clientStream via emptypb.Empty, no body parsing.

### 5.4 Dependencies (wiring from main)

- Route matcher: service.NewRouteMatcherGeneric(cfg.Routes) from domain.RouteConfig.
- Static clusters: map[ClusterID]*grpc.ClientConn; dynamic: map[ClusterID]ConnectionPool (DiscovererHTTP + factory + service.NewConnectionPool).
- Resolver: service.NewConnectionResolverGeneric(staticConns, dynamicPools).
- Auth: service.NewTimeProvider(now), service.NewJWTValidator(secret, timeProvider), helpers.NewConfigurableAuthProcessor(jwtService, cfg.Routes.Routes), helpers.NewHeaderProcessorChain(authProcessor).
- Proxy: service.NewTransparentProxy(pathRouter, clusterResolver, headerChain, logger).
- Server: grpc.NewServer(grpc.UnknownServiceHandler(transparentProxy.Handler)).

---

## 6. Configuration

- **SERVICE_PORT_GRPC** — Incoming gRPC port (1–65535), required.
- **CONFIG_PATH** — Path to YAML (absolute or relative), required.
- **JWT_SECRET** — Required if at least one route has `authorization: required`.
- **RETRY_COUNT** — Number of NewStream attempts for dynamic clusters (integer ≥ 1), required.
- **RETRY_TIMEOUT_MS** — Timeout per attempt in milliseconds (integer > 0), required.

Example YAML:

```yaml
default:
  action: use_cluster
  use_cluster: my_auth

routes:
  - prefix: /myservice/login
    cluster: my_auth
    authorization: none
    balancer:
      type: round_robin

  - prefix: /myservice/myservice
    cluster: my_service
    authorization: required
    balancer:
      type: sticky_sessions
      header: session-id

clusters:
  my_auth:
    type: static
    address: myauth:50051

  my_service:
    type: dynamic
    discoverer_url: http://mydiscoverer:8080
    discoverer_interval_ms: 5000
```

Prefix normalization (in config): if it does not start with `/` it is added; trailing `*` is stripped (prefix match is used).

---

## 7. External integrations

- **Discoverer (HTTP):** Contract per [MyDiscoverer OpenAPI](../MyDiscoverer/api/my-discoverer.openapi.yaml). GET `{baseURL}/v1/instances` — response `{"instances": [{"instance_id", "ipv4", "port"}, ...]}`. Connection address to instance is `ipv4:port`. POST `{baseURL}/v1/unregister/{instance_id}` — 200 OK or error (e.g. 500).
- **Backend (gRPC):** Static address or instances from discoverer; TLS is not used for outgoing connections in the current implementation (insecure credentials).

---

## 8. Build, run, tests

- Build: `go build -o mygateway ./cmd`
- Run: Set env (SERVICE_PORT_GRPC, CONFIG_PATH, JWT_SECRET if needed, RETRY_COUNT, RETRY_TIMEOUT_MS), then `./mygateway`.
- Graceful shutdown: SIGINT/SIGTERM → GracefulStop with 5 s timeout, then Stop if needed.
- Tests: `go test ./...`

---

## 9. Limitations and extensibility

### 9.1 Unimplemented features

- **Retry for multi-message client stream/bidi:** Transfer during active RPC is supported for unary and server-stream (replay first client message), but for client-stream/bidi with multiple client messages full replay of the message sequence is not implemented.

### 9.2 Other

- Authorization is only JWT + session-id from route config; other schemes would require new processors in the chain.
- Retry: For dynamic clusters on NewStream error, up to RETRY_COUNT attempts with RETRY_TIMEOUT_MS per attempt; on each failure OnBackendFailure (unregister + remove from pool), next attempt on another instance.
- Stream transfer on backend failure: For unary/server-stream in dynamic clusters, on error during forward the gateway opens a new stream to another instance, forwards original metadata and replays the first client message.
- After server-stream transfer duplicate messages are possible (new backend starts stream from the beginning), as the backend does not support resume by position.
- TLS for outgoing backend connections is not used (insecure).
- Adding new routes and clusters is via YAML; new header processors — implement `HeaderProcessor` and add to the chain in main.

This document reflects the current state of the code and may be updated when functionality or architecture changes.
