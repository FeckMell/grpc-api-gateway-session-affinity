package service

import (
	"context"
	"io"
	"time"

	"mygateway/domain"
	"mygateway/helpers"
	"mygateway/interfaces"

	"github.com/go-kit/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TransparentProxy is the central gRPC proxy. It is registered with grpc.UnknownServiceHandler so
// all RPCs hit Handler. Functionality: (1) extract method from stream context, (2) match route via
// RouteMatcher, (3) process headers (e.g. auth) via HeaderProcessor, (4) resolve backend connection
// via ConnectionResolver, (5) open a client stream to the backend and bidirectionally forward messages
// using emptypb.Empty (no application-level protobuf parsing). On any backend or stream error it
// calls OnBackendFailure. For dynamic clusters, retries NewStream up to retryCount times with
// retryTimeout per attempt (FR-MGW-4). Fields: router, resolver, headers, logger, retryCount,
// retryTimeout, dynamicClusters.
type TransparentProxy struct {
	router          interfaces.RouteMatcher
	resolver        interfaces.ConnectionResolver
	headers         interfaces.HeaderProcessor
	logger          log.Logger
	retryCount      int
	retryTimeout    time.Duration
	dynamicClusters map[domain.ClusterID]struct{}
}

// NewTransparentProxy creates the proxy with the given router, resolver, header chain and retry parameters. Panics on nil router/resolver/headers/logger (fail-fast at startup).
//
// Parameters: router — method-to-route matching; resolver — backend connection resolution; headers — metadata processing (incl. auth); logger — logger; retryCount — max attempts for dynamic clusters; retryTimeout — timeout per attempt; dynamicClusters — set of ClusterID for which retry is allowed on stream failure.
//
// Returns: *TransparentProxy. Does not return errors (nil dependencies cause panic).
//
// Called from cmd/main when building the gateway.
func NewTransparentProxy(
	router interfaces.RouteMatcher,
	resolver interfaces.ConnectionResolver,
	headers interfaces.HeaderProcessor,
	logger log.Logger,
	retryCount int,
	retryTimeout time.Duration,
	dynamicClusters map[domain.ClusterID]struct{},
) *TransparentProxy {
	return &TransparentProxy{
		router:          helpers.NilPanic(router, "service.transparent.go: router is required"),
		resolver:        helpers.NilPanic(resolver, "service.transparent.go: resolver is required"),
		headers:         helpers.NilPanic(headers, "service.transparent.go: headers is required"),
		logger:          helpers.NilPanic(logger, "service.transparent.go: logger is required"),
		retryCount:      retryCount,
		retryTimeout:    retryTimeout,
		dynamicClusters: dynamicClusters,
	}
}

// Handler implements the handler signature for grpc.UnknownServiceHandler: extracts method from context, matches route, processes headers (auth), gets backend connection, opens stream and forwards messages both ways via emptypb.Empty. On backend/stream error calls OnBackendFailure; for dynamic clusters retries within retryCount.
//
// Parameters: _ — unused (gRPC signature); serverStream — incoming stream from client (RecvMsg/SendMsg to client).
//
// Returns: nil on successful forward completion (EOF from both sides); gRPC status error when method missing in context (Internal), unrouted method (Unimplemented), auth error (Unauthenticated), GetConnection/NewStream or forward error (after interceptor mapping: Unavailable, ResourceExhausted, etc.).
//
// Called by the gRPC server for each unhandled RPC (unary and streaming).
func (p *TransparentProxy) Handler(_ any, serverStream grpc.ServerStream) error {
	fullMethodName, ok := grpc.MethodFromServerStream(serverStream)
	if !ok {
		return status.Errorf(codes.Internal, "missing grpc method in stream context")
	}
	route, ok := p.router.Match(fullMethodName)
	if !ok {
		return status.Error(codes.Unimplemented, "method not routed")
	}

	inMD, _ := metadata.FromIncomingContext(serverStream.Context())
	outMD, err := p.headers.Process(serverStream.Context(), inMD, fullMethodName)
	if err != nil {
		return err
	}
	outCtx := metadata.NewOutgoingContext(serverStream.Context(), outMD)
	_, retryable := p.dynamicClusters[route.Cluster]

	type streamState struct {
		clientStream grpc.ClientStream
		streamCancel context.CancelFunc
		stickyKey    string
		instanceID   string
	}

	openBackendStream := func() (*streamState, error) {
		if retryable {
			for attempt := 0; attempt < p.retryCount; attempt++ {
				backendConn, stickyKey, instanceID, getConnErr := p.resolver.GetConnection(outCtx, route, outMD)
				if getConnErr != nil {
					return nil, getConnErr
				}
				streamCtx, cancel := context.WithCancel(outCtx)
				timer := time.AfterFunc(p.retryTimeout, cancel)
				desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
				clientStream, newStreamErr := backendConn.NewStream(streamCtx, desc, fullMethodName)
				if newStreamErr == nil {
					timer.Stop()
					return &streamState{
						clientStream: clientStream,
						streamCancel: cancel,
						stickyKey:    stickyKey,
						instanceID:   instanceID,
					}, nil
				}
				timer.Stop()
				cancel()
				p.resolver.OnBackendFailure(route, stickyKey, instanceID)
				if attempt == p.retryCount-1 {
					return nil, newStreamErr
				}
			}
			return nil, status.Error(codes.Unavailable, "backend service unavailable")
		}

		backendConn, stickyKey, instanceID, getConnErr := p.resolver.GetConnection(outCtx, route, outMD)
		if getConnErr != nil {
			return nil, getConnErr
		}
		clientCtx, cancel := context.WithCancel(outCtx)
		desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
		clientStream, newStreamErr := backendConn.NewStream(clientCtx, desc, fullMethodName)
		if newStreamErr != nil {
			cancel()
			p.resolver.OnBackendFailure(route, stickyKey, instanceID)
			return nil, newStreamErr
		}
		return &streamState{
			clientStream: clientStream,
			streamCancel: cancel,
			stickyKey:    stickyKey,
			instanceID:   instanceID,
		}, nil
	}

	state, err := openBackendStream()
	if err != nil {
		return err
	}
	defer func() {
		if state != nil && state.streamCancel != nil {
			state.streamCancel()
		}
	}()

	var firstClientMsg *emptypb.Empty
	firstClientMsgCh := make(chan *emptypb.Empty, 1)

	for transferAttempt := 0; ; transferAttempt++ {
		if transferAttempt > 0 {
			// Retry mode for unary/server-stream with one client message:
			// replay the first client message to the new backend stream.
			if firstClientMsg == nil {
				return status.Error(codes.Unavailable, "backend service unavailable")
			}
			msg, ok := proto.Clone(firstClientMsg).(*emptypb.Empty)
			if !ok {
				return status.Error(codes.Internal, "failed to clone first client message")
			}
			if sendErr := state.clientStream.SendMsg(msg); sendErr != nil {
				p.resolver.OnBackendFailure(route, state.stickyKey, state.instanceID)
				state.streamCancel()
				return sendErr
			}
			_ = state.clientStream.CloseSend()

			c2sErr := <-forwardClientToServer(state.clientStream, serverStream, false)
			serverStream.SetTrailer(state.clientStream.Trailer())
			if c2sErr == io.EOF {
				return nil
			}

			p.resolver.OnBackendFailure(route, state.stickyKey, state.instanceID)
			state.streamCancel()
			if !retryable || transferAttempt >= p.retryCount-1 || serverStream.Context().Err() != nil {
				return c2sErr
			}
			nextState, openErr := openBackendStream()
			if openErr != nil {
				return openErr
			}
			state = nextState
			continue
		}

		s2cErrChan := forwardServerToClient(serverStream, state.clientStream, firstClientMsgCh)
		c2sErrChan := forwardClientToServer(state.clientStream, serverStream, true)

		for i := 0; i < 2; i++ {
			select {
			case s2cErr := <-s2cErrChan:
				if s2cErr == io.EOF {
					_ = state.clientStream.CloseSend()
					continue
				}
				select {
				case firstClientMsg = <-firstClientMsgCh:
				default:
				}
				p.resolver.OnBackendFailure(route, state.stickyKey, state.instanceID)
				state.streamCancel()
				if !retryable || firstClientMsg == nil || transferAttempt >= p.retryCount-1 || serverStream.Context().Err() != nil {
					return s2cErr
				}
				nextState, openErr := openBackendStream()
				if openErr != nil {
					return openErr
				}
				state = nextState
				goto retryNextBackend
			case c2sErr := <-c2sErrChan:
				serverStream.SetTrailer(state.clientStream.Trailer())
				if c2sErr == io.EOF {
					return nil
				}
				select {
				case firstClientMsg = <-firstClientMsgCh:
				default:
				}
				p.resolver.OnBackendFailure(route, state.stickyKey, state.instanceID)
				state.streamCancel()
				if !retryable || firstClientMsg == nil || transferAttempt >= p.retryCount-1 || serverStream.Context().Err() != nil {
					return c2sErr
				}
				nextState, openErr := openBackendStream()
				if openErr != nil {
					return openErr
				}
				state = nextState
				goto retryNextBackend
			}
		}

		return status.Errorf(codes.Internal, "unexpected proxy termination")

	retryNextBackend:
		continue
	}
}

// forwardClientToServer in a goroutine forwards messages from backend (src) to client (dst) using emptypb.Empty without protobuf parsing. When sendHeader=true the first response sends backend response headers to client via dst.SendHeader.
//
// Parameters: src — client stream to backend (RecvMsg); dst — server stream to client (SendMsg); sendHeader — when true first response is accompanied by SendHeader(md) from backend.
//
// Returns: channel written once with error: io.EOF on normal end of receive from backend or gRPC/write to dst error.
//
// Called only from TransparentProxy.Handler.
func forwardClientToServer(src grpc.ClientStream, dst grpc.ServerStream, sendHeader bool) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &emptypb.Empty{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if i == 0 && sendHeader {
				md, err := src.Header()
				if err != nil {
					ret <- err
					break
				}
				if err := dst.SendHeader(md); err != nil {
					ret <- err
					break
				}
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}

// forwardServerToClient in a goroutine forwards messages from client (src) to backend (dst). On first message puts a clone into firstClientMsgOut for possible retry/transfer to another instance.
//
// Parameters: src — server stream from client (RecvMsg); dst — client stream to backend (SendMsg); firstClientMsgOut — channel for first message (may be nil); when channel is non-empty and send would block, send is skipped (non-blocking).
//
// Returns: channel written once with error: io.EOF on normal end of receive from client or gRPC/write to dst error.
//
// Called only from TransparentProxy.Handler.
func forwardServerToClient(src grpc.ServerStream, dst grpc.ClientStream, firstClientMsgOut chan<- *emptypb.Empty) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &emptypb.Empty{}
		first := true
		for {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if first && firstClientMsgOut != nil {
				if cloned, ok := proto.Clone(f).(*emptypb.Empty); ok {
					select {
					case firstClientMsgOut <- cloned:
					default:
					}
				}
				first = false
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}
