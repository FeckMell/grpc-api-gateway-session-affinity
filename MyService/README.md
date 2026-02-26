# MyService

## Overview

MyService is a gRPC service on .NET 8. It provides a gRPC API with methods **MyServiceEcho**, **MyServiceSubscribe**, and **MyServiceShutdown** as part of the unified API `my_service.MyServiceAPI`, aggregated via the MyGateway proxy.

### Main features

- **MyServiceEcho**: Unary RPC method returning `EchoResponse` with fields `client_value`, `server_value` (constant `"my_service"`), `pod_name`, `server_session_id`, `client_session_id`, `index` (0), `server_method`
- **MyServiceSubscribe**: Server stream sending `EchoResponse` messages every 5 seconds with an incrementing `index`
- **MyServiceShutdown**: Unary RPC method returning `ShutdownResponse` to the client, then terminating the process (Exit), which stops the container. Works only when client session matches server session.
- **MyDiscoverer registration**: On startup, registration and heartbeat loop run via `DiscovererRegistrationService.StartRegistrationAndHeartbeat()`; up to 100 registration attempts with 2 s interval; then periodic heartbeat to keep the record in EDS
- **Local session binding**: One static `session_id` per instance, stored in `MSShared`; on mismatch with current value — response "Session conflict"
- **gRPC Health Check**: gRPC health check is wired for MyGateway

### Tech stack

- **.NET 8** — main framework
- **gRPC** (HTTP/2) — API transport
- **Kestrel** — web server
- **Grpc.AspNetCore** — gRPC library for ASP.NET Core

---

## Proto and role in the unified API

- **Proto in package `my_service`**: Contract is defined in [MyService/api/Api.proto](MyService/api/Api.proto): service `MyServiceAPI`, RPCs `MyServiceEcho`, `MyServiceSubscribe`, `MyServiceShutdown`. Full method names: `/my_service.MyServiceAPI/MyServiceEcho`, `/my_service.MyServiceAPI/MyServiceSubscribe`, `/my_service.MyServiceAPI/MyServiceShutdown`.
- For the client these methods belong to the unified API `my_service.MyServiceAPI`, aggregated by MyGateway by path prefix (MyServiceEcho / MyServiceSubscribe / MyServiceShutdown → grpc_cluster via EDS).

---

## API

### RPC MyServiceEcho

Unary RPC method returning a structured `EchoResponse`.

#### Request parameters (EchoRequest)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `value` | string | Yes | Value for echo (used as session_id) |

#### Response parameters (EchoResponse)

| Field | Type | Description |
|-------|------|-------------|
| `client_value` | string | Value from client (request.value) |
| `server_value` | string | Constant `"my_service"` |
| `pod_name` | string | Unique instance identifier (from HOSTNAME) |
| `server_session_id` | string | Current session_id on server (from MSShared) |
| `client_session_id` | string | session-id from request header |
| `index` | int | Message number (for MyServiceEcho always 0) |
| `server_method` | string | Method name, e.g. `"MyServiceEcho"` |

#### Logic

1. `ValidateAndSetClientSession(request.Value)` is called; it checks session conflict and sets the new session in `MSShared`.
2. If `session_id` is already set and does not match `request.Value` → `RpcException` with code `Internal` and message "Session conflict: was={old_session}, now={new_session}".
3. Otherwise `session_id = request.Value` is set in `MSShared` and `EchoResponse` with filled fields is returned.

---

### RPC MyServiceSubscribe

Server stream sending `EchoResponse` messages every 5 seconds.

#### Request parameters (EchoRequest)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `value` | string | Yes | Value for stream (used as session_id) |

#### Response parameters (EchoResponse, stream)

Each message in the stream is an `EchoResponse` with the same fields; `index` increments (0, 1, 2, …), `server_method` = `"MyServiceSubscribe"`.

#### Logic

1. `ValidateAndSetClientSession(request.Value)` is called to check and set the session.
2. If `session_id` is already set and does not match `request.Value` → `RpcException` with code `Internal`.
3. Otherwise `session_id = request.Value` is set in `MSShared`.
4. Every 5 seconds an `EchoResponse` message is sent with current fields and incrementing `index`.
5. Stream continues until client cancellation or error.

---

### RPC MyServiceShutdown

Unary RPC method: sends response to client, then terminates the process (stops the container).

#### Request parameters (ShutdownRequest)

Empty message (no fields).

#### Response parameters (ShutdownResponse)

| Field | Type | Description |
|-------|------|-------------|
| `pod_name` | string | Unique instance identifier (from HOSTNAME) |
| `server_session_id` | string | Current session_id on server (from MSShared) |
| `client_session_id` | string | session-id from request header |
| `index` | int | 0 |
| `server_method` | string | `"MyServiceShutdown"` |

#### Logic

1. Verify server session is not empty. If empty → `RpcException` with code `PermissionDenied` and message "Server session is not set".
2. Verify client session matches server session. If not → `RpcException` with code `PermissionDenied` and mismatch message.
3. If both checks pass, build and send response to client with `ShutdownResponse` fields.
4. After sending the response, process termination is triggered (`Environment.Exit(0)`), so the container stops.

---

## MyDiscoverer registration

On application startup, registration and heartbeat run automatically via `DiscovererRegistrationService.StartRegistrationAndHeartbeat()`:

1. POST request is sent to `{url}/register` with body in RegisterRequest format: `instance_id` (from `HOSTNAME`), `service_type` ("grpcserver"), `host` (auto-detected IPv4), `port`, `last_seen` (UTC RFC3339), `ttl_ms` (default 300000), `assigned_client_session_id` (null on startup).
2. Up to 100 attempts with 2 second interval until registration succeeds.
3. After successful registration, periodic heartbeat runs (interval from `REGISTRATION_HEARTBEAT_INTERVAL_MS`, default 120000 ms) to keep the record in EDS.
4. On each heartbeat the current `assigned_client_session_id` from `MSShared` is sent (if session is set) so MyDiscoverer can track instance occupancy correctly.

**Note**: Session is not cleared automatically when the client disconnects. The instance remains occupied until instance restart or until a new session is set (which will cause a conflict if the instance is already occupied).

---

## Configuration

### Environment variables

**Note**: If any required environment variable is missing or invalid, the application exits with an error code.

| Variable | Description | Default | Required | Validation |
|----------|-------------|---------|----------|------------|
| `SERVICE_PORT_GRPC` | gRPC server port | `5000` | No | Must be a valid integer; invalid value causes application exit |
| `MY_DISCOVERER_URL` | MyDiscoverer URL for registration | (not set) | **Yes** | Must be set; missing causes exit with error code |
| `REGISTRATION_TTL_MS` | TTL for MyDiscoverer record, ms (max 600000) | `300000` | No | Must be in range 1–600000; invalid value falls back to default |
| `REGISTRATION_HEARTBEAT_INTERVAL_MS` | Re-registration (heartbeat) interval, ms | `120000` | No | Must be > 0; invalid value falls back to default |
| `JWT_SECRET` | Secret for JWT verification (must match MyAuth) | (not set) | **Yes** | Must be set; missing causes exit with error code |

**Auto-detected parameters:**

- **Instance ID**: Environment variable `HOSTNAME`, set by Docker by default (first 12 chars of container ID). If `HOSTNAME` is not set, application exits with error.
- **Discoverable host**: Container IPv4 is auto-detected via `NetworkHelper.GetPrimaryIPv4Address()`. If detection fails, application exits with error.

### Startup configuration checks

On startup the application validates all environment variables via `MSConfig.Parse()`:

- **`MY_DISCOVERER_URL`**: If not set, application exits with error code 1.
- **`HOSTNAME`**: Used as unique instance id (instance_id in MyDiscoverer and pod_name in gRPC responses). Docker sets this by default to container ID. If `HOSTNAME` is not set, application exits with error code 1.
- **IPv4 auto-detection**: Container IPv4 for discoverable host is auto-detected. If detection fails, application exits with error code 1.
- **`JWT_SECRET`**: If not set or empty, application exits with error code 1.
- **`SERVICE_PORT_GRPC`**: If value cannot be parsed as a number, application exits with error.
- **`REGISTRATION_TTL_MS`**: If invalid (not a number, ≤ 0 or > 600000), default 300000 is used.
- **`REGISTRATION_HEARTBEAT_INTERVAL_MS`**: If invalid (not a number or ≤ 0), default 120000 is used.

**Docker Compose with replicas**: When using `deploy.replicas` in docker-compose.yml, the application uses `HOSTNAME` (unique container ID per replica, set by Docker) and auto-detects IPv4 for each container. No extra configuration is needed.

### Authentication

Calls to **MyServiceEcho**, **MyServiceSubscribe**, and **MyServiceShutdown** require header `Authorization: <token>` (JWT issued by MyAuth) and header `session-id` matching `session_id` in the token. If missing, expired, invalid format, or session mismatch, gRPC status **Unauthenticated** (gRPC code 16) is returned.

Authentication is done via `AuthInterceptor`, which validates the JWT using `JwtVerifier`.

---

## Architecture

### Project structure

```
MyService/
├── MyService.sln              # Solution file
├── MyService/                 # Main project
│   ├── MyService.csproj       # Project file
│   ├── Program.cs             # Entry point, Kestrel and gRPC setup
│   ├── MSConfig.cs            # Configuration class (env parsing)
│   ├── MSShared.cs            # Static class for shared config and session
│   ├── EchoService.cs         # MyServiceAPI implementation (MyServiceEcho, MyServiceSubscribe, MyServiceShutdown)
│   ├── DiscovererRegistrationService.cs  # MyDiscoverer registration and heartbeat
│   ├── AuthInterceptor.cs     # gRPC interceptor for authentication
│   ├── JwtVerifier.cs         # JWT token verification
│   ├── NetworkHelper.cs       # Container IPv4 auto-detection
│   └── api/
│       └── Api.proto          # Proto contract
├── Dockerfile                 # Multi-stage Dockerfile
├── Makefile                   # Development commands
└── README.md                  # Documentation
```

### Components

- **MSConfig**: Class for parsing and validating environment variables. `Parse()` reads all variables, validates them, and on error logs to console and calls `Environment.Exit()`.
- **MSShared**: Static class holding shared configuration (`Config`) and session state (`AssignedSession`). Initialized in `Program.cs` via `MSShared.Initialize(config)`. All session operations are thread-safe (lock).
- **DiscovererRegistrationService**: Service for registering the instance with MyDiscoverer. Uses `MSShared.Config` for configuration and `MSShared.GetAssignedSession()` for current session in heartbeat. `StartRegistrationAndHeartbeat()` runs the retry loop and infinite heartbeat loop.
- **EchoService**: gRPC method implementation. Uses `MSShared.Config.InstanceId` for `pod_name` and `MSShared` for session. `ValidateAndSetClientSession()` checks session conflict and sets the new session.
- **NetworkHelper**: Static class for container IPv4 auto-detection. `GetPrimaryIPv4Address()` returns the first non-loopback IPv4 of an active network interface.

### Startup flow

1. `Program.cs` calls `MSConfig.Parse()` to read and validate configuration.
2. `MSShared.Initialize(config)` is called to initialize shared config access.
3. Kestrel and gRPC server are configured.
4. Services are registered in the DI container.
5. On `ApplicationStarted`, `DiscovererRegistrationService.StartRegistrationAndHeartbeat()` is started as a background task.

---

## Implementation summary

| Feature | Status | Implementation |
|------------------------------|--------|-----------------|
| gRPC server on port from `SERVICE_PORT_GRPC` (default 5000), HTTP/2 only | Done | [Program.cs](MyService/Program.cs) — port from `SERVICE_PORT_GRPC` (default 5000), HTTP/2 via `HttpProtocols.Http2` |
| Implements **MyServiceEcho**, **MyServiceSubscribe**, **MyServiceShutdown** (MyServiceAPI) | Done | [EchoService.cs](MyService/EchoService.cs) — methods `MyServiceEcho`, `MyServiceSubscribe`, `MyServiceShutdown` |
| Local session binding: one static `session_id` per instance; on mismatch — "Session conflict" | Done | [EchoService.cs](MyService/EchoService.cs) — `ValidateAndSetClientSession()`, session stored in `MSShared` |
| On startup (if `MY_DISCOVERER_URL` set) — registration with MyDiscoverer (`POST /register`) with `HOSTNAME` and auto-detected IPv4, up to 100 attempts with 2 s interval | Done | [DiscovererRegistrationService.cs](MyService/DiscovererRegistrationService.cs) — `StartRegistrationAndHeartbeat()`, uses `MSShared.Config` for parameters |
| EchoResponse with client_value, server_value="my_service", pod_name, session ids, index, server_method | Done | [EchoService.cs](MyService/EchoService.cs) — `BuildEchoResponse()`, all fields populated |
| MyServiceShutdown: ShutdownResponse to client, then Exit to stop container | Done | [EchoService.cs](MyService/EchoService.cs) — `MyServiceShutdown()`, session match check, then `Environment.Exit(0)` |
| Request auth: header `Authorization: <token>` and `session-id`, JWT and session_id check, Unauthenticated on error (including MyServiceShutdown) | Done | [AuthInterceptor.cs](MyService/AuthInterceptor.cs) — JWT and session-id check, returns `StatusCode.Unauthenticated` (gRPC code 16) |
| gRPC health check wired for MyGateway | Done | [Program.cs](MyService/Program.cs) — `AddGrpcHealthChecks()` and `MapGrpcHealthChecksService()` |

---

## Non-functional requirements

### Reliability

- **Registration retry**: On MyDiscoverer registration failure the service retries until success (up to 100 attempts with 2 second interval).
- **MyDiscoverer unavailable**: If MyDiscoverer is unavailable after a successful registration, the service logs heartbeat errors but continues serving client requests.
- **Graceful shutdown**: On stop, active connections and streams are shut down cleanly.

### Session management

- **Session release**: Session is not released automatically when the client disconnects. The instance remains occupied (`assigned_client_session_id` kept in `MSShared`) until instance restart or a new session is set (which causes a conflict if the instance is already occupied by another session).
- **Thread-safety**: All session operations in `MSShared` are thread-safe (lock).

---

## Development

### Prerequisites

- **.NET 8 SDK**: https://dotnet.microsoft.com/download

### Running the service locally

```bash
# Restore dependencies (automatic on build)
dotnet restore

# Run with required variables
MY_DISCOVERER_URL=http://localhost:8080 JWT_SECRET=dev-secret dotnet run --project MyService/MyService.csproj

# Run with custom configuration
SERVICE_PORT_GRPC=5000 MY_DISCOVERER_URL=http://localhost:8080 JWT_SECRET=dev-secret dotnet run --project MyService/MyService.csproj

# Note: HOSTNAME is usually set by the OS. To set explicitly:
HOSTNAME=local-instance SERVICE_PORT_GRPC=5000 MY_DISCOVERER_URL=http://localhost:8080 JWT_SECRET=dev-secret dotnet run --project MyService/MyService.csproj
```

### Using the Makefile

```bash
# Show available commands
make help

# Build the project
make build

# Run locally
make run

# Run tests (when test projects are added)
make test

# Clean build artifacts
make clean
```

### Building the Docker image

```bash
# From MyService project root
docker build -t myservice .

# Run container (HOSTNAME set by Docker, IPv4 auto-detected)
docker run -p 5000:5000 \
  -e SERVICE_PORT_GRPC=5000 \
  -e MY_DISCOVERER_URL=http://mydiscoverer:8080 \
  -e JWT_SECRET=dev-secret \
  myservice
```

### Proto code generation

Proto files are compiled automatically on build via `Grpc.Tools` (configured in [MyService.csproj](MyService/MyService.csproj)). Generated files are under `obj/Debug/net8.0/` or `obj/Release/net8.0/`.

---

## Usage examples

### Calling MyServiceEcho (grpcurl)

```bash
grpcurl -plaintext -H "authorization: <token>" -H "session-id: session-123" \
  -d '{"value": "session-123"}' \
  localhost:5000 my_service.MyServiceAPI/MyServiceEcho
```

**Sample response**:

```json
{
  "clientValue": "session-123",
  "serverValue": "my_service",
  "podName": "abc123def456",
  "serverSessionId": "session-123",
  "clientSessionId": "session-123",
  "index": 0,
  "serverMethod": "MyServiceEcho"
}
```

### Calling MyServiceSubscribe (grpcurl)

```bash
grpcurl -plaintext -H "authorization: <token>" -H "session-id: session-123" \
  -d '{"value": "session-123"}' \
  localhost:5000 my_service.MyServiceAPI/MyServiceSubscribe
```

**Sample stream responses**:

```
{
  "clientValue": "session-123",
  "serverValue": "my_service",
  "podName": "abc123def456",
  "serverSessionId": "session-123",
  "clientSessionId": "session-123",
  "index": 0,
  "serverMethod": "MyServiceSubscribe"
}
{
  "clientValue": "session-123",
  "serverValue": "my_service",
  "podName": "abc123def456",
  "serverSessionId": "session-123",
  "clientSessionId": "session-123",
  "index": 1,
  "serverMethod": "MyServiceSubscribe"
}
...
```

### Calling MyServiceShutdown (grpcurl)

Request must include metadata `authorization` and `session-id`. Client session must match server session. After the response the server terminates the process.

```bash
grpcurl -plaintext -H "authorization: <token>" -H "session-id: session-123" \
  -d '{}' localhost:5000 my_service.MyServiceAPI/MyServiceShutdown
```

**Sample response**:

```json
{
  "podName": "abc123def456",
  "serverSessionId": "session-123",
  "clientSessionId": "session-123",
  "index": 0,
  "serverMethod": "MyServiceShutdown"
}
```

**Errors**:
- If server session is empty: `PermissionDenied` with message "Server session is not set"
- If client session does not match server session: `PermissionDenied` with mismatch message

---

## License

[Specify license if applicable]
