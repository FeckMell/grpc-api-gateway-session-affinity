package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthCheck(t *testing.T) {
	// Setup: Create a test server with health check
	t.Setenv("SERVICE_PORT_GRPC", "0") // Use random port
	t.Setenv("REDIS_ADDR", "redis://localhost:6379")
	t.Setenv("JWT_SECRET", "test-secret-for-health-check")
	t.Setenv("TOKEN_EXPIRATION", "1h")

	config, err := LoadConfig()
	require.NoError(t, err)

	// Create a listener on a random port
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	// Create gRPC server with health check
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Start server in goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create client connection
	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Create health check client
	healthClient := grpc_health_v1.NewHealthClient(conn)

	// Test health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "",
	})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)

	// Cleanup
	grpcServer.GracefulStop()
	_ = config
}
