package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"mygateway/domain"
	"mygateway/interfaces/mock"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newTestConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop() })
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestNewConnPool_Panics(t *testing.T) {
	disco := &mock.DiscovererMock{GetInstancesFunc: func() ([]domain.ServiceInstance, error) { return nil, nil }}
	factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) { return nil, nil }
	logger := log.NewNopLogger()
	interval := time.Hour

	t.Run("discoverer_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.connection_pool.go: discoverer is required", func() {
			NewConnectionPool(nil, factory, interval, logger)
		})
	})
	t.Run("factory_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.connection_pool.go: factory is required", func() {
			NewConnectionPool(disco, nil, interval, logger)
		})
	})
	t.Run("logger_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.connection_pool.go: logger is required", func() {
			NewConnectionPool(disco, factory, interval, nil)
		})
	})
}

func TestConnPool_GetConnRoundRobin(t *testing.T) {
	ctx := context.Background()
	testConn := newTestConn(t)
	instances := []domain.ServiceInstance{
		{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
	}

	t.Run("empty_instances_returns_error", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return []domain.ServiceInstance{}, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		_, _, err := p.GetConnectionRoundRobin(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoAvailableConnInstance)
	})

	t.Run("one_instance_factory_succeeds_returns_conn", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return instances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		conn, id, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, "i1", id)
	})

	t.Run("closed_pool_returns_error", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return instances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		p.Close()
		_, _, err := p.GetConnectionRoundRobin(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrConnPoolClosed)
	})

	t.Run("all_factory_calls_fail_returns_error", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return instances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return nil, errors.New("dial failed")
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		_, _, err := p.GetConnectionRoundRobin(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoAvailableConnInstance)
	})

	t.Run("two_instances_first_factory_fails_second_succeeds", func(t *testing.T) {
		twoInstances := []domain.ServiceInstance{
			{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
			{InstanceID: "i2", Ipv4: "127.0.0.1", Port: 9002},
		}
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return twoInstances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			if inst.InstanceID == "i1" {
				return nil, errors.New("dial failed")
			}
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		conn, id, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, "i2", id)
	})

	t.Run("two_instances_round_robin_order", func(t *testing.T) {
		conn2 := newTestConn(t)
		twoInstances := []domain.ServiceInstance{
			{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
			{InstanceID: "i2", Ipv4: "127.0.0.1", Port: 9002},
		}
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return twoInstances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			if inst.InstanceID == "i1" {
				return testConn, nil
			}
			return conn2, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		connA, idA, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		require.NotNil(t, connA)
		connB, idB, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		require.NotNil(t, connB)
		connC, idC, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		require.NotNil(t, connC)
		assert.Equal(t, "i1", idA)
		assert.Equal(t, "i2", idB)
		assert.Equal(t, "i1", idC)
		assert.Same(t, testConn, connA)
		assert.Same(t, conn2, connB)
		assert.Same(t, testConn, connC)
	})
}

func TestConnPool_GetConnForKey(t *testing.T) {
	ctx := context.Background()
	testConn := newTestConn(t)
	instances := []domain.ServiceInstance{
		{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
	}

	t.Run("empty_key_returns_error", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return instances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		_, _, err := p.GetConnectionForKey(ctx, "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoAvailableConnInstance)
	})

	t.Run("existing_key_returns_cached_conn", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return instances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		conn1, id1, err := p.GetConnectionForKey(ctx, "sess-a")
		require.NoError(t, err)
		require.NotNil(t, conn1)
		conn2, id2, err := p.GetConnectionForKey(ctx, "sess-a")
		require.NoError(t, err)
		assert.Same(t, conn1, conn2)
		assert.Equal(t, id1, id2)
		assert.Equal(t, "i1", id1)
	})

	t.Run("closed_pool_returns_error", func(t *testing.T) {
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return instances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		p.Close()
		_, _, err := p.GetConnectionForKey(ctx, "sess-a")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrConnPoolClosed)
	})

	t.Run("instance_occupied_by_other_session_uses_free_instance", func(t *testing.T) {
		conn2 := newTestConn(t)
		twoInstances := []domain.ServiceInstance{
			{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
			{InstanceID: "i2", Ipv4: "127.0.0.1", Port: 9002},
		}
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return twoInstances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			if inst.InstanceID == "i1" {
				return testConn, nil
			}
			return conn2, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		// First bind "other" to i1
		_, _, err := p.GetConnectionForKey(ctx, "other")
		require.NoError(t, err)
		// sess-a should get i2 (i1 is occupied by "other")
		conn, id, err := p.GetConnectionForKey(ctx, "sess-a")
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, "i2", id)
	})

	t.Run("all_instances_occupied_returns_error", func(t *testing.T) {
		conn2 := newTestConn(t)
		twoInstances := []domain.ServiceInstance{
			{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
			{InstanceID: "i2", Ipv4: "127.0.0.1", Port: 9002},
		}
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return twoInstances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			if inst.InstanceID == "i1" {
				return testConn, nil
			}
			return conn2, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		// Bind both instances to other sessions
		_, _, err := p.GetConnectionForKey(ctx, "other1")
		require.NoError(t, err)
		_, _, err = p.GetConnectionForKey(ctx, "other2")
		require.NoError(t, err)
		// sess-a should get ErrNoAvailableConnInstance
		_, _, err = p.GetConnectionForKey(ctx, "sess-a")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoAvailableConnInstance)
	})

	t.Run("factory_fails_for_first_matching_instance_succeeds_for_second", func(t *testing.T) {
		twoInstances := []domain.ServiceInstance{
			{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
			{InstanceID: "i2", Ipv4: "127.0.0.1", Port: 9002},
		}
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				return twoInstances, nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			if inst.InstanceID == "i1" {
				return nil, errors.New("dial failed")
			}
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
		defer p.Close()
		conn, id, err := p.GetConnectionForKey(ctx, "sess-a")
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, "i2", id)
	})
}

func TestConnPool_Refresh(t *testing.T) {
	ctx := context.Background()
	testConn := newTestConn(t)
	instances := []domain.ServiceInstance{
		{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
		{InstanceID: "i2", Ipv4: "127.0.0.1", Port: 9002},
	}

	t.Run("refresh_on_GetInstances_error_keeps_previous_instances", func(t *testing.T) {
		callCount := 0
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				callCount++
				if callCount == 1 {
					return instances[:1], nil
				}
				return nil, errors.New("discoverer error")
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 20*time.Millisecond, log.NewNopLogger())
		defer p.Close()
		_, _, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
		_, _, err = p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
	})

	t.Run("refresh_removes_instance_and_unbinds_key", func(t *testing.T) {
		callCount := 0
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				callCount++
				if callCount == 1 {
					return instances, nil
				}
				return instances[1:], nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 20*time.Millisecond, log.NewNopLogger())
		defer p.Close()
		_, id1, err := p.GetConnectionForKey(ctx, "sess-a")
		require.NoError(t, err)
		require.Contains(t, []string{"i1", "i2"}, id1)
		time.Sleep(30 * time.Millisecond)
		_, id2, err := p.GetConnectionForKey(ctx, "sess-a")
		require.NoError(t, err)
		assert.Equal(t, "i2", id2)
	})

	t.Run("refresh_resets_rr_when_out_of_range", func(t *testing.T) {
		callCount := 0
		disco := &mock.DiscovererMock{
			GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
				callCount++
				if callCount == 1 {
					return instances, nil
				}
				return instances[:1], nil
			},
		}
		factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
			return testConn, nil
		}
		p := NewConnectionPool(disco, factory, 20*time.Millisecond, log.NewNopLogger())
		defer p.Close()
		_, _, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
		conn, id, err := p.GetConnectionRoundRobin(ctx)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, "i1", id)
	})
}

func TestConnPool_OnBackendFailure(t *testing.T) {
	testConn := newTestConn(t)
	instances := []domain.ServiceInstance{
		{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001},
	}
	var unregisterCalls []string
	disco := &mock.DiscovererMock{
		GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
			return instances, nil
		},
		UnregisterInstanceFunc: func(instanceID string) error {
			unregisterCalls = append(unregisterCalls, instanceID)
			return nil
		},
	}
	factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
		return testConn, nil
	}
	p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
	defer p.Close()

	ctx := context.Background()
	_, id, err := p.GetConnectionForKey(ctx, "sess-x")
	require.NoError(t, err)
	require.Equal(t, "i1", id)

	p.OnBackendFailure("sess-x", "i1")
	assert.Equal(t, []string{"i1"}, unregisterCalls)
}

func TestConnPool_Close(t *testing.T) {
	testConn := newTestConn(t)
	disco := &mock.DiscovererMock{
		GetInstancesFunc: func() ([]domain.ServiceInstance, error) {
			return []domain.ServiceInstance{{InstanceID: "i1", Ipv4: "127.0.0.1", Port: 9001}}, nil
		},
	}
	factory := func(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
		return testConn, nil
	}
	p := NewConnectionPool(disco, factory, 10*time.Second, log.NewNopLogger())
	err := p.Close()
	require.NoError(t, err)
	err = p.Close()
	require.NoError(t, err)
	_, _, err = p.GetConnectionRoundRobin(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConnPoolClosed)
}
