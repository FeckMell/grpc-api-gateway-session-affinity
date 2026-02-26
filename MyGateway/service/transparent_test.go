package service

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"mygateway/domain"
	"mygateway/interfaces"
	"mygateway/interfaces/mock"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newProxyForTest(router interfaces.RouteMatcher, resolver interfaces.ConnectionResolver, headers interfaces.HeaderProcessor, logger log.Logger, dynamicClusters map[domain.ClusterID]struct{}) *TransparentProxy {
	if dynamicClusters == nil {
		dynamicClusters = map[domain.ClusterID]struct{}{}
	}
	return NewTransparentProxy(router, resolver, headers, logger, 3, 5*time.Second, dynamicClusters)
}

func TestNewTransparentProxy_Panics(t *testing.T) {
	router := &mock.RouteMatcherMock{}
	resolver := &mock.ConnectionResolverMock{}
	headers := &mock.HeaderProcessorMock{}
	logger := log.NewNopLogger()
	dynamicClusters := map[domain.ClusterID]struct{}{}

	tests := []struct {
		name     string
		router   interfaces.RouteMatcher
		resolver interfaces.ConnectionResolver
		headers  interfaces.HeaderProcessor
		logger   log.Logger
		panicMsg string
	}{
		{"router_nil", nil, resolver, headers, logger, "service.transparent.go: router is required"},
		{"resolver_nil", router, nil, headers, logger, "service.transparent.go: resolver is required"},
		{"headers_nil", router, resolver, nil, logger, "service.transparent.go: headers is required"},
		{"logger_nil", router, resolver, headers, nil, "service.transparent.go: logger is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.PanicsWithValue(t, tt.panicMsg, func() {
				NewTransparentProxy(tt.router, tt.resolver, tt.headers, tt.logger, 3, 5*time.Second, dynamicClusters)
			})
		})
	}
}

// startProxyServer starts a gRPC server that uses the given proxy as UnknownServiceHandler.
// Caller must call srv.Stop() and close the listener when done.
func startProxyServer(t *testing.T, proxy *TransparentProxy) (net.Listener, *grpc.Server) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer(grpc.UnknownServiceHandler(proxy.Handler))
	go func() { _ = srv.Serve(lis) }()
	return lis, srv
}

// bidiSvcServer is the interface required by the test backend service desc.
type bidiSvcServer interface {
	Method(grpc.ServerStream) error
}

// bidiBackendImpl implements a single bidi stream; the handler function defines behavior (echo, return error, etc.).
type bidiBackendImpl struct {
	handler func(grpc.ServerStream) error
}

func (b *bidiBackendImpl) Method(stream grpc.ServerStream) error {
	return b.handler(stream)
}

// startBidiBackend starts a gRPC server that serves one bidirectional stream at "/svc/Method".
// handler is called for each stream; use it to implement echo, immediate error, or blocking behavior.
// Caller must call srv.Stop() and lis.Close() when done.
func startBidiBackend(t *testing.T, handler func(grpc.ServerStream) error) (net.Listener, *grpc.Server) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	impl := &bidiBackendImpl{handler: handler}
	sd := &grpc.ServiceDesc{
		ServiceName: "svc",
		HandlerType: (*bidiSvcServer)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Method",
				Handler:       bidiBackendStreamHandler,
				ServerStreams: true,
				ClientStreams: true,
			},
		},
	}
	srv.RegisterService(sd, impl)
	go func() { _ = srv.Serve(lis) }()
	return lis, srv
}

func bidiBackendStreamHandler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(bidiSvcServer).Method(stream)
}

func TestTransparentProxy_Handler(t *testing.T) {
	t.Run("method_not_routed_returns_unimplemented", func(t *testing.T) {
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) { return domain.Route{}, false },
		}
		resolver := &mock.ConnectionResolverMock{}
		headers := &mock.HeaderProcessorMock{}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)

		lis, srv := startProxyServer(t, proxy)
		defer srv.Stop()
		defer lis.Close()

		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()
		stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/nomatch/Method")
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unimplemented, st.Code())
		assert.Contains(t, st.Message(), "method not routed")
	})

	t.Run("process_returns_error_propagates_status", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		resolver := &mock.ConnectionResolverMock{}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return nil, status.Error(codes.Unauthenticated, "auth failed")
			},
		}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)

		lis, srv := startProxyServer(t, proxy)
		defer srv.Stop()
		defer lis.Close()

		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()
		stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
		assert.Contains(t, st.Message(), "auth failed")
	})

	t.Run("getconn_returns_error_propagates_status", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				return nil, "", "", status.Error(codes.Unavailable, "no backend")
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)

		lis, srv := startProxyServer(t, proxy)
		defer srv.Stop()
		defer lis.Close()

		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		ctx := context.Background()
		stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
		assert.Contains(t, st.Message(), "no backend")
	})

	t.Run("newstream_fails", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		backendLis, backendSrv := startBidiBackend(t, func(grpc.ServerStream) error { return nil })
		backendConn, err := grpc.NewClient(backendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		backendSrv.Stop()
		_ = backendLis.Close()
		var onFailureCalls int32
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				return backendConn, "stickyKey", "instanceID", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				atomic.AddInt32(&onFailureCalls, 1)
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)
		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		assert.GreaterOrEqual(t, atomic.LoadInt32(&onFailureCalls), int32(1), "OnBackendFailure should be called when NewStream fails")
		_ = backendConn.Close()
	})

	t.Run("s2c_error_client_cancel", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		backendLis, backendSrv := startBidiBackend(t, func(stream grpc.ServerStream) error {
			var m emptypb.Empty
			for {
				if err := stream.RecvMsg(&m); err != nil {
					return err
				}
			}
		})
		defer backendSrv.Stop()
		defer backendLis.Close()
		backendConn, err := grpc.NewClient(backendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer backendConn.Close()

		onFailureCh := make(chan struct{}, 1)
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				return backendConn, "", "", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				select {
				case onFailureCh <- struct{}{}:
				default:
				}
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)
		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)
		// Send one message so the proxy is actively forwarding; then close the client connection
		// so the proxy's serverStream.RecvMsg sees connection closed (s2c error path).
		require.NoError(t, stream.SendMsg(&emptypb.Empty{}))
		clientConn.Close()

		// Wait for proxy to observe the closed connection and call OnBackendFailure (with timeout).
		select {
		case <-onFailureCh:
			// OnBackendFailure was called (s2c or c2s path).
		case <-time.After(2 * time.Second):
			t.Fatal("OnBackendFailure was not called within 2s after client closed connection")
		}

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		if st, ok := status.FromError(err); ok && st.Code() == codes.Internal {
			assert.Contains(t, st.Message(), "failed proxying s2c")
		}
	})

	t.Run("c2s_error_backend_returns_error", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		backendLis, backendSrv := startBidiBackend(t, func(grpc.ServerStream) error {
			return status.Error(codes.Unimplemented, "test")
		})
		defer backendSrv.Stop()
		defer backendLis.Close()
		backendConn, err := grpc.NewClient(backendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer backendConn.Close()

		var onFailureCalls int32
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				return backendConn, "", "", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				atomic.AddInt32(&onFailureCalls, 1)
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)
		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unimplemented, st.Code())
		assert.Contains(t, st.Message(), "test")
		assert.GreaterOrEqual(t, atomic.LoadInt32(&onFailureCalls), int32(1), "OnBackendFailure should be called on c2s error")
	})

	t.Run("success_path_full_bidi_proxy", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		backendLis, backendSrv := startBidiBackend(t, func(stream grpc.ServerStream) error {
			for {
				var m emptypb.Empty
				if err := stream.RecvMsg(&m); err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				if err := stream.SendMsg(&m); err != nil {
					return err
				}
			}
		})
		defer backendSrv.Stop()
		defer backendLis.Close()
		backendConn, err := grpc.NewClient(backendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer backendConn.Close()

		var onFailureCalls int32
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				return backendConn, "", "", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				atomic.AddInt32(&onFailureCalls, 1)
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		proxy := newProxyForTest(router, resolver, headers, log.NewNopLogger(), nil)
		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		err = stream.SendMsg(&emptypb.Empty{})
		require.NoError(t, err)
		err = stream.CloseSend()
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.NoError(t, err)
		for {
			err = stream.RecvMsg(&recv)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		assert.Equal(t, int32(0), atomic.LoadInt32(&onFailureCalls), "OnBackendFailure should not be called on success")
	})

	t.Run("retry_succeeds_on_second_attempt", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		badBackendLis, badBackendSrv := startBidiBackend(t, func(grpc.ServerStream) error { return nil })
		badConn, err := grpc.NewClient(badBackendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		badBackendSrv.Stop()
		_ = badBackendLis.Close()
		goodBackendLis, goodBackendSrv := startBidiBackend(t, func(stream grpc.ServerStream) error {
			for {
				var m emptypb.Empty
				if err := stream.RecvMsg(&m); err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				if err := stream.SendMsg(&m); err != nil {
					return err
				}
			}
		})
		defer goodBackendSrv.Stop()
		defer goodBackendLis.Close()
		goodConn, err := grpc.NewClient(goodBackendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer goodConn.Close()

		var getConnCalls int32
		var onFailureCalls int32
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				n := atomic.AddInt32(&getConnCalls, 1)
				if n == 1 {
					return badConn, "", "", nil
				}
				return goodConn, "", "", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				atomic.AddInt32(&onFailureCalls, 1)
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		dynamicClusters := map[domain.ClusterID]struct{}{"test": {}}
		proxy := NewTransparentProxy(router, resolver, headers, log.NewNopLogger(), 3, 5*time.Second, dynamicClusters)
		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		err = stream.SendMsg(&emptypb.Empty{})
		require.NoError(t, err)
		err = stream.CloseSend()
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.NoError(t, err)
		for {
			err = stream.RecvMsg(&recv)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		assert.Equal(t, int32(2), atomic.LoadInt32(&getConnCalls), "GetConnection should be called twice (first failed, second succeeded)")
		assert.Equal(t, int32(1), atomic.LoadInt32(&onFailureCalls), "OnBackendFailure should be called once for the failed first attempt")
		_ = badConn.Close()
	})

	t.Run("retry_exhausted_returns_error", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}
		badBackendLis, badBackendSrv := startBidiBackend(t, func(grpc.ServerStream) error { return nil })
		badConn, err := grpc.NewClient(badBackendLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		badBackendSrv.Stop()
		_ = badBackendLis.Close()

		var onFailureCalls int32
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				return badConn, "", "", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				atomic.AddInt32(&onFailureCalls, 1)
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.New(nil), nil
			},
		}
		dynamicClusters := map[domain.ClusterID]struct{}{"test": {}}
		proxy := NewTransparentProxy(router, resolver, headers, log.NewNopLogger(), 3, 5*time.Second, dynamicClusters)
		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		var recv emptypb.Empty
		err = stream.RecvMsg(&recv)
		require.Error(t, err)
		assert.Equal(t, int32(3), atomic.LoadInt32(&onFailureCalls), "OnBackendFailure should be called for each of the 3 failed attempts")
		_ = badConn.Close()
	})

	t.Run("dynamic_cluster_transfers_stream_and_replays_first_message", func(t *testing.T) {
		route := domain.Route{Prefix: "/svc/", Cluster: "test", Authorization: domain.AuthorizationNone}
		router := &mock.RouteMatcherMock{
			MatchFunc: func(method string) (domain.Route, bool) {
				if method == "/svc/Method" {
					return route, true
				}
				return domain.Route{}, false
			},
		}

		backend1Lis, backend1Srv := startBidiBackend(t, func(stream grpc.ServerStream) error {
			var req emptypb.Empty
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
				return err
			}
			return status.Error(codes.Unavailable, "backend-1 dropped")
		})
		defer backend1Srv.Stop()
		defer backend1Lis.Close()
		backend1Conn, err := grpc.NewClient(backend1Lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer backend1Conn.Close()

		var backend2Auth string
		var backend2Session string
		backend2Lis, backend2Srv := startBidiBackend(t, func(stream grpc.ServerStream) error {
			md, _ := metadata.FromIncomingContext(stream.Context())
			backend2Auth = firstMDValue(md, "authorization")
			backend2Session = firstMDValue(md, "session-id")

			var req emptypb.Empty
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
				return err
			}
			return nil
		})
		defer backend2Srv.Stop()
		defer backend2Lis.Close()
		backend2Conn, err := grpc.NewClient(backend2Lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer backend2Conn.Close()

		var getConnCalls int32
		var onFailureCalls int32
		resolver := &mock.ConnectionResolverMock{
			GetConnectionFunc: func(ctx context.Context, r domain.Route, headers metadata.MD) (*grpc.ClientConn, string, string, error) {
				if atomic.AddInt32(&getConnCalls, 1) == 1 {
					return backend1Conn, "sticky-1", "instance-1", nil
				}
				return backend2Conn, "sticky-1", "instance-2", nil
			},
			OnBackendFailureFunc: func(domain.Route, string, string) {
				atomic.AddInt32(&onFailureCalls, 1)
			},
		}
		headers := &mock.HeaderProcessorMock{
			ProcessFunc: func(ctx context.Context, md metadata.MD, method string) (metadata.MD, error) {
				return metadata.Pairs("authorization", "Bearer test-token", "session-id", "session-123"), nil
			},
		}
		dynamicClusters := map[domain.ClusterID]struct{}{"test": {}}
		proxy := NewTransparentProxy(router, resolver, headers, log.NewNopLogger(), 3, 5*time.Second, dynamicClusters)

		proxyLis, proxySrv := startProxyServer(t, proxy)
		defer proxySrv.Stop()
		defer proxyLis.Close()

		clientConn, err := grpc.NewClient(proxyLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer clientConn.Close()

		ctx := context.Background()
		stream, err := clientConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/svc/Method")
		require.NoError(t, err)

		require.NoError(t, stream.SendMsg(&emptypb.Empty{}))
		require.NoError(t, stream.CloseSend())

		var recv emptypb.Empty
		require.NoError(t, stream.RecvMsg(&recv))
		require.NoError(t, stream.RecvMsg(&recv))
		err = stream.RecvMsg(&recv)
		require.Equal(t, io.EOF, err)

		assert.Equal(t, int32(2), atomic.LoadInt32(&getConnCalls), "stream should be transferred to second backend")
		assert.GreaterOrEqual(t, atomic.LoadInt32(&onFailureCalls), int32(1), "OnBackendFailure should be called when first backend fails")
		assert.Equal(t, "Bearer test-token", backend2Auth, "authorization metadata must be preserved on transferred stream")
		assert.Equal(t, "session-123", backend2Session, "session-id metadata must be preserved on transferred stream")
	})
}

func firstMDValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
