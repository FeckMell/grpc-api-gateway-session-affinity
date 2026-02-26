# MyDiscoverer

## 1. Purpose

MyDiscoverer is an HTTP service for **instance registration and discovery**. It stores instance metadata (identifier, service type, IPv4, port, timestamp, TTL) in Redis and exposes three operations: register an instance, unregister by identifier, and get the list of all registered instances. Consumers (orchestration, load balancers, or other services) use it to register worker instances and get the current set of endpoints. Storage is indexed by `instance_id` with TTL so inactive instances expire automatically.

---

## 2. API overview

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/register` | Register or update one instance (body: instance_id, service_type, ipv4, port, timestamp, ttl_ms). |
| POST | `/v1/unregister/{instance_id}` | Remove the instance with the given identifier from the registry. |
| GET | `/v1/instances` | Return the list of all registered instances (instance_id, ipv4, port). |

Formal specification: [api/my-discoverer.openapi.yaml](api/my-discoverer.openapi.yaml).

---

## 3. API methods (detail)

### 3.1 POST /v1/register

**Request**

- **Method and path:** `POST /v1/register`
- **Headers:** `Content-Type: application/json`
- **Body (JSON):** `RegisterRequest` — all fields required.

| Field | Type | Description |
|-------|------|-------------|
| `instance_id` | string | Unique instance identifier. |
| `service_type` | string | Service type (e.g. grpc). |
| `ipv4` | string | Instance IPv4 address. |
| `port` | integer | Port number (non-zero). |
| `timestamp` | string (date-time) | Timestamp. |
| `ttl_ms` | integer | TTL in milliseconds (positive). |

**Success response**

- **HTTP status:** 200
- **Body:** none.

**Error responses**

| HTTP | error.code | When | Why |
|------|------------|------|-----|
| 400 | `bad_parameter` | Request body is not valid JSON or cannot be bound. | Echo Bind returns error; handler returns `bad_parameter` with message like "invalid request body". |
| 400 | `bad_parameter` | Request field validation failed. | `fromRegisterRequest` returns `BadParameterError`; message is one of: `instance_id is required`, `service_type is required`, `ipv4 is required`, `port is required`, `ttl_ms is required`. Triggered when field is missing, empty string, or (for port/ttl_ms) zero/negative. |
| 500 | `internal_server_error` | Cache write error. | `WriteValue` returns error (e.g. Redis unavailable, marshal error). Handler propagates it; HTTPErrorHandler maps unknown/internal errors to 500. |

**Method logic**

1. Bind request body to `RegisterRequest` (JSON). On bind error → return `bad_parameter` (400).
2. Convert request to domain model `Instance` via `fromRegisterRequest`: validate required fields and port/ttl_ms; on validation error → return that error (wrapped; internal `BadParameterError` yields 400).
3. Call `cache.WriteValue(ctx, req.InstanceId, instance, req.TtlMs)`. On error → return error (wrapped; internal `InternalServerError` from cache yields 500).
4. Return 200 with no body.

---

### 3.2 POST /v1/unregister/{instance_id}

**Request**

- **Method and path:** `POST /v1/unregister/{instance_id}`
- **Path parameter:** `instance_id` (string) — identifier of the instance to remove.
- **Body:** none.

**Success response**

- **HTTP status:** 200
- **Body:** none.

**Error responses**

| HTTP | error.code | When | Why |
|------|------------|------|-----|
| 500 | `internal_server_error` | Cache delete error. | `DeleteValue` returns error (e.g. Redis unavailable). Handler propagates → 500. |

**Method logic**

1. Call `cache.DeleteValue(ctx, instanceId)`.
2. On error → return that error (500).
3. On success → return 200 with no body. Deleting a non-existent key is also success (Redis DEL is idempotent for the API).

---

### 3.3 GET /v1/instances

**Request**

- **Method and path:** `GET /v1/instances`
- **Body:** none.

**Success response**

- **HTTP status:** 200
- **Body (JSON):** `InstancesResponse`: `{ "instances": [ { "instance_id": string, "ipv4": string, "port": integer }, ... ] }`

**Error responses**

| HTTP | error.code | When | Why |
|------|------------|------|-----|
| 404 | `entity_not_found` | No keys in cache or failed to read/unpack values. | Redis implementation for `ListAllValues` returns `entity_not_found` when there are no keys with the prefix or no value could be read/unpacked; handler propagates → 404. |
| 500 | `internal_server_error` | Error enumerating keys in store. | `ListAllValues` returns `internal_server_error` (e.g. Redis KEYS error); handler propagates → 500. |

**Method logic**

1. Call `cache.ListAllValues(ctx)`.
2. On error → return that error (404 for `entity_not_found`, 500 for `internal_server_error`).
3. On success → convert `[]domain.Instance` to `InstancesResponse` (fields: instance_id, ipv4, port) and return 200 with JSON body.

---

## 4. Error code reference

Errors are returned in the body as `{ "error": { "code": "<code>", "message": "<message>" } }`. HTTP status is determined by the code.

| code | HTTP | Semantics | Where used |
|------|------|-----------|------------|
| `bad_parameter` | 400 | Invalid or incomplete request (bind/validation). | POST /v1/register (invalid JSON, missing/empty/zero required fields). |
| `entity_not_found` | 404 | Data not found (empty or unreadable cache). | GET /v1/instances when ListAllValues returns no keys or no readable values. |
| `internal_server_error` | 500 | Server or store error. | POST /v1/register (WriteValue), POST /v1/unregister (DeleteValue), GET /v1/instances (ListAllValues). Also for any error that is not a MyError returned by handlers (message: "an internal server error has occurred"). |

Mapping is implemented in [service/http_error.go](service/http_error.go) (`NewErrorCodeToStatusCodeMaps`). Handlers use `ToMyError` (errors.As), so wrapped errors still yield the correct status.

---

## 5. User scenarios

### 5.1 Success scenarios

**Registration — success**

- **As** a consumer **I want** to register an instance **so that** it appears in the instance list with correct metadata.
- **Given** valid JSON body with all required fields (instance_id, service_type, ipv4, port, timestamp, ttl_ms),
- **When** I send `POST /v1/register` with this body,
- **Then** I get 200 with no body, and the instance is stored in Redis under key `instance:{instance_id}` with the given TTL.
- **Example:** `POST /v1/register` with body `{"instance_id":"inst-1","service_type":"grpc","ipv4":"127.0.0.1","port":9000,"timestamp":"2026-02-19T12:00:00Z","ttl_ms":300000}` → 200, no body.

**Unregister — success**

- **As** a consumer **I want** to remove an instance **so that** it no longer appears in the list.
- **When** I send `POST /v1/unregister/inst-1`,
- **Then** I get 200 with no body, key `instance:inst-1` is deleted (or was already missing).
- **Example:** `POST /v1/unregister/inst-1` → 200, no body.

**Get instance list — success (non-empty)**

- **As** a consumer **I want** to get the list of all registered instances **so that** I can use their endpoints.
- **When** at least one instance is stored and I send `GET /v1/instances`,
- **Then** I get 200 and JSON body with array `instances` containing for each instance_id, ipv4 and port.
- **Example:** `GET /v1/instances` → 200, `{"instances":[{"instance_id":"inst-1","ipv4":"10.0.0.1","port":8080}]}`.

**Get instance list — success (empty)**

- **When** cache returns an empty list (e.g. in tests mock returns `([], nil)`),
- **Then** I get 200 and `{"instances":[]}`.
- **Note:** Current Redis implementation when there are no keys does not return an empty list but returns `entity_not_found`, so in production an empty registry yields 404 (see failure scenario below). Behavior 200 with empty list is only possible if the cache implementation returns `([]T, nil)` for "no instances".

---

### 5.2 Failure scenarios

**Registration — invalid JSON**

- **When** I send `POST /v1/register` with body `{invalid`,
- **Then** I get 400 and `{"error":{"code":"bad_parameter","message":"invalid request body",...}}`.
- **Why:** Body is not valid JSON; Bind fails; handler returns BadParameterError.

**Registration — missing or invalid fields**

- **When** I send valid JSON but omit `instance_id` (or set empty string),
- **Then** I get 400 and `{"error":{"code":"bad_parameter","message":"instance_id is required"}}`.
- **Why:** `fromRegisterRequest` checks required fields and returns BadParameterError with this message. Same for `service_type`, `ipv4`, `port` (zero or missing) and `ttl_ms` (zero or negative) with messages: `service_type is required`, `ipv4 is required`, `port is required`, `ttl_ms is required`.

**Registration — store failure**

- **When** Redis is unavailable or WriteValue fails (e.g. marshal error),
- **Then** I get 500 and `{"error":{"code":"internal_server_error","message":"..."}}`.
- **Why:** Cache returns InternalServerError; handler propagates; HTTPErrorHandler maps to 500.

**Unregister — store failure**

- **When** Redis is unavailable or DeleteValue fails,
- **Then** I get 500 and `{"error":{"code":"internal_server_error","message":"..."}}`.
- **Why:** Handler propagates error from DeleteValue → 500.

**Get list — empty or unreadable cache**

- **When** there are no keys in cache or no value could be read/unpacked (Redis implementation returns `entity_not_found`),
- **Then** I get 404 and `{"error":{"code":"entity_not_found","message":"Entity not found"}}`.
- **Why:** ListAllValues returns entity_not_found when key set is empty or elements could not be loaded; handler propagates → 404.

**Get list — store failure**

- **When** Redis KEYS (or equivalent) call fails,
- **Then** I get 500 and `{"error":{"code":"internal_server_error","message":"..."}}`.
- **Why:** ListAllValues returns internal_server_error; handler propagates → 500.

---

## 6. Cache contract (summary)

The service uses an abstract cache interface ([interfaces/cache.go](interfaces/cache.go)); default implementation is Redis ([adapters/myredis/cache.go](adapters/myredis/cache.go)).

| Method | Success | Errors (code, when) |
|--------|---------|----------------------|
| **WriteValue** | `nil` | `internal_server_error` — marshal or store write error. |
| **ListAllValues** | `(items, nil)` | `entity_not_found` — no keys or could not read/unpack values; `internal_server_error` — key enumeration error (e.g. Redis). |
| **DeleteValue** | `nil` | `internal_server_error` — store delete error. |

---

## 7. Configuration, run and build

**Configuration (environment variables)**

| Variable | Description | Required |
|----------|-------------|----------|
| `REDIS_ADDR` | Redis address (e.g. `redis://localhost:6379`). | Yes |
| `SERVICE_PORT_HTTP` | HTTP server port. | Yes |

**Run**

```bash
go mod download
REDIS_ADDR=redis://localhost:6379 SERVICE_PORT_HTTP=8080 go run cmd/main.go
```

**Build and tests**

```bash
make generate   # OpenAPI codegen and mocks
make test       # All tests
make build      # Binary mydiscoverer
make test-redis # Redis adapter tests (docker-compose)
```

**Stack**

- Go, Echo v4, Redis (go-redis/v8), go-kit/log, oapi-codegen, moq.

**Project structure**

- `domain/` — Domain model (Instance)
- `handlers/` — HTTP handlers and converters (types from OpenAPI)
- `service/` — Error types and HTTP error handler
- `adapters/myredis/` — Redis cache implementation
- `interfaces/` — Cache interface and mocks
- `cmd/` — Entry point and configuration
