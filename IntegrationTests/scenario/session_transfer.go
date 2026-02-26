package scenario

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const scenarioSessionTransfer = "session_transfer"

func init() {
	Register(scenarioSessionTransfer, runSessionTransfer)
}

// runSessionTransfer tests session transfer to another instance when the current instance shuts down.
// The scenario:
// 1. Login
// 2. Echo twice → get service_id_1
// 3. Shutdown service_id_1
// 4. Echo twice → get service_id_2
// 5. Shutdown service_id_2
// 6. Echo twice → get service_id_3
// 7. Shutdown service_id_3
// 8. Echo → receive error (no instances available)
func runSessionTransfer(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	sessionID := sessionIDPrefix + "-transfer-" + time.Now().Format("20060102150405")

	// 1. Login
	token, err := Login(ctx, client, cfg, sessionID)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// Create authenticated context
	md := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID,
	)
	ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

	// Helper function to echo twice and get pod name with retry logic
	// After instance shutdown, MyGateway may need time to update EDS, so we retry on UNAVAILABLE
	echoTwice := func(expectedPodName string) (string, error) {
		var podName string
		maxRetries := 5
		retryDelay := 2 * time.Second

		for i := 0; i < 2; i++ {
			var pod string
			var err error

			// Retry logic for echo calls after instance shutdown
			for retry := 0; retry < maxRetries; retry++ {
				pod, err = Echo(ctxWithAuth, client, expectedPodName)
				if err == nil {
					break
				}

				// Check if it's a retryable error (UNAVAILABLE after shutdown)
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.Unavailable && retry < maxRetries-1 {
					fmt.Printf("  Retry %d/%d after UNAVAILABLE error: %v\n", retry+1, maxRetries-1, err)
					time.Sleep(retryDelay)
					continue
				}

				// Non-retryable error or last retry
				return "", fmt.Errorf("myservice_echo (iteration %d): %w", i, err)
			}

			if err != nil {
				return "", fmt.Errorf("myservice_echo (iteration %d): %w", i, err)
			}

			if podName == "" {
				podName = pod
			} else if podName != pod {
				return "", fmt.Errorf("sticky session violation: expected %q, got %q", podName, pod)
			}
		}
		return podName, nil
	}

	// 2. First instance (service_id_1)
	fmt.Printf("Step 1: Getting first instance...\n")
	podName1, err := echoTwice("")
	if err != nil {
		return fmt.Errorf("first instance echo: %w", err)
	}
	fmt.Printf("First instance: %s\n", podName1)

	// Shutdown first instance
	fmt.Printf("Step 2: Shutting down first instance %s...\n", podName1)
	if err := Shutdown(ctxWithAuth, client, podName1); err != nil {
		return fmt.Errorf("shutdown first instance: %w", err)
	}
	fmt.Printf("First instance %s shut down successfully\n", podName1)

	// Wait for MyGateway to detect the instance is down and update EDS
	// EDS updates every 5 seconds, so wait a bit longer
	// Also need time for health check to mark instance as unhealthy
	fmt.Printf("Waiting for MyGateway to detect instance shutdown...\n")
	time.Sleep(3 * time.Second)

	// 3. Second instance (service_id_2)
	fmt.Printf("Step 3: Getting second instance (session transfer)...\n")
	podName2, err := echoTwice("")
	if err != nil {
		return fmt.Errorf("second instance echo: %w", err)
	}
	if podName2 == podName1 {
		return fmt.Errorf("session not transferred: still on instance %s", podName1)
	}
	fmt.Printf("Second instance: %s (session transferred from %s)\n", podName2, podName1)

	// Shutdown second instance
	fmt.Printf("Step 4: Shutting down second instance %s...\n", podName2)
	if err := Shutdown(ctxWithAuth, client, podName2); err != nil {
		return fmt.Errorf("shutdown second instance: %w", err)
	}
	fmt.Printf("Second instance %s shut down successfully\n", podName2)

	// Wait for MyGateway to detect the instance is down and update EDS
	fmt.Printf("Waiting for MyGateway to detect instance shutdown...\n")
	time.Sleep(3 * time.Second)

	// 4. Third instance (service_id_3)
	fmt.Printf("Step 5: Getting third instance (session transfer)...\n")
	podName3, err := echoTwice("")
	if err != nil {
		return fmt.Errorf("third instance echo: %w", err)
	}
	if podName3 == podName1 || podName3 == podName2 {
		return fmt.Errorf("session not transferred: still on instance %s or %s, got %s", podName1, podName2, podName3)
	}
	fmt.Printf("Third instance: %s (session transferred from %s)\n", podName3, podName2)

	// Shutdown third instance
	fmt.Printf("Step 6: Shutting down third instance %s...\n", podName3)
	if err := Shutdown(ctxWithAuth, client, podName3); err != nil {
		return fmt.Errorf("shutdown third instance: %w", err)
	}
	fmt.Printf("Third instance %s shut down successfully\n", podName3)

	// Wait for MyGateway to detect the instance is down and update EDS
	fmt.Printf("Waiting for MyGateway to detect instance shutdown...\n")
	time.Sleep(3 * time.Second)

	// 5. No instances available - should get error
	fmt.Printf("Step 7: Attempting echo after all instances shut down (expecting error)...\n")
	_, err = Echo(ctxWithAuth, client, "")
	if err == nil {
		return fmt.Errorf("expected error when no instances available, but got success")
	}

	// Check that error is UNAVAILABLE or RESOURCE_EXHAUSTED
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	code := st.Code()
	if code != codes.Unavailable && code != codes.ResourceExhausted {
		return fmt.Errorf("expected UNAVAILABLE or RESOURCE_EXHAUSTED, got %v: %v", code, st.Message())
	}

	fmt.Printf("Received expected error (no instances available): %v (code: %v)\n", err, code)
	fmt.Printf("Session transfer scenario completed successfully!\n")

	return nil
}
