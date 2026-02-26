package handlers

import (
	"mydiscoverer/domain"
	"mydiscoverer/service"
)

// fromRegisterRequest converts RegisterRequest to domain.Instance.
// Returns service.BadParameterError on validation failure.
func fromRegisterRequest(req RegisterRequest) (domain.Instance, error) {
	if req.InstanceId == "" {
		return domain.Instance{}, service.NewBadParameterError("instance_id is required", nil)
	}
	if req.ServiceType == "" {
		return domain.Instance{}, service.NewBadParameterError("service_type is required", nil)
	}
	if req.Ipv4 == "" {
		return domain.Instance{}, service.NewBadParameterError("ipv4 is required", nil)
	}
	if req.Port == 0 {
		return domain.Instance{}, service.NewBadParameterError("port is required", nil)
	}
	if req.TtlMs <= 0 {
		return domain.Instance{}, service.NewBadParameterError("ttl_ms is required", nil)
	}

	return domain.Instance{
		InstanceID:  req.InstanceId,
		ServiceType: req.ServiceType,
		Ipv4:        req.Ipv4,
		Port:        req.Port,
		Timestamp:   req.Timestamp,
		TTLMs:       req.TtlMs,
	}, nil
}
