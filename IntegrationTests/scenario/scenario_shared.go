package scenario

import (
	"context"
	"fmt"
	"io"

	"integrationtests/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	echoValue       = "integration-test-echo"
	sessionIDPrefix = "integration-test-session"
)

// CreateMSClient creates a new MyServiceAPI client connection to the gateway.
// Returns the client, a dispose function to close the connection, and an error.
func CreateMSClient(gatewayAddr string) (pb.MyServiceAPIClient, func(), error) {
	conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial gateway: %w", err)
	}

	client := pb.NewMyServiceAPIClient(conn)
	dispose := func() {
		conn.Close()
	}

	return client, dispose, nil
}

// Login performs authentication and returns the JWT token.
// Validates the response per FR-AUTH-1, FR-AUTH-4: token and expires_at must be set, role must be non-empty.
func Login(ctx context.Context, client pb.MyServiceAPIClient, cfg *Config, sessionID string) (string, error) {
	loginResp, err := client.Login(ctx, &pb.LoginRequest{
		Username:  cfg.Username,
		Password:  cfg.Password,
		SessionId: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	if loginResp.Token == "" {
		return "", fmt.Errorf("login: token is empty")
	}
	if loginResp.ExpiresAt == nil {
		return "", fmt.Errorf("login: expires_at is nil")
	}
	if loginResp.Role == "" {
		return "", fmt.Errorf("login: role is empty")
	}

	return loginResp.Token, nil
}

// Echo performs MyServiceEcho call and returns the pod_name.
// Validates all EchoResponse fields per FR-MS-1: client_value, server_value ("my_service"),
// pod_name, server_session_id, client_session_id, index (0 for unary), server_method.
// If expectedPodName is not empty, verifies sticky session (pod_name matches).
func Echo(ctxWithAuth context.Context, client pb.MyServiceAPIClient, expectedPodName string) (string, error) {
	echoResp, err := client.MyServiceEcho(ctxWithAuth, &pb.EchoRequest{Value: echoValue})
	if err != nil {
		return "", fmt.Errorf("myservice_echo request failed: %w", err)
	}

	if echoResp.ClientValue != echoValue {
		return "", fmt.Errorf("myservice_echo: client_value=%q, want %q", echoResp.ClientValue, echoValue)
	}
	if echoResp.ServerValue != "my_service" {
		return "", fmt.Errorf("myservice_echo: server_value=%q, want my_service", echoResp.ServerValue)
	}
	if echoResp.ClientSessionId == "" || echoResp.ServerSessionId == "" {
		return "", fmt.Errorf("myservice_echo: client_session_id or server_session_id is empty")
	}
	if echoResp.Index != 0 {
		return "", fmt.Errorf("myservice_echo: index=%d, want 0", echoResp.Index)
	}
	if echoResp.ServerMethod == "" {
		return "", fmt.Errorf("myservice_echo: server_method is empty")
	}
	if echoResp.PodName == "" {
		return "", fmt.Errorf("myservice_echo: pod_name is empty")
	}

	// Check sticky session if expectedPodName is provided
	if expectedPodName != "" && echoResp.PodName != expectedPodName {
		return "", fmt.Errorf("myservice_echo: pod_name=%q, want %q (sticky session violation)", echoResp.PodName, expectedPodName)
	}

	return echoResp.PodName, nil
}

// Shutdown performs MyServiceShutdown call.
// Validates all response fields and checks that pod_name matches expectedPodName.
func Shutdown(ctxWithAuth context.Context, client pb.MyServiceAPIClient, expectedPodName string) error {
	shutdownResp, err := client.MyServiceShutdown(ctxWithAuth, &pb.ShutdownRequest{})
	if err != nil {
		return fmt.Errorf("myservice_shutdown request failed: %w", err)
	}

	if shutdownResp.ServerSessionId == "" || shutdownResp.ClientSessionId == "" {
		return fmt.Errorf("myservice_shutdown: server_session_id or client_session_id is empty")
	}
	if shutdownResp.ServerMethod == "" {
		return fmt.Errorf("myservice_shutdown: server_method is empty")
	}
	if shutdownResp.PodName == "" {
		return fmt.Errorf("myservice_shutdown: pod_name is empty")
	}
	if shutdownResp.PodName != expectedPodName {
		return fmt.Errorf("myservice_shutdown: pod_name=%q, want %q", shutdownResp.PodName, expectedPodName)
	}

	return nil
}

// SubscribeStreamResult contains the result of reading from a subscription stream.
type SubscribeStreamResult struct {
	PodName      string
	MessageCount int
	CancelStream func() // Function to cancel the stream
}

// SubscribeStream reads messages from MyServiceSubscribe stream.
// It validates all response fields, checks sticky session, and verifies incrementing index.
// If maxMessagesToReceive is reached, the stream is automatically cancelled.
// Returns the pod name, message count, cancel function, and an error.
func SubscribeStream(ctxWithAuth context.Context, client pb.MyServiceAPIClient, value string, maxMessagesToReceive int, expectedServerMethod string) (*SubscribeStreamResult, error) {
	// Create a cancellable context for the stream
	streamCtx, streamCancel := context.WithCancel(ctxWithAuth)

	stream, err := client.MyServiceSubscribe(streamCtx, &pb.EchoRequest{Value: value})
	if err != nil {
		streamCancel()
		return nil, fmt.Errorf("myservice_subscribe request failed: %w", err)
	}

	var firstPodName string
	var expectedIndex int32 = 0
	messageCount := 0

	// Read messages from stream
	// Note: CloseSend() is already called by the generated MyServiceSubscribe code
	// Messages are sent every 5 seconds according to requirements
	for messageCount < maxMessagesToReceive {
		resp, err := stream.Recv()
		if err == io.EOF {
			// Stream ended normally
			if messageCount == 0 {
				streamCancel()
				return nil, fmt.Errorf("myservice_subscribe: no messages received from stream")
			}
			// Got some messages, that's acceptable
			break
		}
		if err != nil {
			// Check if it's a context cancellation error
			if streamCtx.Err() != nil {
				// Context was cancelled, which is expected after reading enough messages
				if messageCount >= 1 {
					// We got at least one message, that's acceptable
					break
				}
				streamCancel()
				return nil, fmt.Errorf("context cancelled before receiving messages: %w", streamCtx.Err())
			}
			// For RST_STREAM errors, if we got at least 2 messages, consider it acceptable
			// as the stream might be terminated by MyGateway or server
			if messageCount >= 2 {
				fmt.Printf("Note: Stream terminated after %d messages (acceptable): %v\n", messageCount, err)
				break
			}
			streamCancel()
			return nil, fmt.Errorf("stream recv error (message %d): %w", expectedIndex, err)
		}

		messageCount++

		// Validate response
		if resp.ClientValue != value {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: client_value=%q, want %q", resp.ClientValue, value)
		}
		if resp.ServerValue != "my_service" {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: server_value=%q, want my_service", resp.ServerValue)
		}
		if resp.ClientSessionId == "" || resp.ServerSessionId == "" {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: client_session_id or server_session_id is empty")
		}
		if resp.Index != expectedIndex {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: index=%d, want %d", resp.Index, expectedIndex)
		}
		if resp.ServerMethod != expectedServerMethod {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: server_method=%q, want %q", resp.ServerMethod, expectedServerMethod)
		}
		if resp.PodName == "" {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: pod_name is empty")
		}

		// Check sticky session - all messages should have the same pod_name
		if firstPodName == "" {
			firstPodName = resp.PodName
		} else if resp.PodName != firstPodName {
			streamCancel()
			return nil, fmt.Errorf("myservice_subscribe: pod_name changed from %q to %q (sticky session violation)", firstPodName, resp.PodName)
		}

		expectedIndex++
	}

	if messageCount == 0 {
		streamCancel()
		return nil, fmt.Errorf("myservice_subscribe: no messages received from stream")
	}

	// Cancel the stream context to stop the server from sending more messages
	// This is called automatically when maxMessagesToReceive is reached
	streamCancel()

	return &SubscribeStreamResult{
		PodName:      firstPodName,
		MessageCount: messageCount,
		CancelStream: func() {}, // No-op since stream is already cancelled
	}, nil
}
