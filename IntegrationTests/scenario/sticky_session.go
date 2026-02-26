package scenario

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/metadata"
)

const scenarioStickySession = "sticky_session"

func init() {
	Register(scenarioStickySession, runStickySession)
}

func runStickySession(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Create first client connection
	client1, dispose1, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client1: %w", err)
	}
	defer dispose1()

	// Create second client connection
	client2, dispose2, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client2: %w", err)
	}
	defer dispose2()

	// Generate different session IDs for each client
	timestamp := time.Now().Format("20060102150405")
	sessionID1 := sessionIDPrefix + "-client1-" + timestamp
	sessionID2 := sessionIDPrefix + "-client2-" + timestamp

	// 1. Login both clients
	token1, err := Login(ctx, client1, cfg, sessionID1)
	if err != nil {
		return fmt.Errorf("login (client1): %w", err)
	}

	token2, err := Login(ctx, client2, cfg, sessionID2)
	if err != nil {
		return fmt.Errorf("login (client2): %w", err)
	}

	// Create authenticated contexts for both clients
	md1 := metadata.Pairs(
		"authorization", token1,
		"session-id", sessionID1,
	)
	ctxWithAuth1 := metadata.NewOutgoingContext(ctx, md1)

	md2 := metadata.Pairs(
		"authorization", token2,
		"session-id", sessionID2,
	)
	ctxWithAuth2 := metadata.NewOutgoingContext(ctx, md2)

	// 2. MyServiceEcho - alternating between clients, 10 iterations (10 calls per client)
	var podName1, podName2 string
	for i := 0; i < 10; i++ {
		// Client 1 echo
		pod1, err := Echo(ctxWithAuth1, client1, podName1)
		if err != nil {
			return fmt.Errorf("myservice_echo (client1, iteration %d): %w", i, err)
		}
		if podName1 == "" {
			podName1 = pod1
		}

		// Client 2 echo
		pod2, err := Echo(ctxWithAuth2, client2, podName2)
		if err != nil {
			return fmt.Errorf("myservice_echo (client2, iteration %d): %w", i, err)
		}
		if podName2 == "" {
			podName2 = pod2
		}
	}

	// Verify that clients are on different instances
	if podName1 == podName2 {
		return fmt.Errorf("both clients are on the same instance: %s (expected different instances for sticky session)", podName1)
	}

	// 3. MyServiceShutdown for both clients
	if err := Shutdown(ctxWithAuth1, client1, podName1); err != nil {
		return fmt.Errorf("myservice_shutdown (client1): %w", err)
	}

	if err := Shutdown(ctxWithAuth2, client2, podName2); err != nil {
		return fmt.Errorf("myservice_shutdown (client2): %w", err)
	}

	return nil
}
