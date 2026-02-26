package domain

import "time"

// Instance represents a registered instance stored by MyDiscoverer.
// Fields match API: instance_id, service_type, ipv4, port, timestamp, ttl_ms.
type Instance struct {
	InstanceID  string    // unique instance identifier
	ServiceType string
	Ipv4        string    // IPv4 address
	Port        int       // port
	Timestamp   time.Time // timestamp from request
	TTLMs       int       // TTL in milliseconds
}
