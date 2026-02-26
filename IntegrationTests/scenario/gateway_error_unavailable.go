package scenario

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"integrationtests/docker"
	"integrationtests/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Expected gateway error message when backend is unavailable (FR-MGW-5).
const expectedMsgBackendUnavailable = "backend service unavailable"

const scenarioGatewayErrorUnavailable = "gateway_error_unavailable"

const myserviceServiceName = "myservice"

func init() {
	Register(scenarioGatewayErrorUnavailable, runGatewayErrorUnavailable)
}

// runGatewayErrorUnavailable verifies FR-MGW-5: when backend (MyService) is unavailable,
// MyServiceEcho returns UNAVAILABLE with message "backend service unavailable".
// It stops the myservice service, calls MyServiceEcho, then starts the service again.
func runGatewayErrorUnavailable(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	if cfg.ComposePath == "" {
		return fmt.Errorf("compose path is required for this scenario (set --compose-file or COMPOSE_FILE)")
	}
	absPath, err := filepath.Abs(cfg.ComposePath)
	if err != nil {
		return fmt.Errorf("resolve compose path: %w", err)
	}
	workDir := filepath.Dir(absPath)

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	// 1. Login to get valid token (gateway will accept auth, then fail on backend)
	sessionID := sessionIDPrefix + "-unavailable-" + time.Now().Format("20060102150405")
	token, err := Login(ctx, client, cfg, sessionID)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	md := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID,
	)
	ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

	// 2. Stop MyService so gateway has no healthy backend
	if err := docker.StopService(workDir, myserviceServiceName); err != nil {
		return fmt.Errorf("stop myservice: %w", err)
	}
	defer func() {
		if startErr := docker.StartService(workDir, myserviceServiceName); startErr != nil {
			fmt.Printf("Warning: failed to start myservice after test: %v\n", startErr)
		}
	}()

	// Wait for gateway to see no instances / connection failures
	time.Sleep(8 * time.Second)

	// 3. MyServiceEcho must return UNAVAILABLE with exact message
	_, err = client.MyServiceEcho(ctxWithAuth, &pb.EchoRequest{Value: echoValue})
	if err == nil {
		return fmt.Errorf("expected error when backend unavailable, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status, got: %w", err)
	}
	if st.Code() != codes.Unavailable {
		return fmt.Errorf("expected UNAVAILABLE, got %v: %v", st.Code(), st.Message())
	}
	if st.Message() != expectedMsgBackendUnavailable {
		return fmt.Errorf("expected message %q, got %q", expectedMsgBackendUnavailable, st.Message())
	}

	return nil
}
