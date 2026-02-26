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

const (
	msgServerSessionNotSet = "Server session is not set"
	msgClientSessionPrefix = "Client session mismatch:"
)

const scenarioMyServiceShutdownErrors = "myservice_shutdown_errors"

func init() {
	Register(scenarioMyServiceShutdownErrors, runMyServiceShutdownErrors)
}

// runMyServiceShutdownErrors verifies FR-MS-7: MyServiceShutdown error cases.
// 1. MyServiceShutdown without prior Echo/Subscribe -> PERMISSION_DENIED "Server session is not set".
// 2. MyServiceShutdown with session-id in metadata not matching server session -> PERMISSION_DENIED "Client session mismatch: ...".
func runMyServiceShutdownErrors(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	// 1. Server session is not set: Login, then MyServiceShutdown without ever calling Echo/Subscribe.
	sessionID1 := sessionIDPrefix + "-shutdown1-" + time.Now().Format("20060102150405")
	token, err := Login(ctx, client, cfg, sessionID1)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	md1 := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID1,
	)
	ctx1 := metadata.NewOutgoingContext(ctx, md1)

	_, err = client.MyServiceShutdown(ctx1, &pb.ShutdownRequest{})
	if err == nil {
		return fmt.Errorf("expected error for MyServiceShutdown without session, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status, got: %w", err)
	}
	if st.Code() != codes.PermissionDenied {
		return fmt.Errorf("expected PERMISSION_DENIED, got %v: %v", st.Code(), st.Message())
	}
	if st.Message() != msgServerSessionNotSet {
		return fmt.Errorf("expected message %q, got %q", msgServerSessionNotSet, st.Message())
	}

	// 2. Client session mismatch: two clients; first establishes session on some instance,
	// second calls MyServiceShutdown and may hit the same instance (mismatch) or another (server session not set).
	client2, dispose2, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client2: %w", err)
	}
	defer dispose2()

	sessionID2 := sessionIDPrefix + "-shutdown2-" + time.Now().Format("20060102150405")
	sessionID3 := sessionIDPrefix + "-shutdown3-" + time.Now().Format("20060102150405")
	token2, err := Login(ctx, client, cfg, sessionID2)
	if err != nil {
		return fmt.Errorf("login (client2): %w", err)
	}
	token3, err := Login(ctx, client2, cfg, sessionID3)
	if err != nil {
		return fmt.Errorf("login (client3): %w", err)
	}
	md2 := metadata.Pairs("authorization", token2, "session-id", sessionID2)
	ctx2 := metadata.NewOutgoingContext(ctx, md2)
	md3 := metadata.Pairs("authorization", token3, "session-id", sessionID3)
	ctx3 := metadata.NewOutgoingContext(ctx, md3)

	_, err = Echo(ctx2, client, "")
	if err != nil {
		// With a single instance, gateway returns "all instances are busy" for sessionID2 (instance already bound to sessionID1). Skip step 2.
		if st, ok := status.FromError(err); ok && st.Code() == codes.ResourceExhausted && strings.Contains(st.Message(), "all instances are busy") {
			return nil // FR-MS-7 step 1 verified; step 2 requires 2+ instances, skip
		}
		return fmt.Errorf("echo (client2) to establish session: %w", err)
	}
	// Client3 calls MyServiceShutdown. If routed to same instance as client2 -> "Client session mismatch".
	// If routed to another instance -> "Server session is not set". Both are valid FR-MS-7 responses.
	_, err = client2.MyServiceShutdown(ctx3, &pb.ShutdownRequest{})
	if err == nil {
		return fmt.Errorf("expected error (mismatch or server session not set), got nil")
	}
	st, ok = status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status, got: %w", err)
	}
	if st.Code() != codes.PermissionDenied {
		return fmt.Errorf("expected PERMISSION_DENIED, got %v: %v", st.Code(), st.Message())
	}
	msg := st.Message()
	if msg != msgServerSessionNotSet && !strings.HasPrefix(msg, msgClientSessionPrefix) {
		return fmt.Errorf("expected %q or message starting with %q, got %q", msgServerSessionNotSet, msgClientSessionPrefix, msg)
	}
	if strings.HasPrefix(msg, msgClientSessionPrefix) {
		if !strings.Contains(msg, sessionID3) {
			return fmt.Errorf("Client session mismatch message should contain client session id %q: %q", sessionID3, msg)
		}
	}

	return nil
}
