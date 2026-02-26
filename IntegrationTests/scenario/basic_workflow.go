package scenario

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/metadata"
)

const scenarioBasicWorkflow = "basic_workflow"

func init() {
	Register(scenarioBasicWorkflow, runBasicWorkflow)
}

func runBasicWorkflow(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	sessionID := sessionIDPrefix + "-" + time.Now().Format("20060102150405")

	// 1. Login
	token, err := Login(ctx, client, cfg, sessionID)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	// session-id must match session_id claim in JWT (the value we sent in Login), not the token itself
	md := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID,
	)
	ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

	// 2. MyServiceEcho
	var myserviceID string
	for i := 0; i < 10; i++ {
		podName, err := Echo(ctxWithAuth, client, myserviceID)
		if err != nil {
			return fmt.Errorf("myservice_echo (iteration %d): %w", i, err)
		}
		if myserviceID == "" {
			myserviceID = podName
		}
	}

	// 3. MyServiceShutdown
	if err := Shutdown(ctxWithAuth, client, myserviceID); err != nil {
		return fmt.Errorf("myservice_shutdown: %w", err)
	}

	return nil
}
