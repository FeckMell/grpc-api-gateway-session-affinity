package scenario

import (
	"context"
	"fmt"
	"time"

	"integrationtests/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const scenarioLoginErrors = "login_errors"

func init() {
	Register(scenarioLoginErrors, runLoginErrors)
}

// runLoginErrors tests various Login error scenarios:
// 1. Validation errors (empty username, empty session_id)
// 2. Credential errors (invalid username, invalid password)
func runLoginErrors(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	// Test 1: Empty username
	if err := testEmptyUsername(ctx, client); err != nil {
		return fmt.Errorf("test empty username: %w", err)
	}

	// Test 2: Empty session_id
	if err := testEmptySessionID(ctx, client, cfg); err != nil {
		return fmt.Errorf("test empty session_id: %w", err)
	}

	// Test 3: Invalid username (user not found)
	if err := testInvalidUsername(ctx, client); err != nil {
		return fmt.Errorf("test invalid username: %w", err)
	}

	// Test 4: Invalid password
	if err := testInvalidPassword(ctx, client, cfg); err != nil {
		return fmt.Errorf("test invalid password: %w", err)
	}

	return nil
}

// testEmptyUsername tests Login with empty username.
// Expected: INVALID_ARGUMENT with message about username being required.
func testEmptyUsername(ctx context.Context, client pb.MyServiceAPIClient) error {
	sessionID := sessionIDPrefix + "-" + time.Now().Format("20060102150405")

	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  "", // Empty username
		Password:  "password",
		SessionId: sessionID,
	})

	if err == nil {
		return fmt.Errorf("expected error for empty username, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.InvalidArgument {
		return fmt.Errorf("expected INVALID_ARGUMENT, got %v: %v", st.Code(), st.Message())
	}

	// Check that error message mentions username
	if st.Message() == "" {
		return fmt.Errorf("expected error message about username, got empty message")
	}

	return nil
}

// testEmptySessionID tests Login with empty session_id.
// Expected: INVALID_ARGUMENT with message about session_id being required.
func testEmptySessionID(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  cfg.Username,
		Password:  cfg.Password,
		SessionId: "", // Empty session_id
	})

	if err == nil {
		return fmt.Errorf("expected error for empty session_id, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.InvalidArgument {
		return fmt.Errorf("expected INVALID_ARGUMENT, got %v: %v", st.Code(), st.Message())
	}

	// Check that error message mentions session_id
	if st.Message() == "" {
		return fmt.Errorf("expected error message about session_id, got empty message")
	}

	return nil
}

// testInvalidUsername tests Login with non-existent username.
// Expected: PERMISSION_DENIED with message about invalid credentials.
func testInvalidUsername(ctx context.Context, client pb.MyServiceAPIClient) error {
	sessionID := sessionIDPrefix + "-" + time.Now().Format("20060102150405")

	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  "nonexistent_user_12345", // Non-existent username
		Password:  "password",
		SessionId: sessionID,
	})

	if err == nil {
		return fmt.Errorf("expected error for invalid username, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.PermissionDenied {
		return fmt.Errorf("expected PERMISSION_DENIED, got %v: %v", st.Code(), st.Message())
	}

	// Check that error message mentions invalid credentials
	if st.Message() == "" {
		return fmt.Errorf("expected error message about invalid credentials, got empty message")
	}

	return nil
}

// testInvalidPassword tests Login with wrong password for existing user.
// Expected: PERMISSION_DENIED with message about invalid credentials.
func testInvalidPassword(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	sessionID := sessionIDPrefix + "-" + time.Now().Format("20060102150405")

	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  cfg.Username,           // Valid username
		Password:  "wrong_password_12345", // Wrong password
		SessionId: sessionID,
	})

	if err == nil {
		return fmt.Errorf("expected error for invalid password, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status error, got: %w", err)
	}

	if st.Code() != codes.PermissionDenied {
		return fmt.Errorf("expected PERMISSION_DENIED, got %v: %v", st.Code(), st.Message())
	}

	// Check that error message mentions invalid credentials
	if st.Message() == "" {
		return fmt.Errorf("expected error message about invalid credentials, got empty message")
	}

	return nil
}
