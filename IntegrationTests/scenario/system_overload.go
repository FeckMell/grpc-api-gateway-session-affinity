package scenario

import (
	"context"
	"fmt"
	"strings"
	"time"

	"integrationtests/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const scenarioSystemOverload = "system_overload"

func init() {
	Register(scenarioSystemOverload, runSystemOverload)
}

// runSystemOverload tests the scenario when there are more clients than available instances.
// With 3 instances and 4 clients, one client should receive an error from MyGateway.
func runSystemOverload(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// We'll create 4 clients (more than the 3 available instances)
	numClients := 4
	clients := make([]struct {
		client    pb.MyServiceAPIClient
		dispose   func()
		sessionID string
		token     string
	}, numClients)

	// Create all client connections
	for i := 0; i < numClients; i++ {
		client, dispose, err := CreateMSClient(cfg.GatewayAddr)
		if err != nil {
			return fmt.Errorf("create client%d: %w", i+1, err)
		}
		clients[i].client = client
		clients[i].dispose = dispose
		defer dispose()
	}

	// Generate unique session IDs for each client
	timestamp := time.Now().Format("20060102150405")
	for i := 0; i < numClients; i++ {
		clients[i].sessionID = fmt.Sprintf("%s-client%d-%s", sessionIDPrefix, i+1, timestamp)
	}

	// 1. Login all clients
	for i := 0; i < numClients; i++ {
		token, err := Login(ctx, clients[i].client, cfg, clients[i].sessionID)
		if err != nil {
			return fmt.Errorf("login (client%d): %w", i+1, err)
		}
		clients[i].token = token
	}

	// 2. Try to establish sessions for all clients
	// With 3 instances and 4 clients, at least 1 should fail
	// Due to HASH_SET, multiple clients might hash to the same instance
	var successfulClients []struct {
		idx     int
		podName string
	}
	var failedClients []struct {
		idx   int
		error error
	}

	for i := 0; i < numClients; i++ {
		md := metadata.Pairs(
			"authorization", clients[i].token,
			"session-id", clients[i].sessionID,
		)
		ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

		// Try to call MyServiceEcho
		podName, err := Echo(ctxWithAuth, clients[i].client, "")
		if err != nil {
			// Check if this is an expected error
			st, ok := status.FromError(err)
			if ok {
				code := st.Code()
				msg := st.Message()

				// Accept RESOURCE_EXHAUSTED or UNAVAILABLE from MyGateway
				if code == codes.ResourceExhausted || code == codes.Unavailable {
					failedClients = append(failedClients, struct {
						idx   int
						error error
					}{idx: i, error: err})
					fmt.Printf("Client %d correctly received error from MyGateway: %v (code: %v)\n", i+1, err, code)
					continue
				}

				// Also accept Internal error with "Session conflict" message
				// This happens when MyGateway routes to a busy instance via HASH_SET
				// This is acceptable behavior when instances are busy
				if code == codes.Internal && strings.Contains(msg, "Session conflict") {
					failedClients = append(failedClients, struct {
						idx   int
						error error
					}{idx: i, error: err})
					fmt.Printf("Client %d received session conflict error (instance busy): %v\n", i+1, err)
					continue
				}
			}

			// Other errors are unexpected
			return fmt.Errorf("unexpected error for client%d: %w", i+1, err)
		}
		successfulClients = append(successfulClients, struct {
			idx     int
			podName string
		}{idx: i, podName: podName})
	}

	// Verify that at least 1 client failed (since we have 4 clients and only 3 instances)
	if len(failedClients) == 0 {
		return fmt.Errorf("expected at least 1 client to fail (4 clients, 3 instances), but all clients succeeded")
	}

	// Verify that not all clients failed
	if len(successfulClients) == 0 {
		return fmt.Errorf("expected at least some clients to succeed, but all clients failed")
	}

	fmt.Printf("Successfully verified: %d clients got instances, %d client(s) failed as expected\n", len(successfulClients), len(failedClients))
	for _, fc := range failedClients {
		fmt.Printf("  - Client %d failed with: %v\n", fc.idx+1, fc.error)
	}

	// 3. Cleanup: Shutdown successful clients
	for _, sc := range successfulClients {
		md := metadata.Pairs(
			"authorization", clients[sc.idx].token,
			"session-id", clients[sc.idx].sessionID,
		)
		ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

		if err := Shutdown(ctxWithAuth, clients[sc.idx].client, sc.podName); err != nil {
			// Log but don't fail the test
			fmt.Printf("Warning: failed to shutdown client%d: %v\n", sc.idx+1, err)
		}
	}

	return nil
}
