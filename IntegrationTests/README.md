# Integration Tests (MyService)

Integration tests for the MyService system per [requirements.md](../requirements.md). Intended for running by an AI agent or manually.

## Purpose

Tests cover scenarios from the requirements: single entry point (FR-1), authentication (FR-2), MyService* calls via gateway (FR-MG-1), Login / MyServiceEcho / MyServiceShutdown contracts (FR-AUTH-1, FR-MS-1, FR-MS-7), sticky session (session-id in metadata). Each scenario creates a new gRPC client and context.

## Prerequisites

- System is up: `make up` or `docker-compose up` from the repository root.
- Gateway (MyGateway) is available at `localhost:10000` (or set via `--gateway` / `GATEWAY_ADDR`).
- For scenarios with successful login, the default is MyAuth’s built-in test user: username `TestUser`, password `TestPassword`, role `admin`. Override via flags `--username` / `--password` or env `TEST_USERNAME` / `TEST_PASSWORD` if needed.

## Build and code generation

1. Install Go (1.22+). For proto codegen you need `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc` (from root: `make install`).
2. Generate code from proto: `make generate`.
3. Build: `make build` → binary `integrationtests` in the current directory (or `./bin` if desired).

## Running for AI

- **List scenarios:**  
  `./integrationtests --list`  
  Prints scenario names and exits with code 0.

- **Run a scenario:**  
  `./integrationtests <scenario_name>`  
  or  
  `./integrationtests --scenario=<name>`

- **Options:**
  - `--gateway=host:port` — API Gateway address (default `localhost:10000`). Can use `GATEWAY_ADDR`.
  - `--username=...` — Login username (default MyAuth test user `TestUser`; or `TEST_USERNAME`).
  - `--password=...` — Login password (default test user password; or `TEST_PASSWORD`).
  - `--compose-file=PATH` — Path to docker-compose.yml (required for `gateway_error_unavailable`; default `COMPOSE_FILE` or `../docker-compose.yml`).

- **Exit code:** 0 — scenario passed; non-zero — error (gRPC or validation). AI can run the binary and check exit code and optionally stdout/stderr.

## Project structure

- `cmd/main.go` — Entry point, flag parsing, scenario registration and execution.
- `api/Api.proto` — Copy of MyServiceAPI proto (Login, MyServiceEcho, MyServiceSubscribe, MyServiceShutdown); Go client is generated from it.
- `pb/` — Generated code from proto (do not edit).
- `scenario/` — Go files with each scenario’s logic and assertions. Each scenario gets its own context and creates a new gRPC client.
- `scenario_md/` — Markdown scenario descriptions (steps, error codes, requirement links).

## Scenarios

- **basic_workflow** — Login, MyServiceEcho, MyServiceShutdown (FR-1, FR-2, FR-AUTH-1, FR-MS-1, FR-MS-7, sticky session). Validates LoginResponse (token, expires_at, role) and EchoResponse (all fields per FR-MS-1).
- **login_errors** — Login errors: empty username, empty session_id, invalid credentials (FR-AUTH-2).
- **sticky_session** — Two clients with different session-id hit different instances (FR-4).
- **stream_subscription** — MyServiceSubscribe, stream and sticky session checks (FR-MS-2).
- **session_transfer** — Instance shutdown, session transfer, no instances (FR-5).
- **myservice_authentication_errors** — Missing/invalid JWT, session-id mismatch (FR-MGW-5).
- **system_overload** — More clients than instances (FR-6, FR-MGW-5).
- **gateway_error_unauthenticated** — MyServiceEcho without/invalid token → UNAUTHENTICATED "missing or invalid token" (FR-MGW-5).
- **gateway_error_unavailable** — MyServiceEcho when backend unavailable → UNAVAILABLE "backend service unavailable" (FR-MGW-5). Requires `--compose-file`.
- **myservice_shutdown_errors** — MyServiceShutdown without session or session mismatch → PERMISSION_DENIED (FR-MS-7).
- **no_instances_available** — All instances busy → RESOURCE_EXHAUSTED "all instances are busy" (FR-MGW-5, FR-5).

## Adding scenarios

1. Add a new file in package `scenario` (e.g. `scenario/my_scenario.go`).
2. Implement `Run(ctx context.Context, cfg *Config) error`: create a new gRPC connection to the gateway, run steps, validate responses; return `error` on failure.
3. Register the scenario in `scenario.All()` (or in `cmd` via `scenario.Register(...)` depending on registration scheme).
4. The scenario is then available by name: `./integrationtests my_scenario`.

## Relation to requirements.md

Tests cover:

- **FR-1:** Single entry point via MyGateway (all calls go to the gateway).
- **FR-2:** Authentication via Login and JWT in subsequent calls.
- **FR-MG-1:** Routing Login → MyAuth, MyService* → MyService.
- **FR-AUTH-1, FR-MS-1, FR-MS-7:** Login, MyServiceEcho, MyServiceShutdown contracts and response field checks.
- **Sticky session:** Sending `session-id` in metadata for MyService* calls.
