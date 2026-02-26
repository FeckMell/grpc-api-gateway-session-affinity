package interfaces

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// HeaderProcessor processes incoming gRPC metadata before forwarding to the backend
// and returns the outgoing metadata to send (or a gRPC error).
//
// Used primarily for authentication (e.g. JWT validation for routes with
// authorization=required). Must not mutate the input headers; return a copy with
// modifications. If an error is returned, the proxy returns it to the client as-is
// (typically codes.Unauthenticated or codes.Internal).
//
// Process(ctx, headers, method): ctx is the request context; headers is the incoming
// metadata from the client; method is the full gRPC method name (used for per-route
// policy, e.g. longest-prefix match). Returns (outgoing metadata, nil) or (nil, error).
//
// Implemented by helpers.ConfigurableAuthProcessor and composed in helpers.HeaderProcessorChain.
// Called from service.TransparentProxy.Handler after route matching and before ConnectionResolver.GetConnection.
//
//go:generate moq -stub -out mock/header_processor.go -pkg mock . HeaderProcessor
type HeaderProcessor interface {
	// Process processes incoming gRPC metadata before forwarding to the backend (auth, header enrichment). Must not mutate headers; returns a copy with changes.
	// Parameters: ctx — request context (cancel/timeout handled by implementation); headers — incoming metadata from client (nil allowed, implementation must handle); method — full gRPC method name (e.g. /pkg.Svc/Method), used for longest-prefix policy selection by route.
	// Returns: (outgoing metadata.MD, nil) on success; (nil, error) on error (often gRPC status: Unauthenticated or Internal). Error is returned to the client as-is.
	// Called from service.TransparentProxy.Handler after Match and before ConnectionResolver.GetConnection.
	Process(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error)
}
