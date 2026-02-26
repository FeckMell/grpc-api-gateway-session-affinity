package domain

import "time"

// ClusterType is static (single address) or dynamic (instances from discoverer).
type ClusterType string

const (
	ClusterTypeStatic  ClusterType = "static"
	ClusterTypeDynamic ClusterType = "dynamic"
)

// ClusterConfig holds cluster type and, for static, Address; for dynamic, DiscovererURL and DiscovererInterval.
type ClusterConfig struct {
	Type               ClusterType
	Address            string
	DiscovererURL      string
	DiscovererInterval time.Duration
}
