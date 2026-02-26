package scenario

import "context"

// Runner runs a single scenario with the given config. Each scenario should create
// its own gRPC client and context.
type Runner func(ctx context.Context, cfg *Config) error

var registry = make(map[string]Runner)

// Register adds a scenario by name. Call from init() in scenario files.
func Register(name string, fn Runner) {
	registry[name] = fn
}

// All returns all registered scenario names and their runners.
func All() map[string]Runner {
	out := make(map[string]Runner, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// Run runs the named scenario. Returns the runner's error if the scenario exists.
func Run(name string, ctx context.Context, cfg *Config) error {
	fn, ok := registry[name]
	if !ok {
		return &UnknownScenarioError{Name: name}
	}
	return fn(ctx, cfg)
}
