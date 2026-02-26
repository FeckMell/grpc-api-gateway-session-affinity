# Integration Tests (MyService)

Integration tests for the MyService system. Intended for running by an AI agent or manually.

## Purpose

Tests cover scenarios: single entry point, authentication, MyService* calls via gateway, Login / MyServiceEcho / MyServiceShutdown contracts, sticky session (session-id in metadata). Each scenario creates a new gRPC client and context.

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
- `scenario_md/` — Markdown scenario descriptions (steps, error codes).

## Scenario documentation

Detailed step-by-step documentation for each scenario:

- [basic_workflow](scenario_md/basic_workflow.md)
- [login_errors](scenario_md/login_errors.md)
- [sticky_session](scenario_md/sticky_session.md)
- [stream_subscription](scenario_md/stream_subscription.md)
- [stream_subscription_myservice_stop](scenario_md/stream_subscription_myservice_stop.md)
- [session_transfer](scenario_md/session_transfer.md)
- [myservice_authentication_errors](scenario_md/myservice_authentication_errors.md)
- [myservice_shutdown_errors](scenario_md/myservice_shutdown_errors.md)
- [no_instances_available](scenario_md/no_instances_available.md)
- [system_overload](scenario_md/system_overload.md)
- [gateway_error_unauthenticated](scenario_md/gateway_error_unauthenticated.md)
- [gateway_error_unavailable](scenario_md/gateway_error_unavailable.md)

## Scenarios

- **basic_workflow** — Login, MyServiceEcho, MyServiceShutdown, sticky session. Validates LoginResponse (token, expires_at, role) and EchoResponse (all fields).
- **login_errors** — Login errors: empty username, empty session_id, invalid credentials.
- **sticky_session** — Two clients with different session-id hit different instances.
- **stream_subscription** — MyServiceSubscribe, stream and sticky session checks.
- **session_transfer** — Instance shutdown, session transfer, no instances.
- **myservice_authentication_errors** — Missing/invalid JWT, session-id mismatch.
- **system_overload** — More clients than instances.
- **gateway_error_unauthenticated** — MyServiceEcho without/invalid token → UNAUTHENTICATED "missing or invalid token".
- **gateway_error_unavailable** — MyServiceEcho when backend unavailable → UNAVAILABLE "backend service unavailable". Requires `--compose-file`.
- **myservice_shutdown_errors** — MyServiceShutdown without session or session mismatch → PERMISSION_DENIED.
- **no_instances_available** — All instances busy → RESOURCE_EXHAUSTED "all instances are busy".

## Adding scenarios

1. Add a new file in package `scenario` (e.g. `scenario/my_scenario.go`).
2. Implement `Run(ctx context.Context, cfg *Config) error`: create a new gRPC connection to the gateway, run steps, validate responses; return `error` on failure.
3. Register the scenario in `scenario.All()` (or in `cmd` via `scenario.Register(...)` depending on registration scheme).
4. The scenario is then available by name: `./integrationtests my_scenario`.

## What the tests cover

- **Single entry point:** All calls go via MyGateway.
- **Authentication:** Login and JWT in subsequent calls.
- **Routing:** Login → MyAuth, MyService* → MyService.
- **Contracts:** Login, MyServiceEcho, MyServiceShutdown and response field checks.
- **Sticky session:** Sending `session-id` in metadata for MyService* calls.
