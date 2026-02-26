// Package main is the entry point for the MyGateway generic gRPC proxy. It loads configuration
// (env + YAML), builds the route matcher (service.NewRouteMatcherGeneric), static client connections and dynamic
// ConnectionPools (DiscovererHTTP + service.NewConnectionPool per dynamic cluster), the cluster resolver
// (service.NewConnectionResolverGeneric), the time provider and JWT validator, the header chain
// (helpers.ConfigurableAuthProcessor), and the transparent proxy (service.NewTransparentProxy). The gRPC server
// uses UnknownServiceHandler(proxy.Handler) so all RPCs are proxied. It listens on GRPCPort and
// on SIGINT/SIGTERM performs GracefulStop with a 5s timeout, then Stop if needed.
package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mygateway/adapters"
	"mygateway/domain"
	"mygateway/helpers"
	"mygateway/interfaces"
	"mygateway/service"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// main is the MyGateway entry point: loads config (LoadConfig), builds route matcher, static connections and dynamic pools (DiscovererHTTP + NewConnectionPool per cluster), resolver (NewConnectionResolverGeneric), time provider and JWT validator, header chain (ConfigurableAuthProcessor) and transparent proxy (NewTransparentProxy). Registers UnknownServiceHandler(proxy.Handler) and stream interceptor for error mapping. Listens on GRPCPort; on SIGINT/SIGTERM performs GracefulStop (5s timeout), then Stop if needed.
//
// Parameters and return: none (exits via os.Exit(1) on config/startup error).
//
// Called when the binary is started.
func main() {
	logger := log.NewLogfmtLogger(os.Stderr)
	cfg, err := LoadConfig()
	if err != nil {
		level.Error(logger).Log("msg", "failed to load configuration", "err", err)
		os.Exit(1)
	}
	pathRouter, routeErr := service.NewRouteMatcherGeneric(cfg.Routes)
	if routeErr != nil {
		level.Error(logger).Log("msg", "invalid route config", "err", routeErr)
		os.Exit(1)
	}
	staticConns := map[domain.ClusterID]*grpc.ClientConn{}
	dynamicPools := map[domain.ClusterID]interfaces.ConnectionPool{}
	for clusterID, cluster := range cfg.Clusters {
		switch cluster.Type {
		case domain.ClusterTypeStatic:
			conn, dialErr := grpc.NewClient(cluster.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if dialErr != nil {
				level.Error(logger).Log("msg", "dial static cluster", "cluster", clusterID, "err", dialErr)
				os.Exit(1)
			}
			staticConns[clusterID] = conn
		case domain.ClusterTypeDynamic:
			discoverer := adapters.DiscovererHTTP(cluster.DiscovererURL, &http.Client{Timeout: 10 * time.Second})
			factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
				addr := net.JoinHostPort(inst.Ipv4, strconv.Itoa(inst.Port))
				return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			}
			dynamicPools[clusterID] = service.NewConnectionPool(discoverer, factory, cluster.DiscovererInterval, logger)
		default:
			level.Error(logger).Log("msg", "unknown cluster type", "cluster", clusterID, "type", cluster.Type)
			os.Exit(1)
		}
	}
	clusterResolver := service.NewConnectionResolverGeneric(staticConns, dynamicPools)
	defer clusterResolver.Close()

	dynamicClusterIDs := make(map[domain.ClusterID]struct{})
	for clusterID, cluster := range cfg.Clusters {
		if cluster.Type == domain.ClusterTypeDynamic {
			dynamicClusterIDs[clusterID] = struct{}{}
		}
	}

	timeProvider := service.NewTimeProvider(func() time.Time { return time.Now().UTC() })
	jwtService := service.NewJWTValidator(cfg.JWTSecret, timeProvider)
	headerChain := helpers.NewHeaderProcessorChain(
		helpers.NewConfigurableAuthProcessor(jwtService, cfg.Routes.Routes),
	)
	transparentProxy := service.NewTransparentProxy(pathRouter, clusterResolver, headerChain, logger, cfg.RetryCount, cfg.RetryTimeout, dynamicClusterIDs)
	srv := grpc.NewServer(
		grpc.ChainStreamInterceptor(service.GatewayErrorToGRPCStreamInterceptor(logger)),
		grpc.UnknownServiceHandler(transparentProxy.Handler),
	)

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.GRPCPort))
	if err != nil {
		level.Error(logger).Log("msg", "listen", "err", err)
		os.Exit(1)
	}
	defer lis.Close()

	level.Info(logger).Log("msg", "starting MyGateway generic proxy", "port", cfg.GRPCPort)
	go func() {
		if err := srv.Serve(lis); err != nil {
			level.Error(logger).Log("msg", "serve", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	level.Info(logger).Log("msg", "shutting down")
	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		srv.Stop()
	}
}
