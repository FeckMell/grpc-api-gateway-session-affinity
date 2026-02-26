# Scenario: MyServiceSubscribe transfer when backend goes down

## Description

This scenario verifies that an active `MyServiceSubscribe` stream is transferred to another MyService instance when the current one goes down. The client performs Login, MyServiceEcho (to establish the session), subscribes to MyServiceSubscribe and receives the first message. Then one MyService container is stopped; the stream is expected to continue on another instance — the next `Recv()` returns a successful response, and the `pod_name` in that response **differs** from the first message's `pod_name` (confirming switch to a new instance).

**Implementation:** [`scenario/stream_subscription_myservice_stop.go`](../scenario/stream_subscription_myservice_stop.go)  
**Run:** `./integrationtests --compose-file=../docker-compose.yml stream_subscription_myservice_stop`

## Prerequisites

- Path to `docker-compose.yml` is required: `--compose-file` flag or `COMPOSE_FILE` env. Without it the scenario fails.
- Environment is expected to be up before run (`make up` or similar).
- Service `myservice` must have at least 2 containers (`docker-compose ps -q myservice`); otherwise the scenario fails.

## Steps

### 1. Connect to the system

The client creates a gRPC connection to the API Gateway at `localhost:10000`.

### 2. Authentication (Login)

**Request:**
- Method: `Login`
- Parameters:
  - `username`: user name
  - `password`: password
  - `session_id`: unique session identifier (e.g. "integration-test-session-sub-stop-{timestamp}")

**Success response:**
- `token`: JWT for subsequent requests
- `expires_at`: token expiry
- `role`: user role

### 3. Establish session (MyServiceEcho)

The client calls MyServiceEcho with the token and session-id from Login. The session is established on one of the MyService instances.

**Request:**
- Method: `MyServiceEcho`
- Parameters: `value` (e.g. "integration-test-echo")
- Metadata: `authorization`, `session-id`

**Success response:** EchoResponse with `pod_name`, `server_value == "my_service"`, etc.

### 4. Stream subscription (MyServiceSubscribe)

**Request:**
- Method: `MyServiceSubscribe` (server stream)
- Parameters: `value` (string for stream)
- Metadata: `authorization`, `session-id`

The client receives the **first** message from the stream (index = 0). Response fields are checked (`client_value`, `server_value`, `pod_name`, `index == 0`, `server_method == "MyServiceSubscribe"`). The `pod_name` is stored as the **original instance** for the later instance-change check.

### 5. Start reading from stream in a separate goroutine

The client starts a **separate goroutine** that calls `Recv()` on the stream (blocking on the next message or error). So when the container is stopped, the stream is already waiting for the next `Recv()`.

### 6. Pause 1 second

After starting the goroutine, pause for 1 second.

### 7. Stop the container serving the stream

The scenario gets the list of `myservice` containers and for each gets the hostname via `docker inspect` (in MyService, `pod_name` in responses matches the container's `HOSTNAME` env). The container whose hostname equals the first message's `pod_name` is stopped — i.e. the instance that was serving the stream. Stop is done with `docker stop <container-id>`. Other instances stay up; the gateway should pick another instance on session transfer.

### 8. Wait for response after transfer

The main flow waits for the goroutine's `Recv()` result. A **successful** response is expected (not error, not EOF).

**Success criteria:**
- No error is returned (in particular not `UNAVAILABLE`).
- The response has correct fields (`client_value`, `server_method`, `pod_name`, etc.).
- **`pod_name` in the response after transfer is different from the first message's `pod_name`** — this confirms the response came from another MyService instance.

If `pod_name` does not change, the scenario fails (switch to a new instance was expected).

### 9. Restore environment

In `defer` the scenario runs `docker start <container-id>` for the stopped container. If start fails a warning is printed, but the scenario is still considered successful if validations for steps 1–8 passed.

## Interaction diagram

```mermaid
sequenceDiagram
    participant Client
    participant Gateway as MyGateway
    participant MS1 as MyService 1
    participant MS2 as MyService 2

    Note over Client,MS2: Environment up (multiple MS instances)

    Client->>Gateway: Login(username, password, session_id)
    Gateway-->>Client: LoginResponse(token, ...)

    Client->>Gateway: MyServiceEcho(value)<br/>[authorization, session-id]
    Gateway->>MS1: Echo
    MS1-->>Gateway-->>Client: EchoResponse(pod_name=A, ...)

    Client->>Gateway: MyServiceSubscribe(value)<br/>[authorization, session-id]
    Gateway->>MS1: Subscribe
    MS1-->>Gateway-->>Client: EchoResponse index=0, pod_name=A

    Note over Client: Pause 1 s
    Note over Client: Goroutine: Recv() blocks

    Client->>Gateway: docker stop <container MS1>
    Note over MS1: One MS container stopped

    Gateway->>Gateway: OnBackendFailure, retry on another instance
    Gateway->>MS2: NewStream + replay first message
    MS2-->>Gateway-->>Client: EchoResponse (pod_name=B)

    Note over Client: pod_name B != A — scenario success

    Note over Client,MS2: Restore
    Client->>Gateway: defer docker start <container>
```

## Validations

The scenario checks:

1. **ComposePath:**
   - If `cfg.ComposePath` is empty, the scenario returns an error before any actions.

2. **Instance count:**
   - Service `myservice` has at least 2 containers; otherwise the scenario fails.

3. **Login:**
   - Login succeeds (token and response pass standard Login helper validation).

4. **MyServiceEcho:**
   - Echo succeeds; response has correct fields and `pod_name`.

5. **First stream message:**
   - MyServiceSubscribe is established successfully.
   - First message is received; fields are checked (`client_value`, `server_value`, `pod_name`, `index == 0`, `server_method == "MyServiceSubscribe"`). `pod_name` is remembered.

6. **Response after container stop (transfer):**
   - Stream is read in a separate goroutine; after stopping one container, `Recv()` returns a successful response (not `UNAVAILABLE`, not EOF).
   - Response after transfer has correct fields (`client_value`, `server_method`, non-empty `pod_name`).
   - **`pod_name` in the response after transfer is different from the first message's `pod_name`** — confirms switch to another MyService instance.

7. **Restore:**
   - In `defer`, `docker start` is run for the stopped container; on error a warning is printed but the test does not fail.

## Scenario limitations

- The scenario targets a single-request stream (`MyServiceSubscribe`): client sends one message and receives a stream of responses.
- After transfer, duplicate messages are possible (new backend may start from index=0), as the backend does not support resume by offset.
