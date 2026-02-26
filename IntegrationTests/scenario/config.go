package scenario

// Config holds settings for running a scenario (gateway address, login credentials, optional compose path).
type Config struct {
	GatewayAddr string
	Username    string
	Password    string
	// ComposePath is the path to docker-compose.yml (used by scenarios that need to stop/start services).
	ComposePath string
}
