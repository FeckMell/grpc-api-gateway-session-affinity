# MyAuth

## Overview

MyAuth is an authentication service in Go. It provides a gRPC API with a single method **Login**: validate credentials by username and password and issue a JWT.

### Main features

- **Login**: Validates username and password via store (Redis), issues JWT with fields login, role, session_id, expires_at, issued_at; HMAC-SHA256 signature; token lifetime is set from configuration at startup
- **User storage**: Read from Redis by key `external_user:{login}` (JSON with `password`, `role`); implementation can be swapped to Postgres via the UserStore interface without changing gRPC or business logic

### Tech stack

- **Go** — main language
- **gRPC** (HTTP/2) — API transport
- **Redis** — user store (prefix `external_user`)
- **go-kit/log** — structured logging

---

## Proto and role in the unified API

- **Proto in package `my_service`**: Contract is in [api/api.proto](api/api.proto): service `MyServiceAPI`, RPC `Login`. Full method name: `/my_service.MyServiceAPI/Login`.
- For the client this method is part of the unified API `my_service.MyServiceAPI`, aggregated by MyGateway by method name (Login → auth_cluster).

---

## API

### RPC Login

Validates credentials and issues a JWT.

#### Request parameters (LoginRequest)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | Yes | User login (non-empty) |
| `password` | string | No | Password |
| `session_id` | string | Yes | Session identifier (non-empty) |

#### Response parameters (LoginResponse)

| Field | Type | Description |
|-------|------|-------------|
| `token` | string | JWT for API access (HMAC-SHA256, payload: login, role, session_id, expires_at, issued_at) |
| `expires_at` | google.protobuf.Timestamp | Token expiry time |
| `role` | string | User role |

#### Request validation

- `request == nil` → error `bad_parameter` ("request is nil")
- `username == ""` → error `bad_parameter` ("username is required")
- `session_id == ""` → error `bad_parameter` ("session_id is required")

#### Responses and errors

**Success** — `LoginResponse` with `token`, `expires_at`, `role` is returned.

**Errors**: domain codes `service.AuthError` are mapped to gRPC status before sending to the client (unary interceptor in [service/grpc_error.go](service/grpc_error.go)):

- `bad_parameter` → `codes.InvalidArgument` — invalid request (nil, empty username or session_id)
- `invalid_user_or_password` → `codes.PermissionDenied` — user not found or wrong password; message "invalid username or password"
- `internal_server_error` → `codes.Internal` — error reading from store or creating token
- other errors → `codes.Unknown` with message "internal error" (original error is logged)

---

## Login method logic

### Main flow

1. **Request validation**: Check `req != nil`, `req.Username != ""`, `req.SessionId != ""`. On violation — return `BadParameterError`.
2. **Get user**: Call `UserStore.GetByLogin(ctx, req.Username)`. On `EntityNotFound` or password mismatch (`user.Password != req.Password`) — return `InvalidUserOrPasswordError` with message "invalid username or password". On any other store error — `InternalServerError`.
3. **Issue JWT**: Call `JwtService.CreateToken(login, role, sessionID, validTill, now)` where `validTill = now.Add(TokenExpiration)` (from config). On error — `InternalServerError` ("failed to create token").
4. **Build response**: Fill `LoginResponse`: `Token`, `ExpiresAt` (timestamppb.New(validTill)), `Role`.

### Edge cases

- **nil request** → `bad_parameter`, "request is nil"
- **Empty username** → `bad_parameter`, "username is required"
- **Empty session_id** → `bad_parameter`, "session_id is required"
- **User not found** → `invalid_user_or_password`, "invalid username or password"
- **Wrong password** → `invalid_user_or_password`, "invalid username or password"
- **Store error** (not EntityNotFound) → `internal_server_error`, "failed to get user"
- **CreateToken error** → `internal_server_error`, "failed to create token"

---

## Case coverage and results (test coverage)

Below are the cases covered by tests in [handlers/grpc_test.go](handlers/grpc_test.go) and the corresponding result or error code.

### RPC Login

| Case | Condition / input | Result |
|------|-------------------|--------|
| nil request | `req == nil` | Error, code `bad_parameter` |
| empty username | `Username: ""`, rest set | Error, code `bad_parameter` |
| empty session_id | `SessionId: ""`, rest set | Error, code `bad_parameter` |
| user not found | GetByLogin returns EntityNotFound | Error, code `invalid_user_or_password` |
| store internal error | GetByLogin returns arbitrary error | Error, code `internal_server_error` |
| wrong password | Request password does not match store | Error, code `invalid_user_or_password` |
| CreateToken error | CreateToken returns error | Error, code `internal_server_error` |
| success | Valid request, user found, password matches, CreateToken succeeds | Success: `token`, `role`, `expires_at` in response; `expires_at` after current time |

---

## Technologies

### Language and frameworks

#### Go
Main language. Used for all service components.

#### gRPC (google.golang.org/grpc)
gRPC server over HTTP/2. Used to handle Login and register service `MyServiceAPI`.

#### go-kit/log
Structured logging. Used for startup, config, and shutdown logs.

---

### Data store

#### Redis
Used as user store.

**Library**: `go-redis/redis/v8` (UniversalClient)

**Keys and values**:
- Key: `external_user:{login}` (prefix `external_user`)
- Value: JSON with `password` and `role` (string)

**Operations**: Read by key via `Get(ctx, key)`; on `redis.Nil` return error `EntityNotFound`, which in the Login handler is treated as "invalid username or password".

**Implementation**: [adapters/redis/user_store.go](adapters/redis/user_store.go), interface — [interfaces/user_store.go](interfaces/user_store.go) (`UserStore`). Implementation can be replaced with Postgres without changing gRPC or business logic.

---

### JWT

#### Token format
String `base64(payload).base64(signature)`:
- **payload** — JSON with `login`, `role`, `session_id`, `expires_at` (RFC3339), `issued_at` (RFC3339)
- **signature** — HMAC-SHA256(secret, payload), secret from config (`JWT_SECRET`)

**Implementation**: [service/jwt.go](service/jwt.go) — `CreateToken` and `ParseAndVerify`; [handlers/jwt_service.go](handlers/jwt_service.go) — `JwtService` adapter implementing [interfaces/jwt_service.go](interfaces/jwt_service.go) and passing secret from config.

---

## Architecture

The project follows **Clean Architecture**:

```
MyAuth/
├── domain/              # Domain models
│   └── user.go          # User (Login, Password, Role)
├── handlers/            # gRPC handlers (Presentation Layer)
│   ├── grpc.go          # grpcServer — MyServiceAPIServer implementation
│   ├── jwt_service.go   # JWT adapter with secret from config
│   ├── api.pb.go        # Generated types from proto
│   └── api_grpc.pb.go   # Generated gRPC server
├── service/             # Business logic and utilities (Application Layer)
│   ├── jwt.go           # JWT creation and verification
│   ├── errors.go        # AuthError and error codes
│   └── grpc_error.go    # AuthError to gRPC status mapping, unary interceptor
├── interfaces/          # Interfaces (Ports)
│   ├── user_store.go    # UserStore
│   └── jwt_service.go   # JwtService
├── adapters/redis/      # Adapters (Infrastructure Layer)
│   ├── user_store.go    # Redis UserStore implementation
│   └── config.go       # Redis client setup
└── cmd/                 # Application entry point
    ├── main.go          # gRPC server init and start
    └── config.go        # Config load (SERVICE_PORT_GRPC, REDIS_ADDR, JWT_SECRET, TOKEN_EXPIRATION)
```

**Layers**:
- **Domain** — User model, no dependencies
- **Handlers** — gRPC Login handler and JWT adapter
- **Service** — JWT logic and error types
- **Interfaces** — UserStore and JwtService contracts
- **Adapters** — Redis UserStore implementation

---

## Requirements compliance (requirements.md)

The document [requirements.md](../requirements.md) defines the following for **MyAuthService (myauth)**. Below is how the current implementation complies.

| Requirement (requirements.md) | Status | Implementation |
|------------------------------|--------|-----------------|
| API: gRPC, port from SERVICE_PORT_GRPC (default 5001), HTTP/2 only | Done | [cmd/config.go](cmd/config.go) — SERVICE_PORT_GRPC; [cmd/main.go](cmd/main.go) — gRPC server on `:SERVICE_PORT_GRPC` |
| Implements only **Login** (MyServiceAPI); MyServiceEcho/MyServiceSubscribe not declared | Done | Proto has only Login; MyServiceAPI with one method Login |
| Store — Redis: key external_user:{login}, value JSON (password, role) | Done | [adapters/redis/user_store.go](adapters/redis/user_store.go) — prefix `external_user`, JSON with password and role |
| Login response: token (JWT with login, role, valid_till), valid_till, role; signature from config secret | Done | [handlers/grpc.go](handlers/grpc.go) — response with token, expires_at, role; [service/jwt.go](service/jwt.go) — HMAC-SHA256(secret) |
| Token lifetime from config at startup (TOKEN_EXPIRATION) | Done | [cmd/config.go](cmd/config.go) — TOKEN_EXPIRATION; passed to grpcServer |
| DB read via interface (UserStore) for Postgres swap | Done | [interfaces/user_store.go](interfaces/user_store.go); main wires Redis implementation |
| Handler errors mapped to gRPC status (InvalidArgument, PermissionDenied, Internal) | Done | [service/grpc_error.go](service/grpc_error.go) — AuthError to codes; unary interceptor in [cmd/main.go](cmd/main.go) |

---

## Differences from requirements.md

Requirements in [requirements.md](../requirements.md) are aligned with the implementation for the unified API (`my_service.MyServiceAPI`), per-service proto, and MyGateway routing by method name. Remaining differences; requirements can be updated or the service adjusted.

| Aspect | In requirements.md | In MyAuth implementation |
|--------|--------------------|---------------------------|
| **LoginRequest** | Login request fields not listed. | Request: **username**, **password**, **session_id** (required). Response — token, expires_at, role. Worth specifying `session_id` in requirements if required. |
| **JWT payload** | "login, role, valid_till". | Payload also includes: **session_id**, **issued_at** ([service/jwt.go](service/jwt.go)). Either document extended payload in requirements or simplify implementation to login/role/valid_till. |

Error-to-gRPC mapping is implemented: interceptor in [service/grpc_error.go](service/grpc_error.go), domain codes mapped to status codes (InvalidArgument, PermissionDenied, Internal).

---

## Configuration

### Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVICE_PORT_GRPC` | gRPC server port | `5001` |
| `REDIS_ADDR` | Redis address | `redis://localhost:6379` |
| `JWT_SECRET` | JWT signing secret | **Required**, service will not start without it |
| `TOKEN_EXPIRATION` | Token lifetime (duration, e.g. `1h`, `30m`) | `1h` |

Config loading: [cmd/config.go](cmd/config.go).

---

## Usage examples

### Calling Login (grpcurl)

```bash
grpcurl -plaintext -d '{
  "username": "user1",
  "password": "secret",
  "session_id": "sess-123"
}' localhost:5001 my_service.MyServiceAPI/Login
```

**Sample response**:

```json
{
  "token": "eyJsb2dpbiI6InVzZXIxIiwicm9sZSI6InVzZXIiLC...",
  "expiresAt": "2026-02-16T14:00:00Z",
  "role": "user"
}
```

---

## Development

### Running the service

```bash
# Install dependencies
go mod download

# Run with default variables
go run cmd/main.go

# Run with custom config
SERVICE_PORT_GRPC=5001 REDIS_ADDR=redis://localhost:6379 JWT_SECRET=mysecret TOKEN_EXPIRATION=30m go run cmd/main.go
```

### Proto code generation

```bash
make generate
```

Generates [handlers/api.pb.go](handlers/api.pb.go) and [handlers/api_grpc.pb.go](handlers/api_grpc.pb.go) from [api/api.proto](api/api.proto).

### Testing

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# By package
go test ./handlers/...
go test ./service/...
go test ./adapters/redis/...
```

---

## Health Check

MyAuth provides a gRPC Health Check endpoint for availability. Implemented with `google.golang.org/grpc/health`.

### Usage

Health Check is available via the standard gRPC Health API:

```bash
# Check with grpcurl
grpcurl -plaintext localhost:5001 grpc.health.v1.Health/Check
```

**Success response**:
```json
{
  "status": "SERVING"
}
```

### Implementation

- Health Check service is registered in `cmd/main.go` at server start
- Status is set to `SERVING` when the service starts
- MyGateway uses Health Check to determine instance availability

### Testing

Health Check tests are in [cmd/health_test.go](cmd/health_test.go):

```bash
go test ./cmd/... -run TestHealthCheck
```

---

## Possible improvements

Possible future improvements:

### 1. Map ErrEntityNotFound to codes.NotFound
**Current**: In `service/grpc_error.go` there is mapping `ErrEntityNotFound` → `codes.NotFound`, but it is not used in Login.

**Improvement**: Use `codes.NotFound` when user is not found instead of `codes.PermissionDenied`. Would require changing error handling in `handlers/grpc.go`.

### 2. Wrong-password error handling
**Current**: In `handlers/grpc.go:68` when password is wrong, `nil` is passed as the inner error.

**Improvement**: Add more error detail or a dedicated error type to distinguish "user not found" vs "wrong password" in logs.

---

## License

[Specify license if applicable]
