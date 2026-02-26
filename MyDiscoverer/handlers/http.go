// Package handlers contains http handlers for mydiscoverer.
//
//go:generate oapi-codegen -config openapi-api.config.yaml ../api/my-discoverer.openapi.yaml
//go:generate oapi-codegen -config openapi-types.config.yaml ../api/my-discoverer.openapi.yaml
package handlers

import (
	"fmt"
	"net/http"

	"mydiscoverer/domain"
	"mydiscoverer/interfaces"
	"mydiscoverer/service"

	"github.com/go-kit/log"
	"github.com/labstack/echo/v4"
)

// HTTPServer implements ServerInterface generated from OpenAPI spec.
type HTTPServer struct {
	cache  interfaces.Cache[domain.Instance]
	logger log.Logger
}

// NewHTTPServer creates a new HTTPServer.
func NewHTTPServer(cache interfaces.Cache[domain.Instance], logger log.Logger) *HTTPServer {
	logger = log.WithPrefix(logger, "component", "HTTPServer")
	return &HTTPServer{
		cache:  cache,
		logger: logger,
	}
}

// RegisterInstance (POST /v1/register) writes request body to Redis. Returns 200 on success, 400 on parse/validation error, 500 on Redis error.
func (h *HTTPServer) RegisterInstance(ectx echo.Context) error {
	var req RegisterRequest
	if err := ectx.Bind(&req); err != nil {
		return service.NewBadParameterError("invalid request body", err)
	}

	instance, err := fromRegisterRequest(req)
	if err != nil {
		return fmt.Errorf("registerInstance failed to convert request to instance, err: %w", err)
	}

	ctx := ectx.Request().Context()
	if err := h.cache.WriteValue(ctx, req.InstanceId, instance, req.TtlMs); err != nil {
		return fmt.Errorf("registerInstance failed to write instance to cache, err: %w", err)
	}

	return ectx.NoContent(http.StatusOK)
}

// UnregisterInstance (POST /v1/unregister/{instance_id}) removes instance from Redis.
func (h *HTTPServer) UnregisterInstance(ectx echo.Context, instanceId string) error {
	ctx := ectx.Request().Context()
	if err := h.cache.DeleteValue(ctx, instanceId); err != nil {
		return fmt.Errorf("unregisterInstance failed to delete instance from cache, err: %w", err)
	}

	return ectx.NoContent(http.StatusOK)
}

// GetInstances (GET /v1/instances) reads all values from cache and returns instances.
func (h *HTTPServer) GetInstances(ectx echo.Context) error {
	ctx := ectx.Request().Context()
	instances, err := h.cache.ListAllValues(ctx)
	if err != nil {
		return fmt.Errorf("getInstances failed to list all instances from cache, err: %w", err)
	}

	return ectx.JSON(http.StatusOK, toInstancesResponse(instances))
}
