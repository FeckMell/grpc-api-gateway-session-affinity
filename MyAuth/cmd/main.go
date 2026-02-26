package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"myauth/adapters/redis"
	"myauth/handlers"
	"myauth/interfaces"
	"myauth/service"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.WithPrefix(logger, "ts", log.DefaultTimestampUTC)
	logger = log.WithPrefix(logger, "caller", log.DefaultCaller)

	level.Info(logger).Log("msg", "Starting MyAuth service")

	config, err := LoadConfig()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to load configuration", "err", err)
		os.Exit(1)
	}

	level.Info(logger).Log(
		"msg", "Configuration loaded",
		"service_port_grpc", config.Port,
		"redis_addr", config.RedisAddr,
		"token_expiration", config.TokenExpiration,
	)

	now := func() time.Time {
		return time.Now().UTC()
	}

	var userStore interfaces.UserStore
	{
		redisClient, err := redis.NewRedisUniversalClient(config.RedisAddr)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to create Redis client", "err", err)
			os.Exit(1)
		}
		userStore = redis.NewUserStore(redisClient)
	}

	var (
		authService handlers.MyServiceAPIServer
		jwtService  interfaces.JwtService
	)
	{
		jwtService = handlers.NewJwtService(config.JWTSecret)
		authService = handlers.NewGrpcServer(
			userStore,
			jwtService,
			config.TokenExpiration,
			now,
			logger,
		)
	}

	var grpcServer *grpc.Server
	{
		errorCodeOption := grpc.ChainUnaryInterceptor(service.AuthErrorToGRPCInterceptor(logger))
		grpcServer = grpc.NewServer(errorCodeOption)
		handlers.RegisterMyServiceAPIServer(grpcServer, authService)

		// Register health check service
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
		grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

		reflection.Register(grpcServer)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		level.Error(logger).Log("msg", "Failed to listen", "err", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		level.Info(logger).Log("msg", "Starting gRPC server", "addr", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			level.Error(logger).Log("msg", "gRPC server error", "err", err)
		}
	}()

	<-quit
	level.Info(logger).Log("msg", "Shutting down...")

	grpcServer.GracefulStop()
	level.Info(logger).Log("msg", "Server stopped")
}
