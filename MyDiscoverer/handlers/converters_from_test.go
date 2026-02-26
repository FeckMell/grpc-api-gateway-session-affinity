package handlers

import (
	"testing"
	"time"

	"mydiscoverer/domain"
	"mydiscoverer/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromRegisterRequest(t *testing.T) {
	ts := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		request       RegisterRequest
		expected      domain.Instance
		expectedError string
	}{
		{
			name: "valid",
			request: RegisterRequest{
				InstanceId:  "inst-1",
				ServiceType: "grpc",
				Ipv4:        "127.0.0.1",
				Port:        9000,
				Timestamp:   ts,
				TtlMs:       300000,
			},
			expected: domain.Instance{
				InstanceID:  "inst-1",
				ServiceType: "grpc",
				Ipv4:        "127.0.0.1",
				Port:        9000,
				Timestamp:   ts,
				TTLMs:       300000,
			},
		},
		{
			name: "empty instance_id",
			request: RegisterRequest{
				InstanceId:  "",
				ServiceType: "grpc",
				Ipv4:        "127.0.0.1",
				Port:        9000,
				Timestamp:   ts,
				TtlMs:       300000,
			},
			expectedError: "instance_id is required",
		},
		{
			name: "empty service_type",
			request: RegisterRequest{
				InstanceId:  "inst-1",
				ServiceType: "",
				Ipv4:        "127.0.0.1",
				Port:        9000,
				Timestamp:   ts,
				TtlMs:       300000,
			},
			expectedError: "service_type is required",
		},
		{
			name: "empty ipv4",
			request: RegisterRequest{
				InstanceId:  "inst-1",
				ServiceType: "grpc",
				Ipv4:        "",
				Port:        9000,
				Timestamp:   ts,
				TtlMs:       300000,
			},
			expectedError: "ipv4 is required",
		},
		{
			name: "port zero",
			request: RegisterRequest{
				InstanceId:  "inst-1",
				ServiceType: "grpc",
				Ipv4:        "127.0.0.1",
				Port:        0,
				Timestamp:   ts,
				TtlMs:       300000,
			},
			expectedError: "port is required",
		},
		{
			name: "ttl_ms zero",
			request: RegisterRequest{
				InstanceId:  "inst-1",
				ServiceType: "grpc",
				Ipv4:        "127.0.0.1",
				Port:        9000,
				Timestamp:   ts,
				TtlMs:       0,
			},
			expectedError: "ttl_ms is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromRegisterRequest(tt.request)
			if tt.expectedError != "" {
				require.Error(t, err)
				myErr := service.ToMyError(err)
				require.NotNil(t, myErr)
				assert.Equal(t, tt.expectedError, myErr.Message)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
