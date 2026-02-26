package scenario

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/metadata"
)

const scenarioStreamSubscription = "stream_subscription"

func init() {
	Register(scenarioStreamSubscription, runStreamSubscription)
}

// runStreamSubscription tests the MyServiceSubscribe stream method.
// It verifies that messages are received every 5 seconds with incrementing index,
// and that sticky session is maintained throughout the stream.
func runStreamSubscription(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
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

	// Create authenticated context
	md := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID,
	)
	ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

	// 2. Subscribe to stream and read messages
	maxMessages := 3 // Read 3 messages to verify incrementing index
	result, err := SubscribeStream(ctxWithAuth, client, echoValue, maxMessages, "MyServiceSubscribe")
	if err != nil {
		return fmt.Errorf("subscribe stream: %w", err)
	}
	// Stream is automatically cancelled when maxMessagesToReceive is reached

	// 3. Optional: MyServiceShutdown
	// Note: After stream cancellation, we can optionally shutdown the instance
	// However, since stream is cancelled, the instance might still have the session
	// We'll try shutdown but it might fail if session was cleared
	shutdownCtx, shutdownCancel := context.WithTimeout(ctxWithAuth, 5*time.Second)
	defer shutdownCancel()

	// Try shutdown, but don't fail if it errors (session might be cleared)
	if err := Shutdown(shutdownCtx, client, result.PodName); err != nil {
		// Log but don't fail - this is acceptable after stream cancellation
		fmt.Printf("Note: MyServiceShutdown after stream cancellation returned error (expected): %v\n", err)
	}

	return nil
}
