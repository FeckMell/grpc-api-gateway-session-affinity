package handlers

import (
	"testing"
	"time"

	"mydiscoverer/domain"

	"github.com/stretchr/testify/assert"
)

func TestToInstancesResponse(t *testing.T) {
	ts := time.Now()

	tests := []struct {
		name      string
		instances []domain.Instance
		wantLen   int
		wantFirst *InstanceInfo
	}{
		{
			name:      "nil",
			instances: nil,
			wantLen:   0,
		},
		{
			name:      "empty",
			instances: []domain.Instance{},
			wantLen:   0,
		},
		{
			name: "one",
			instances: []domain.Instance{{
				InstanceID: "inst-1",
				Ipv4:       "10.0.0.1",
				Port:       8080,
				Timestamp:  ts,
			}},
			wantLen: 1,
			wantFirst: &InstanceInfo{
				InstanceId: "inst-1",
				Ipv4:       "10.0.0.1",
				Port:       8080,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInstancesResponse(tt.instances)
			assert.Len(t, got.Instances, tt.wantLen)
			if tt.wantFirst != nil && len(got.Instances) > 0 {
				assert.Equal(t, *tt.wantFirst, got.Instances[0])
			}
		})
	}
}
