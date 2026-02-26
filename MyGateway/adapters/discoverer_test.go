package adapters

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mygateway/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscovererHTTP_Panics(t *testing.T) {
	t.Run("baseURL_empty", func(t *testing.T) {
		assert.PanicsWithValue(t, "adapters.discoverer.go: baseURL is required", func() {
			DiscovererHTTP("", &http.Client{})
		})
	})
	t.Run("client_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "adapters.discoverer.go: http client is required", func() {
			DiscovererHTTP("http://localhost:8080", nil)
		})
	})
}

func TestDiscovererHTTP_GetInstances(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           string
		wantInstances  []domain.ServiceInstance
		wantErr        bool
		wantErrContain string
	}{
		{
			name:       "success_openapi_shape",
			statusCode: http.StatusOK,
			body:       `{"instances":[{"instance_id":"i1","ipv4":"127.0.0.1","port":9000}]}`,
			wantInstances: []domain.ServiceInstance{
				{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9000, AssignedClientSessionID: ""},
			},
		},
		{
			name:          "success_empty_list",
			statusCode:    http.StatusOK,
			body:          `{"instances":[]}`,
			wantInstances: []domain.ServiceInstance{},
		},
		{
			name:       "success_extra_fields_ignored",
			statusCode: http.StatusOK,
			body:       `{"instances":[{"instance_id":"i2","ipv4":"10.0.0.1","port":9001,"resolved_address":"10.0.0.2","assigned_client_session_id":"sess-1"}]}`,
			wantInstances: []domain.ServiceInstance{
				{InstanceID: "i2", Ipv4: "10.0.0.1", Port: 9001, AssignedClientSessionID: ""},
			},
		},
		{
			name:           "non_200_returns_error",
			statusCode:     http.StatusInternalServerError,
			body:           `{}`,
			wantErr:        true,
			wantErrContain: "500",
		},
		{
			name:          "404_treated_as_empty_list",
			statusCode:    http.StatusNotFound,
			body:          `{}`,
			wantInstances: []domain.ServiceInstance{},
		},
		{
			name:           "invalid_json_returns_error",
			statusCode:     http.StatusOK,
			body:           `not json`,
			wantErr:        true,
			wantErrContain: "",
		},
		{
			name:           "empty_object_missing_instances_returns_error",
			statusCode:     http.StatusOK,
			body:           `{}`,
			wantErr:        true,
			wantErrContain: "missing instances",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod string
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			disc := DiscovererHTTP(server.URL, server.Client())
			got, err := disc.GetInstances()
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "GET", gotMethod)
			assert.Equal(t, "/v1/instances", gotPath)
			assert.Equal(t, tt.wantInstances, got)
		})
	}
}

func TestDiscovererHTTP_UnregisterInstance(t *testing.T) {
	tests := []struct {
		name           string
		instanceID     string
		statusCode     int
		wantErr        bool
		wantErrContain string
		wantPathSuffix string
	}{
		{
			name:           "success_200",
			instanceID:     "inst-1",
			statusCode:     http.StatusOK,
			wantPathSuffix: "/v1/unregister/inst-1",
		},
		{
			name:           "500_returns_error",
			instanceID:     "inst-2",
			statusCode:     http.StatusInternalServerError,
			wantErr:        true,
			wantErrContain: "500",
			wantPathSuffix: "/v1/unregister/inst-2",
		},
		{
			name:           "instance_id_path_escaped",
			instanceID:     "inst/1",
			statusCode:     http.StatusOK,
			wantPathSuffix: "/v1/unregister/inst%2F1", // RawPath; Path would be decoded to inst/1
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod string
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.RawPath
				if gotPath == "" {
					gotPath = r.URL.Path
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			disc := DiscovererHTTP(server.URL, server.Client())
			err := disc.UnregisterInstance(tt.instanceID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "POST", gotMethod)
			assert.Equal(t, tt.wantPathSuffix, gotPath)
		})
	}
}
