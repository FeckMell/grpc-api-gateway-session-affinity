package handlers

import (
	"mydiscoverer/domain"
)

// toInstancesResponse converts domain instances to API response.
func toInstancesResponse(instances []domain.Instance) InstancesResponse {
	out := make([]InstanceInfo, 0, len(instances))
	for _, i := range instances {
		out = append(out, InstanceInfo{
			InstanceId: i.InstanceID,
			Ipv4:       i.Ipv4,
			Port:       i.Port,
		})
	}
	return InstancesResponse{Instances: out}
}
