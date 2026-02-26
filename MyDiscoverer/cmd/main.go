package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mydiscoverer/adapters/myredis"
	"mydiscoverer/domain"
	"mydiscoverer/handlers"
	"mydiscoverer/interfaces"
	"mydiscoverer/service"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/labstack/echo/v4"
)

func main() {
	// Initialize logger
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.WithPrefix(logger, "ts", log.DefaultTimestampUTC)
	logger = log.WithPrefix(logger, "caller", log.DefaultCaller)

	level.Info(logger).Log("msg", "Starting MyDiscoverer service")

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to load configuration", "err", err)
		os.Exit(1)
	}
	level.Info(logger).Log(
		"msg", "Configuration loaded",
		"service_port_http", config.HTTPPort,
		"redis_addr", config.Redis.Addr,
	)

	var cache interfaces.Cache[domain.Instance]
	{
		redisClient, err := myredis.NewRedisUniversalClient(config.Redis.Addr)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to create Redis client", "err", err)
			os.Exit(1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			level.Error(logger).Log("msg", "Failed to connect to Redis", "err", err)
			os.Exit(1)
		}
		level.Info(logger).Log("msg", "Connected to Redis")

		marshal := func(i domain.Instance) ([]byte, error) { return json.Marshal(i) }
		unmarshal := func(b []byte) (domain.Instance, error) {
			var i domain.Instance
			err := json.Unmarshal(b, &i)
			return i, err
		}
		cache = myredis.NewCache[domain.Instance](redisClient, "instance", marshal, unmarshal)
	}

	// Create HTTPServer
	var httpServer handlers.ServerInterface
	{
		httpServer = handlers.NewHTTPServer(cache, logger)
	}

	// Create HTTP server (Echo)
	var e *echo.Echo
	{
		e = echo.New()
		e.HideBanner = true
		service.RegisterErrorHandler(e, logger)
		handlers.RegisterHandlers(e, httpServer)
	}

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		addr := fmt.Sprintf(":%d", config.HTTPPort)
		level.Info(logger).Log("msg", "Starting HTTP server", "addr", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			level.Error(logger).Log("msg", "HTTP server error", "err", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	level.Info(logger).Log("msg", "Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		level.Error(logger).Log("msg", "Error during server shutdown", "err", err)
	}

	level.Info(logger).Log("msg", "Server stopped")
}
