package helpers

import (
	"context"
	"sort"
	"strings"

	"mygateway/domain"
	"mygateway/interfaces"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthRule is an internal rule built from route config: a method prefix and the authorization mode for that prefix.
// Used for longest-prefix lookup in Process to decide whether to require JWT for the current method.
type AuthRule struct {
	Prefix        string
	Authorization domain.AuthorizationMode
}

// ConfigurableAuthProcessor implements interfaces.HeaderProcessor. It applies per-route authorization:
// for methods matching a route with authorization=required it requires session-id and authorization metadata
// and validates the JWT via JwtService; for authorization=none it passes headers through unchanged.
// Holds JwtService and a list of AuthRules sorted by prefix length (descending) for longest-prefix match.
type ConfigurableAuthProcessor struct {
	JwtService interfaces.JwtService
	rules      []AuthRule
}

// NewConfigurableAuthProcessor creates a header processor with per-route authorization: AuthRules are built from routes and sorted by descending prefix length for longest-prefix in Process. Panics on nil jwt.
//
// Parameters: jwt — JWT validation service; routes — routes from config (may be empty — then authorization=none for any method).
//
// Returns: *ConfigurableAuthProcessor implementing interfaces.HeaderProcessor.
//
// Called from cmd/main when building the header chain.
func NewConfigurableAuthProcessor(jwt interfaces.JwtService, routes []domain.Route) *ConfigurableAuthProcessor {
	rules := make([]AuthRule, 0, len(routes))
	for _, r := range routes {
		mode := r.Authorization
		if mode == "" {
			mode = domain.AuthorizationNone
		}
		rules = append(rules, AuthRule{Prefix: r.Prefix, Authorization: mode})
	}
	sort.Slice(rules, func(i, j int) bool {
		return len(rules[i].Prefix) > len(rules[j].Prefix)
	})
	return &ConfigurableAuthProcessor{
		JwtService: NilPanic(jwt, "helpers.configurable_auth_processor.go: JwtService is required"),
		rules:      rules,
	}
}

// Process selects a rule by longest-prefix for method; when authorization=required it extracts session-id and authorization, validates token via JwtService and on success returns headers unchanged, on error returns gRPC status (Unauthenticated or Internal). When authorization=none returns headers unchanged.
//
// Parameters: ctx — request context (passed to JwtService if needed); headers — incoming client metadata; method — full gRPC method name.
//
// Returns: (headers, nil) when auth is not required or validation passed; (nil, status.Error) when session-id is missing ("missing session-id"), token missing/invalid ("missing or invalid token") or validation internal error (Internal).
//
// Called from HeaderProcessorChain.Process inside TransparentProxy.Handler.
func (p *ConfigurableAuthProcessor) Process(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error) {
	mode := domain.AuthorizationNone
	for _, rule := range p.rules {
		if strings.HasPrefix(method, rule.Prefix) {
			mode = rule.Authorization
			break
		}
	}
	if mode != domain.AuthorizationRequired {
		return headers, nil
	}
	sessionID, ok := GetSessionID(headers)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing session-id")
	}
	token, ok := GetAuthToken(headers)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing or invalid token")
	}
	valid, err := p.JwtService.ValidateToken(sessionID, token)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !valid {
		return nil, status.Error(codes.Unauthenticated, "missing or invalid token")
	}
	return headers, nil
}
