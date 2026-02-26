package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mydiscoverer/domain"
	"mydiscoverer/interfaces/mock"
	"mydiscoverer/service"

	"github.com/go-kit/log"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerHandlers(e *echo.Echo, server ServerInterface) {
	RegisterHandlers(e, server)
	service.RegisterErrorHandler(e, log.NewNopLogger())
}

func TestHTTPServer_RegisterInstance(t *testing.T) {
	validBody := `{"instance_id":"inst-1","service_type":"grpc","ipv4":"127.0.0.1","port":9000,"timestamp":"2026-02-19T12:00:00Z","ttl_ms":300000}`

	tests := []struct {
		name           string
		body           string
		cache          *mock.CacheMock[domain.Instance]
		expectedStatus int
		emptyBody      bool
	}{
		{
			name: "ok",
			body: validBody,
			cache: &mock.CacheMock[domain.Instance]{
				WriteValueFunc: func(ctx context.Context, key string, item domain.Instance, ttlMs int) error {
					assert.Equal(t, "inst-1", key)
					assert.Equal(t, "inst-1", item.InstanceID)
					assert.Equal(t, "127.0.0.1", item.Ipv4)
					assert.Equal(t, 9000, item.Port)
					assert.Equal(t, 300000, ttlMs)
					return nil
				},
			},
			expectedStatus: http.StatusOK,
			emptyBody:      true,
		},
		{
			name:           "400 invalid JSON",
			body:           `{invalid`,
			cache:          &mock.CacheMock[domain.Instance]{},
			expectedStatus: http.StatusBadRequest,
			emptyBody:      false,
		},
		{
			name:           "400 missing instance_id",
			body:           `{"service_type":"grpc","ipv4":"127.0.0.1","port":9000,"timestamp":"2026-02-19T12:00:00Z","ttl_ms":300000}`,
			cache:          &mock.CacheMock[domain.Instance]{},
			expectedStatus: http.StatusBadRequest,
			emptyBody:      false,
		},
		{
			name: "500 WriteValue error",
			body: validBody,
			cache: &mock.CacheMock[domain.Instance]{
				WriteValueFunc: func(ctx context.Context, key string, item domain.Instance, ttlMs int) error {
					return assert.AnError
				},
			},
			expectedStatus: http.StatusInternalServerError,
			emptyBody:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			registerHandlers(e, NewHTTPServer(tt.cache, log.NewNopLogger()))
			req := httptest.NewRequest(http.MethodPost, "/v1/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.emptyBody {
				assert.Empty(t, rec.Body.Bytes())
			} else {
				// 400/500 return error JSON
				var errBody struct {
					Error *struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
				require.NotNil(t, errBody.Error)
				assert.NotEmpty(t, errBody.Error.Code)
				assert.NotEmpty(t, errBody.Error.Message)
			}
		})
	}
}

func TestHTTPServer_UnregisterInstance(t *testing.T) {
	tests := []struct {
		name           string
		instanceId     string
		cache          *mock.CacheMock[domain.Instance]
		expectedStatus int
	}{
		{
			name:       "ok",
			instanceId: "inst-1",
			cache: &mock.CacheMock[domain.Instance]{
				DeleteValueFunc: func(ctx context.Context, key string) error {
					assert.Equal(t, "inst-1", key)
					return nil
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "500 DeleteValue error",
			instanceId: "inst-err",
			cache: &mock.CacheMock[domain.Instance]{
				DeleteValueFunc: func(ctx context.Context, key string) error {
					return assert.AnError
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			registerHandlers(e, NewHTTPServer(tt.cache, log.NewNopLogger()))
			req := httptest.NewRequest(http.MethodPost, "/v1/unregister/"+tt.instanceId, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedStatus == http.StatusOK {
				assert.Empty(t, rec.Body.Bytes())
			}
		})
	}
}

func TestHTTPServer_GetInstances(t *testing.T) {
	tests := []struct {
		name           string
		cache          *mock.CacheMock[domain.Instance]
		expectedStatus int
		wantInstances  int
	}{
		{
			name: "ok empty",
			cache: &mock.CacheMock[domain.Instance]{
				ListAllValuesFunc: func(ctx context.Context) ([]domain.Instance, error) {
					return []domain.Instance{}, nil
				},
			},
			expectedStatus: http.StatusOK,
			wantInstances:  0,
		},
		{
			name: "ok one instance",
			cache: &mock.CacheMock[domain.Instance]{
				ListAllValuesFunc: func(ctx context.Context) ([]domain.Instance, error) {
					return []domain.Instance{{
						InstanceID: "inst-1",
						Ipv4:       "10.0.0.1",
						Port:       8080,
						Timestamp:  time.Now(),
					}}, nil
				},
			},
			expectedStatus: http.StatusOK,
			wantInstances:  1,
		},
		{
			name: "cache ListAllValues returns EntityNotFoundError then 200 with empty instances",
			cache: &mock.CacheMock[domain.Instance]{
				ListAllValuesFunc: func(ctx context.Context) ([]domain.Instance, error) {
					return []domain.Instance{}, nil
				},
			},
			expectedStatus: http.StatusOK,
			wantInstances:  0,
		},
		{
			name: "500 ListAllValues error",
			cache: &mock.CacheMock[domain.Instance]{
				ListAllValuesFunc: func(ctx context.Context) ([]domain.Instance, error) {
					return nil, assert.AnError
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "500 ListAllValues returns custom error",
			cache: &mock.CacheMock[domain.Instance]{
				ListAllValuesFunc: func(ctx context.Context) ([]domain.Instance, error) {
					return nil, assert.AnError
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			registerHandlers(e, NewHTTPServer(tt.cache, log.NewNopLogger()))
			req := httptest.NewRequest(http.MethodGet, "/v1/instances", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedStatus == http.StatusOK {
				var resp InstancesResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Len(t, resp.Instances, tt.wantInstances)
			}
		})
	}
}
