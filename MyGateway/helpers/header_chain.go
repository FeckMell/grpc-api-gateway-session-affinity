package helpers

import (
	"context"
	"strconv"

	"mygateway/interfaces"

	"google.golang.org/grpc/metadata"
)

// HeaderProcessorChain is a slice of HeaderProcessors run in sequence; each processor receives
// the output metadata of the previous. Used to compose auth (ConfigurableAuthProcessor) and any
// future processors (e.g. logging, tracing). Implements interfaces.HeaderProcessor.
type HeaderProcessorChain []interfaces.HeaderProcessor

// NewHeaderProcessorChain creates a chain of header processors from the given list. Panics on nil slice or nil element (fail-fast at startup).
//
// Parameters: processors — ordered list of HeaderProcessor implementations (Process call order: first gets incoming headers, next gets previous result).
//
// Returns: HeaderProcessorChain ([]HeaderProcessor) implementing interfaces.HeaderProcessor.
//
// Called from cmd/main when building the gateway (e.g. chain of a single ConfigurableAuthProcessor).
func NewHeaderProcessorChain(processors ...interfaces.HeaderProcessor) HeaderProcessorChain {
	for i, p := range processors {
		if p == nil {
			panic("helpers.header_chain.go: processor at index " + strconv.Itoa(i) + " is required")
		}
	}
	return HeaderProcessorChain(NilPanic(processors, "helpers.header_chain.go: processors is required"))
}

// Process runs all processors in order: output of one is input to the next. Input headers are not mutated (work is done on a copy). Returns the first processor error.
//
// Parameters: ctx — request context; headers — incoming metadata from client; method — full gRPC method name (for per-route policy inside processors).
//
// Returns: (outgoing metadata, nil) when all processors succeed; (nil, error) on any processor error (often gRPC status: Unauthenticated, Internal).
//
// Called from service.TransparentProxy.Handler after Match and before GetConnection.
func (c HeaderProcessorChain) Process(ctx context.Context, headers metadata.MD, method string) (metadata.MD, error) {
	out := headers.Copy()
	for _, p := range c {
		next, err := p.Process(ctx, out, method)
		if err != nil {
			return nil, err
		}
		out = next
	}
	return out, nil
}
