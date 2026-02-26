package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"mygateway/domain"
	"mygateway/helpers"
	"mygateway/interfaces"
)

// DiscovererHTTP creates an interfaces.Discoverer that talks to MyDiscoverer over HTTP: GET baseURL/v1/instances and POST baseURL/v1/unregister/{instance_id}. Panics on empty baseURL or nil client.
//
// Parameters: baseURL — discoverer base URL (e.g. http://mydiscoverer:8080), no trailing slash; client — HTTP client (timeout recommended; main uses 10s).
//
// Returns: interfaces.Discoverer (*discovererHTTP).
//
// Called from cmd/main for each dynamic cluster.
func DiscovererHTTP(baseURL string, client *http.Client) interfaces.Discoverer {
	return &discovererHTTP{
		baseURL: helpers.StrPanic(baseURL, "adapters.discoverer.go: baseURL is required"),
		client:  helpers.NilPanic(client, "adapters.discoverer.go: http client is required"),
	}
}

// discovererHTTP implements interfaces.Discoverer. Used by service.connectionPool to fetch the instance
// list (refresh) and to unregister an instance on backend failure. Holds baseURL and http.Client.
type discovererHTTP struct {
	baseURL string
	client  *http.Client
}

// instancesResponse is the JSON shape of GET /v1/instances response: { "instances": [ InstanceInfo ] }.
type instancesResponse struct {
	Instances []instanceInfo `json:"instances"`
}

// instanceInfo is one element of the instances array in the discoverer JSON (instance_id, ipv4, port).
type instanceInfo struct {
	InstanceID string `json:"instance_id"`
	Ipv4       string `json:"ipv4"`
	Port       int    `json:"port"`
}

// GetInstances performs GET baseURL/v1/instances with 5s timeout. On 404 (MyDiscoverer entity_not_found when no instances) returns empty slice; on 200 parses JSON and maps to domain.ServiceInstance (AssignedClientSessionID is not set by the adapter).
//
// Parameters: none.
//
// Returns: ([]domain.ServiceInstance, nil) on 200 (possibly empty slice) or 404 (empty slice); (nil, error) on other status, network error or JSON parse error (e.g. missing "instances" field).
//
// Called from service.connectionPool.refresh (on timer and at startup).
func (d *discovererHTTP) GetInstances() ([]domain.ServiceInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reqURL := d.baseURL + "/v1/instances"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// MyDiscoverer returns 404 when there are no instances (entity_not_found); treat as empty list.
		return []domain.ServiceInstance{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discoverer returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw instancesResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.Instances == nil {
		return nil, fmt.Errorf("discoverer response missing instances field")
	}
	out := make([]domain.ServiceInstance, 0, len(raw.Instances))
	for _, r := range raw.Instances {
		addr := r.Ipv4
		out = append(out, domain.ServiceInstance{
			InstanceID:              r.InstanceID,
			Ipv4:                    addr,
			Port:                    r.Port,
			AssignedClientSessionID: "",
		})
	}
	return out, nil
}

// UnregisterInstance performs POST baseURL/v1/unregister/{instance_id} with 5s timeout so the discoverer can remove or mark the instance as unavailable.
//
// Parameter instanceID — instance identifier to unregister; substituted in URL via url.PathEscape (special chars escaped).
//
// Returns: nil on 200; error on non-200 or request error (network, timeout).
//
// Called from service.connectionPool.OnBackendFailure after closing the connection to the instance.
func (d *discovererHTTP) UnregisterInstance(instanceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path := "/v1/unregister/" + url.PathEscape(instanceID)
	reqURL := d.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discoverer unregister returned %d", resp.StatusCode)
	}
	return nil
}
