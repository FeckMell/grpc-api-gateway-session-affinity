package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mygateway/domain"

	"gopkg.in/yaml.v3"
)

// Env variable names.
const (
	envGRPCPort       = "SERVICE_PORT_GRPC"
	envJWTSecret      = "JWT_SECRET"
	envConfigPath     = "CONFIG_PATH"
	envRetryCount     = "RETRY_COUNT"
	envRetryTimeoutMs = "RETRY_TIMEOUT_MS"
)

// Config holds the full gateway configuration loaded by LoadConfig from environment variables and the YAML file.
// GRPCPort is the listening port (from SERVICE_PORT_GRPC); JWTSecret from JWT_SECRET; Routes and Clusters from YAML;
// RetryCount and RetryTimeout for FR-MGW-4 retry on dynamic clusters (from RETRY_COUNT, RETRY_TIMEOUT_MS).
type Config struct {
	GRPCPort     int
	JWTSecret    []byte
	Routes       domain.RouteConfig
	Clusters     map[domain.ClusterID]domain.ClusterConfig
	RetryCount   int
	RetryTimeout time.Duration
}

// yamlConfig is the root struct for YAML unmarshalling; contains default, routes, and clusters.
type yamlConfig struct {
	Default  yamlDefault            `yaml:"default"`
	Routes   []yamlRoute            `yaml:"routes"`
	Clusters map[string]yamlCluster `yaml:"clusters"`
}

// yamlDefault holds the default route action (error|use_cluster) and optional use_cluster name.
type yamlDefault struct {
	Action     string `yaml:"action"`
	UseCluster string `yaml:"use_cluster"`
}

// yamlRoute is one route entry: prefix, cluster name, authorization (none|required), balancer (type and header).
type yamlRoute struct {
	Prefix        string       `yaml:"prefix"`
	Cluster       string       `yaml:"cluster"`
	Authorization string       `yaml:"authorization"`
	Balancer      yamlBalancer `yaml:"balancer"`
}

// yamlBalancer holds balancer type (round_robin|sticky_sessions) and optional header for sticky.
type yamlBalancer struct {
	Type   string `yaml:"type"`
	Header string `yaml:"header"`
}

// yamlCluster is one cluster entry: type (static|dynamic), address (static), discoverer_url and discoverer_interval_ms (dynamic).
type yamlCluster struct {
	Type               string `yaml:"type"`
	Address            string `yaml:"address"`
	DiscovererURL      string `yaml:"discoverer_url"`
	DiscovererInterval int    `yaml:"discoverer_interval_ms"`
}

// loadYAMLConfig reads the YAML file at path and unmarshals it into yamlConfig (default, routes, clusters).
//
// Parameter path — absolute path to the file (LoadConfig converts CONFIG_PATH to absolute via filepath.Abs).
//
// Returns: (*yamlConfig, nil) on successful read and yaml.Unmarshal; (nil, error) on os.ReadFile or yaml.Unmarshal error.
//
// Called only from LoadConfig.
func loadYAMLConfig(path string) (*yamlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out yamlConfig
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LoadConfig builds gateway config from environment variables and YAML at CONFIG_PATH. Reads SERVICE_PORT_GRPC (required, 1–65535), CONFIG_PATH (required), JWT_SECRET (required if any route has authorization=required), RETRY_COUNT and RETRY_TIMEOUT_MS (required, positive). CONFIG_PATH is converted to absolute; YAML is loaded via loadYAMLConfig; routes are normalized (normalizePrefix, authorization, balancer); ValidateRouteConfig is run; clusters are validated for static (address) and dynamic (discoverer_url, discoverer_interval_ms); all route.cluster and default.cluster must exist in clusters.
//
// Parameters: none (source — os.Getenv and file at CONFIG_PATH).
//
// Returns: (*Config, nil) on success; (nil, error) on invalid port, missing CONFIG_PATH/JWT_SECRET (if needed)/RETRY_*, YAML load/parse error, invalid RouteConfig or reference to non-existent cluster.
//
// Called only from main at startup.
func LoadConfig() (*Config, error) {
	grpcPortStr := os.Getenv(envGRPCPort)
	grpcPort, err := strconv.Atoi(grpcPortStr)
	if err != nil || grpcPortStr == "" {
		return nil, fmt.Errorf("%s must be a valid port (1-65535)", envGRPCPort)
	}
	if grpcPort <= 0 || grpcPort > 65535 {
		return nil, fmt.Errorf("%s must be 1-65535, got %d", envGRPCPort, grpcPort)
	}
	configPath := strings.TrimSpace(os.Getenv(envConfigPath))
	if configPath == "" {
		return nil, fmt.Errorf("%s is required", envConfigPath)
	}
	if !filepath.IsAbs(configPath) {
		abs, absErr := filepath.Abs(configPath)
		if absErr != nil {
			return nil, absErr
		}
		configPath = abs
	}
	raw, err := loadYAMLConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", configPath, err)
	}

	routes := make([]domain.Route, 0, len(raw.Routes))
	needsJWT := false
	for _, route := range raw.Routes {
		prefix := normalizePrefix(route.Prefix)
		balancerType := domain.BalancerType(strings.TrimSpace(route.Balancer.Type))
		if balancerType == "" {
			balancerType = domain.BalancerRoundRobin
		}
		auth := domain.AuthorizationMode(strings.TrimSpace(route.Authorization))
		if auth == "" {
			auth = domain.AuthorizationNone
		}
		if auth == domain.AuthorizationRequired {
			needsJWT = true
		}
		routes = append(routes, domain.Route{
			Prefix:        prefix,
			Cluster:       domain.ClusterID(strings.TrimSpace(route.Cluster)),
			Authorization: auth,
			Balancer: domain.BalancerConfig{
				Type:   balancerType,
				Header: strings.TrimSpace(route.Balancer.Header),
			},
		})
	}
	defaultCfg := domain.DefaultRoute{
		Action: domain.DefaultRouteError,
	}
	switch strings.TrimSpace(raw.Default.Action) {
	case "", "error":
		defaultCfg.Action = domain.DefaultRouteError
	case "use_cluster":
		defaultCfg.Action = domain.DefaultRouteUseCluster
		defaultCfg.Cluster = domain.ClusterID(strings.TrimSpace(raw.Default.UseCluster))
	default:
		return nil, fmt.Errorf("default.action must be error|use_cluster")
	}
	routeCfg := domain.RouteConfig{Routes: routes, Default: defaultCfg}
	if err := domain.ValidateRouteConfig(routeCfg); err != nil {
		return nil, err
	}
	clusters := make(map[domain.ClusterID]domain.ClusterConfig, len(raw.Clusters))
	for name, cluster := range raw.Clusters {
		clusterType := domain.ClusterType(strings.TrimSpace(cluster.Type))
		cfg := domain.ClusterConfig{
			Type:               clusterType,
			Address:            strings.TrimSpace(cluster.Address),
			DiscovererURL:      strings.TrimSpace(cluster.DiscovererURL),
			DiscovererInterval: time.Duration(cluster.DiscovererInterval) * time.Millisecond,
		}
		if cfg.Type == domain.ClusterTypeStatic && cfg.Address == "" {
			return nil, fmt.Errorf("cluster %s: address is required for static cluster", name)
		}
		if cfg.Type == domain.ClusterTypeDynamic {
			if cfg.DiscovererURL == "" {
				return nil, fmt.Errorf("cluster %s: discoverer_url is required for dynamic cluster", name)
			}
			if cfg.DiscovererInterval <= 0 {
				return nil, fmt.Errorf("cluster %s: discoverer_interval_ms must be positive", name)
			}
		}
		if cfg.Type != domain.ClusterTypeStatic && cfg.Type != domain.ClusterTypeDynamic {
			return nil, fmt.Errorf("cluster %s: type must be static|dynamic", name)
		}
		clusters[domain.ClusterID(name)] = cfg
	}
	for _, route := range routeCfg.Routes {
		if _, ok := clusters[route.Cluster]; !ok {
			return nil, fmt.Errorf("route prefix %q references unknown cluster %q", route.Prefix, route.Cluster)
		}
	}
	if routeCfg.Default.Action == domain.DefaultRouteUseCluster {
		if _, ok := clusters[routeCfg.Default.Cluster]; !ok {
			return nil, fmt.Errorf("default cluster %q is not defined", routeCfg.Default.Cluster)
		}
	}
	jwtSecret := []byte(strings.TrimSpace(os.Getenv(envJWTSecret)))
	if needsJWT && len(jwtSecret) == 0 {
		return nil, fmt.Errorf("%s is required when at least one route has authorization=required", envJWTSecret)
	}
	retryCountStr := strings.TrimSpace(os.Getenv(envRetryCount))
	if retryCountStr == "" {
		return nil, fmt.Errorf("%s is required", envRetryCount)
	}
	retryCount, err := strconv.Atoi(retryCountStr)
	if err != nil || retryCount < 1 {
		return nil, fmt.Errorf("%s must be a positive integer, got %q", envRetryCount, retryCountStr)
	}
	retryTimeoutMsStr := strings.TrimSpace(os.Getenv(envRetryTimeoutMs))
	if retryTimeoutMsStr == "" {
		return nil, fmt.Errorf("%s is required", envRetryTimeoutMs)
	}
	retryTimeoutMs, err := strconv.Atoi(retryTimeoutMsStr)
	if err != nil || retryTimeoutMs <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer (ms), got %q", envRetryTimeoutMs, retryTimeoutMsStr)
	}
	retryTimeout := time.Duration(retryTimeoutMs) * time.Millisecond
	return &Config{
		GRPCPort:     grpcPort,
		JWTSecret:    jwtSecret,
		Routes:       routeCfg,
		Clusters:     clusters,
		RetryCount:   retryCount,
		RetryTimeout: retryTimeout,
	}, nil
}

// normalizePrefix trims spaces, removes trailing "*" if present and adds leading "/" if needed so route matching (strings.HasPrefix) works correctly.
//
// Parameter prefix — prefix string from YAML (may lack leading "/" or have trailing "*").
//
// Returns: normalized string (leading "/", no trailing "*").
//
// Called only from LoadConfig when parsing routes.
func normalizePrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	if strings.HasSuffix(p, "*") {
		p = strings.TrimSuffix(p, "*")
	}
	if p != "" && p[0] != '/' {
		p = "/" + p
	}
	return p
}
