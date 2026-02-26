package domain

// ServiceInstance is a single backend instance from the discoverer (e.g. GET /v1/instances).
// AssignedClientSessionID is the session bound to this instance, or empty if free.
type ServiceInstance struct {
	InstanceID              string
	Ipv4                    string
	Port                    int
	AssignedClientSessionID string // empty if free
}
