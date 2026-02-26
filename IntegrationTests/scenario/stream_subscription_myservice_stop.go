package scenario

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"integrationtests/docker"
	"integrationtests/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const scenarioStreamSubscriptionMyServiceStop = "stream_subscription_myservice_stop"

const myserviceServiceNameForStop = "myservice"

func init() {
	Register(scenarioStreamSubscriptionMyServiceStop, runStreamSubscriptionMyServiceStop)
}

// runStreamSubscriptionMyServiceStop verifies transfer of an active MyServiceSubscribe stream:
// after first message is received, one MyService container is stopped and stream must
// continue from another backend without UNAVAILABLE on client side.
func runStreamSubscriptionMyServiceStop(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	if cfg.ComposePath == "" {
		return fmt.Errorf("compose path is required for this scenario (set --compose-file or COMPOSE_FILE)")
	}
	absPath, err := filepath.Abs(cfg.ComposePath)
	if err != nil {
		return fmt.Errorf("resolve compose path: %w", err)
	}
	workDir := filepath.Dir(absPath)

	client, dispose, err := CreateMSClient(cfg.GatewayAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer dispose()

	sessionID := sessionIDPrefix + "-sub-stop-" + time.Now().Format("20060102150405")
	token, err := Login(ctx, client, cfg, sessionID)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	md := metadata.Pairs(
		"authorization", token,
		"session-id", sessionID,
	)
	ctxWithAuth := metadata.NewOutgoingContext(ctx, md)

	_, err = Echo(ctxWithAuth, client, "")
	if err != nil {
		return fmt.Errorf("echo: %w", err)
	}

	streamCtx, streamCancel := context.WithCancel(ctxWithAuth)
	defer streamCancel()

	stream, err := client.MyServiceSubscribe(streamCtx, &pb.EchoRequest{Value: echoValue})
	if err != nil {
		return fmt.Errorf("myservice_subscribe request failed: %w", err)
	}

	// First message: validate and ensure stream is established
	firstResp, err := stream.Recv()
	if err == io.EOF {
		return fmt.Errorf("myservice_subscribe: stream ended before first message")
	}
	if err != nil {
		return fmt.Errorf("myservice_subscribe first recv: %w", err)
	}
	if firstResp.ClientValue != echoValue {
		return fmt.Errorf("myservice_subscribe: client_value=%q, want %q", firstResp.ClientValue, echoValue)
	}
	if firstResp.ServerValue != "my_service" {
		return fmt.Errorf("myservice_subscribe: server_value=%q, want my_service", firstResp.ServerValue)
	}
	if firstResp.ClientSessionId == "" || firstResp.ServerSessionId == "" {
		return fmt.Errorf("myservice_subscribe: client_session_id or server_session_id is empty")
	}
	if firstResp.Index != 0 {
		return fmt.Errorf("myservice_subscribe: index=%d, want 0", firstResp.Index)
	}
	if firstResp.ServerMethod != "MyServiceSubscribe" {
		return fmt.Errorf("myservice_subscribe: server_method=%q, want MyServiceSubscribe", firstResp.ServerMethod)
	}
	if firstResp.PodName == "" {
		return fmt.Errorf("myservice_subscribe: pod_name is empty")
	}
	firstPodName := firstResp.PodName

	containers, err := docker.ServiceContainerIDs(workDir, myserviceServiceNameForStop)
	if err != nil {
		return fmt.Errorf("list myservice containers: %w", err)
	}
	if len(containers) < 2 {
		return fmt.Errorf("need at least 2 myservice containers for transfer scenario, got %d", len(containers))
	}
	var containerToStop string
	for _, id := range containers {
		// Match by hostname (Config.Hostname or short ID) or by container ID prefix (pod_name is often short ID)
		hostname, err := docker.ContainerHostname(id)
		if err != nil {
			return fmt.Errorf("container hostname %s: %w", id, err)
		}
		if hostname == firstPodName {
			containerToStop = id
			break
		}
		if id == firstPodName || strings.HasPrefix(id, firstPodName) {
			containerToStop = id
			break
		}
	}
	if containerToStop == "" {
		return fmt.Errorf("no myservice container with hostname/id %q (pod_name from first message)", firstPodName)
	}

	type recvResult struct {
		resp *pb.EchoResponse
		err  error
	}
	// Start Recv in a separate goroutine before stopping backend.
	recvDone := make(chan recvResult, 1)
	go func() {
		resp, recvErr := stream.Recv()
		recvDone <- recvResult{resp: resp, err: recvErr}
	}()

	// Stop well before backend sends second message (5s). Use kill so connection drops immediately.
	time.Sleep(1 * time.Second)

	if err := docker.KillContainer(containerToStop); err != nil {
		return fmt.Errorf("kill myservice container %s: %w", containerToStop, err)
	}
	defer func() {
		if startErr := docker.StartContainer(containerToStop); startErr != nil {
			fmt.Printf("Warning: failed to start myservice container %s after test: %v\n", containerToStop, startErr)
		}
	}()

	// After backend failure, stream should transfer and continue with next message.
	result := <-recvDone
	if result.err == io.EOF {
		return fmt.Errorf("myservice_subscribe: stream ended unexpectedly after backend stop")
	}
	if result.err != nil {
		st, ok := status.FromError(result.err)
		if ok && st.Code() == codes.Unavailable {
			return fmt.Errorf("stream transfer failed, got UNAVAILABLE: %s", st.Message())
		}
		return fmt.Errorf("myservice_subscribe recv after backend stop: %w", result.err)
	}
	if result.resp == nil {
		return fmt.Errorf("myservice_subscribe: empty response after backend stop")
	}
	if result.resp.ClientValue != echoValue {
		return fmt.Errorf("myservice_subscribe after transfer: client_value=%q, want %q", result.resp.ClientValue, echoValue)
	}
	if result.resp.ServerMethod != "MyServiceSubscribe" {
		return fmt.Errorf("myservice_subscribe after transfer: server_method=%q, want MyServiceSubscribe", result.resp.ServerMethod)
	}
	if result.resp.PodName == "" {
		return fmt.Errorf("myservice_subscribe after transfer: pod_name is empty")
	}
	if result.resp.PodName == firstPodName {
		return fmt.Errorf("myservice_subscribe after transfer: pod_name unchanged (%q), expected response from different MyService instance", firstPodName)
	}
	return nil
}
