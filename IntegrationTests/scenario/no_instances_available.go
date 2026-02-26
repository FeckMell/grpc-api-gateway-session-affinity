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

// Expected gateway error when all instances are busy (FR-MGW-5).
const expectedMsgAllInstancesBusy = "all instances are busy"

const scenarioNoInstancesAvailable = "no_instances_available"

func init() {
	Register(scenarioNoInstancesAvailable, runNoInstancesAvailable)
}

// runNoInstancesAvailable verifies FR-MGW-5 and FR-5: when all instances are busy,
// MyServiceEcho returns RESOURCE_EXHAUSTED with message "all instances are busy".
// 4 clients log in; 3 call Echo (occupying 3 instances); 4th client calls MyServiceEcho and must get the error.
func runNoInstancesAvailable(ctx context.Context, cfg *Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	const numOccupants = 3
	const extraClient = 1
	clients := make([]struct {
		client    pb.MyServiceAPIClient
		dispose   func()
		sessionID string
		token     string
	}, numOccupants+extraClient)

	for i := 0; i < numOccupants+extraClient; i++ {
		c, dispose, err := CreateMSClient(cfg.GatewayAddr)
		if err != nil {
			return fmt.Errorf("create client%d: %w", i+1, err)
		}
		clients[i].client = c
		clients[i].dispose = dispose
		defer dispose()
		clients[i].sessionID = fmt.Sprintf("%s-noinst-%d-%s", sessionIDPrefix, i+1, time.Now().Format("20060102150405"))
	}

	for i := 0; i < numOccupants+extraClient; i++ {
		token, err := Login(ctx, clients[i].client, cfg, clients[i].sessionID)
		if err != nil {
			return fmt.Errorf("login (client%d): %w", i+1, err)
		}
		clients[i].token = token
	}

	// Occupy all 3 instances (Echo, no Shutdown)
	for i := 0; i < numOccupants; i++ {
		md := metadata.Pairs("authorization", clients[i].token, "session-id", clients[i].sessionID)
		ctxAuth := metadata.NewOutgoingContext(ctx, md)
		_, err := Echo(ctxAuth, clients[i].client, "")
		if err != nil {
			return fmt.Errorf("echo (client%d) to occupy instance: %w", i+1, err)
		}
	}

	// 4th client: MyServiceEcho must return RESOURCE_EXHAUSTED "all instances are busy"
	idx := numOccupants
	md := metadata.Pairs("authorization", clients[idx].token, "session-id", clients[idx].sessionID)
	ctxAuth := metadata.NewOutgoingContext(ctx, md)
	_, err := clients[idx].client.MyServiceEcho(ctxAuth, &pb.EchoRequest{Value: echoValue})
	if err == nil {
		return fmt.Errorf("expected RESOURCE_EXHAUSTED when all instances busy, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("expected gRPC status, got: %w", err)
	}
	if st.Code() != codes.ResourceExhausted {
		return fmt.Errorf("expected RESOURCE_EXHAUSTED, got %v: %v", st.Code(), st.Message())
	}
	if st.Message() != expectedMsgAllInstancesBusy {
		return fmt.Errorf("expected message %q, got %q", expectedMsgAllInstancesBusy, st.Message())
	}

	return nil
}
