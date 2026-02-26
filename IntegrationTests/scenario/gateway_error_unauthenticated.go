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

const (
	expectedMsgMissingSessionID      = "missing session-id"
	expectedMsgMissingOrInvalidToken = "missing or invalid token"
)

const scenarioGatewayErrorUnauthenticated = "gateway_error_unauthenticated"

func init() {
	Register(scenarioGatewayErrorUnauthenticated, runGatewayErrorUnauthenticated)
}

// runGatewayErrorUnauthenticated verifies FR-MGW-5 and FR-AUTH-2: distinct errors for
// missing session-id vs missing/invalid token; and Login vs Echo behaviour for each case.
// Cases:
//  1. Both session-id and authorization not set: Login fails (session_id required), Echo fails ("missing session-id").
//  2. session-id set, authorization missing: Login passes, Echo fails ("missing or invalid token").
//  3. session-id not set, authorization set: Login fails (session_id required), Echo fails ("missing session-id").
//  4. session-id set, invalid token: Echo fails ("missing or invalid token").
func runGatewayErrorUnauthenticated(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	// Case 1: Both not set — Login fails (session_id in request body required), Echo fails (missing session-id)
	if err := case1BothNotSet(ctx, client, cfg); err != nil {
		return fmt.Errorf("case 1 (both not set): %w", err)
	}

	// Case 2: session-id set, authorization missing — Login passes, Echo fails (missing or invalid token)
	if err := case2SessionIDSetAuthMissing(ctx, client, cfg); err != nil {
		return fmt.Errorf("case 2 (session-id set, auth missing): %w", err)
	}

	// Case 3: session-id not set, authorization set — Login fails (session_id in body required), Echo fails (missing session-id)
	if err := case3SessionIDMissingAuthSet(ctx, client, cfg); err != nil {
		return fmt.Errorf("case 3 (session-id not set, auth set): %w", err)
	}

	// Case 4: session-id set, invalid token — Echo fails (missing or invalid token)
	if err := case4InvalidToken(ctx, client); err != nil {
		return fmt.Errorf("case 4 (invalid token): %w", err)
	}

	return nil
}

func case1BothNotSet(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	// Login with empty session_id in request body → INVALID_ARGUMENT "session_id is required"
	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  cfg.Username,
		Password:  cfg.Password,
		SessionId: "", // required by MyAuth
	})
	if err == nil {
		return fmt.Errorf("login with empty session_id: expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		return fmt.Errorf("login with empty session_id: expected INVALID_ARGUMENT, got %v", err)
	}
	if st.Message() != "session_id is required" {
		return fmt.Errorf("login with empty session_id: expected message about session_id, got %q", st.Message())
	}

	// Echo with no metadata → UNAUTHENTICATED "missing session-id"
	_, err = client.MyServiceEcho(ctx, &pb.EchoRequest{Value: echoValue})
	if err == nil {
		return fmt.Errorf("echo without metadata: expected error, got nil")
	}
	st, ok = status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		return fmt.Errorf("echo without metadata: expected UNAUTHENTICATED, got %v", err)
	}
	if st.Message() != expectedMsgMissingSessionID {
		return fmt.Errorf("echo without metadata: expected %q, got %q", expectedMsgMissingSessionID, st.Message())
	}
	return nil
}

func case2SessionIDSetAuthMissing(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	sessionID := sessionIDPrefix + "-case2-" + time.Now().Format("20060102150405")

	// Login with session_id in body (no auth header needed) → success
	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  cfg.Username,
		Password:  cfg.Password,
		SessionId: sessionID,
	})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// Echo with session-id set but no authorization → UNAUTHENTICATED "missing or invalid token"
	md := metadata.Pairs("session-id", sessionID)
	ctxEcho := metadata.NewOutgoingContext(ctx, md)
	_, err = client.MyServiceEcho(ctxEcho, &pb.EchoRequest{Value: echoValue})
	if err == nil {
		return fmt.Errorf("echo with session-id only: expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		return fmt.Errorf("echo with session-id only: expected UNAUTHENTICATED, got %v", err)
	}
	if st.Message() != expectedMsgMissingOrInvalidToken {
		return fmt.Errorf("echo with session-id only: expected %q, got %q", expectedMsgMissingOrInvalidToken, st.Message())
	}
	return nil
}

func case3SessionIDMissingAuthSet(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config) error {
	// Login with empty session_id in body → INVALID_ARGUMENT "session_id is required"
	_, err := client.Login(ctx, &pb.LoginRequest{
		Username:  cfg.Username,
		Password:  cfg.Password,
		SessionId: "", // required
	})
	if err == nil {
		return fmt.Errorf("login with empty session_id: expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		return fmt.Errorf("login with empty session_id: expected INVALID_ARGUMENT, got %v", err)
	}

	// Obtain a valid token for Echo (Login with session_id set)
	sessionID := sessionIDPrefix + "-case3-" + time.Now().Format("20060102150405")
	token, err := Login(ctx, client, cfg, sessionID)
	if err != nil {
		return fmt.Errorf("login for token: %w", err)
	}

	// Echo with authorization set but no session-id → UNAUTHENTICATED "missing session-id"
	md := metadata.Pairs("authorization", token)
	ctxEcho := metadata.NewOutgoingContext(ctx, md)
	_, err = client.MyServiceEcho(ctxEcho, &pb.EchoRequest{Value: echoValue})
	if err == nil {
		return fmt.Errorf("echo with auth only: expected error, got nil")
	}
	st, ok = status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		return fmt.Errorf("echo with auth only: expected UNAUTHENTICATED, got %v", err)
	}
	if st.Message() != expectedMsgMissingSessionID {
		return fmt.Errorf("echo with auth only: expected %q, got %q", expectedMsgMissingSessionID, st.Message())
	}
	return nil
}

func case4InvalidToken(ctx context.Context, client pb.MyServiceAPIClient) error {
	md := metadata.Pairs(
		"authorization", "invalid_token_format",
		"session-id", sessionIDPrefix+"-case4-"+time.Now().Format("20060102150405"),
	)
	ctxEcho := metadata.NewOutgoingContext(ctx, md)
	_, err := client.MyServiceEcho(ctxEcho, &pb.EchoRequest{Value: echoValue})
	if err == nil {
		return fmt.Errorf("echo with invalid token: expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		return fmt.Errorf("echo with invalid token: expected UNAUTHENTICATED, got %v", err)
	}
	if st.Message() != expectedMsgMissingOrInvalidToken {
		return fmt.Errorf("echo with invalid token: expected %q, got %q", expectedMsgMissingOrInvalidToken, st.Message())
	}
	return nil
}
