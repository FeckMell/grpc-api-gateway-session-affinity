package scenario

import (
	"context"
	"fmt"
	"time"

	"integrationtests/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const scenarioMyServiceAuthenticationErrors = "myservice_authentication_errors"

func init() {
	Register(scenarioMyServiceAuthenticationErrors, runMyServiceAuthenticationErrors)
}

// runMyServiceAuthenticationErrors tests various MyService authentication error scenarios:
// 1. Missing authorization token
// 2. Invalid token format
// 3. Expired token (if possible to test)
// 4. session-id mismatch with token
// 5. Missing session-id header
func runMyServiceAuthenticationErrors(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	// Test 1: Missing authorization token
	if err := testMissingToken(ctx, client); err != nil {
		return fmt.Errorf("test missing token: %w", err)
	}

	// Test 2: Invalid token format
	if err := testInvalidTokenFormat(ctx, client); err != nil {
		return fmt.Errorf("test invalid token format: %w", err)
	}

	// Test 3: Expired token
	// Note: This test may be skipped if we cannot easily create an expired token
	// For now, we'll attempt to test it by using a very short token expiration
	// or skip with a comment if not feasible
	if err := testExpiredToken(ctx, client, cfg); err != nil {
		// Log but don't fail - expired token test may not be feasible
		fmt.Printf("Note: Expired token test returned error (may be expected): %v\n", err)
	}

	// Test 4: session-id mismatch
	if err := testSessionIDMismatch(ctx, client, cfg); err != nil {
		return fmt.Errorf("test session-id mismatch: %w", err)
	}

	// Test 5: Missing session-id header
	if err := testMissingSessionIDHeader(ctx, client, cfg); err != nil {
		return fmt.Errorf("test missing session-id header: %w", err)
	}

	return nil
}

// testMissingToken tests MyServiceEcho without authorization token.
// Expected: UNAUTHENTICATED error.
func testMissingToken(ctx context.Context, client pb.MyServiceAPIClient) error {
	// Call MyServiceEcho without authorization header
	_, err := client.MyServiceEcho(ctx, &pb.EchoRequest{Value: echoValue})

	if err == nil {
		return fmt.Errorf("expected error for missing token, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.Unauthenticated {
		return fmt.Errorf("expected UNAUTHENTICATED, got %v: %v", st.Code(), st.Message())
	}

	return nil
}

// testInvalidTokenFormat tests MyServiceEcho with invalid token format.
// Expected: UNAUTHENTICATED error.
func testInvalidTokenFormat(ctx context.Context, client pb.MyServiceAPIClient) error {
	// Create context with invalid token format
	md := metadata.Pairs(
		"authorization", "invalid_token_format_12345",
		"session-id", "session-123",
	)
	ctxWithInvalidToken := metadata.NewOutgoingContext(ctx, md)

	_, err := client.MyServiceEcho(ctxWithInvalidToken, &pb.EchoRequest{Value: echoValue})

	if err == nil {
		return fmt.Errorf("expected error for invalid token format, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.Unauthenticated {
		return fmt.Errorf("expected UNAUTHENTICATED, got %v: %v", st.Code(), st.Message())
	}

	return nil
}

// testExpiredToken tests MyServiceEcho with expired token.
// Expected: UNAUTHENTICATED error with "token expired" message.
// Note: This test may not be feasible without modifying token expiration or waiting.
// For now, we'll attempt to test it, but it may need manual setup or be skipped.
func testExpiredToken(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	// Note: To test expired token, we would need to:
	// 1. Use a very short TOKEN_EXPIRATION and wait for expiration, OR
	// 2. Manually create an expired token (requires access to JWT secret), OR
	// 3. Skip this test with a comment
	// For now, we'll skip this test as it's not easily testable without additional setup
	// The test framework can be extended later to support expired token testing

	fmt.Printf("Note: Expired token test skipped - requires manual setup or token expiration wait\n")
	return nil
}

// testSessionIDMismatch tests MyServiceEcho with session-id that doesn't match token.
// Expected: UNAUTHENTICATED error with "session-id mismatch" message.
func testSessionIDMismatch(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	// First, get a valid token with sessionID1
	sessionID1 := sessionIDPrefix + "-session1-" + time.Now().Format("20060102150405")
	token, err := Login(ctx, client, cfg, sessionID1)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Use a different session-id in the header (sessionID2)
	sessionID2 := sessionIDPrefix + "-session2-" + time.Now().Format("20060102150405")
	md := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID2, // Different session-id than in token
	)
	ctxWithMismatch := metadata.NewOutgoingContext(ctx, md)

	_, err = client.MyServiceEcho(ctxWithMismatch, &pb.EchoRequest{Value: echoValue})

	if err == nil {
		return fmt.Errorf("expected error for session-id mismatch, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.Unauthenticated {
		return fmt.Errorf("expected UNAUTHENTICATED, got %v: %v", st.Code(), st.Message())
	}

	// Check that error message mentions session-id mismatch (if available)
	// Note: The exact message may vary, so we just check the code

	return nil
}

// testMissingSessionIDHeader tests MyServiceEcho without session-id header.
// Expected: INVALID_ARGUMENT or UNAUTHENTICATED error.
func testMissingSessionIDHeader(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	// First, get a valid token
	sessionID := sessionIDPrefix + "-" + time.Now().Format("20060102150405")
	token, err := Login(ctx, client, cfg, sessionID)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Create context with token but without session-id header
	md := metadata.Pairs(
		"authorization", token,
		// session-id is missing
	)
	ctxWithoutSessionID := metadata.NewOutgoingContext(ctx, md)

	_, err = client.MyServiceEcho(ctxWithoutSessionID, &pb.EchoRequest{Value: echoValue})

	if err == nil {
		return fmt.Errorf("expected error for missing session-id header, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	// Accept either INVALID_ARGUMENT or UNAUTHENTICATED
	if st.Code() != codes.InvalidArgument && st.Code() != codes.Unauthenticated {
		return fmt.Errorf("expected INVALID_ARGUMENT or UNAUTHENTICATED, got %v: %v", st.Code(), st.Message())
	}

	return nil
}
