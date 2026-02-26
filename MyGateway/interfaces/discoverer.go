package interfaces

import "mygateway/domain"

// Discoverer provides the list of backend instances for dynamic clusters and supports
// unregistering an instance on failure.
//
// GetInstances returns the current set of instances (e.g. from HTTP GET /v1/instances).
// UnregisterInstance notifies the discoverer to remove an instance from the registry
// (e.g. POST /v1/unregister/{instance_id}), typically called when a backend connection
// fails so the instance can be removed or marked unhealthy.
//
// Implemented by adapters.DiscovererHTTP. Called from service.connectionPool in refresh (GetInstances)
// and in OnBackendFailure (UnregisterInstance).
//
//go:generate moq -stub -out mock/discoverer.go -pkg mock . Discoverer
type Discoverer interface {
	// GetInstances returns the current list of backend instances for the dynamic cluster (e.g. from HTTP GET /v1/instances).
	// Parameters: none.
	// Returns: ([]ServiceInstance, nil) on success; (nil, error) on network or response parse error. Empty list is valid (e.g. 404 from discoverer).
	// Called from service.connectionPool.refresh (background refresh) and at pool startup in NewConnectionPool.
	GetInstances() ([]domain.ServiceInstance, error)

	// UnregisterInstance notifies the discoverer to remove the instance from the registry (e.g. POST /v1/unregister/{id}) so the instance can be marked unavailable.
	// Parameter instanceID — identifier of the instance that failed; empty string is allowed but the call may have no effect on the discoverer side.
	// Returns: nil on success (e.g. 200); error on request error or non-200 response.
	// Called from service.connectionPool.OnBackendFailure after closing the connection to this instance.
	UnregisterInstance(instanceID string) error
}
