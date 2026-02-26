package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"mygateway/domain"
	"mygateway/helpers"
	"mygateway/interfaces"

	"github.com/go-kit/log"
	"google.golang.org/grpc"
)

// ErrConnPoolClosed is returned by GetConnRoundRobin and GetConnForKey when the pool has been closed.
var ErrConnPoolClosed = errors.New("conn pool is closed")

// ErrNoAvailableConnInstance is returned when no instance is available: empty list, all factory dials failed, or (for GetConnForKey) no free instance or key empty.
var ErrNoAvailableConnInstance = errors.New("no available backend instance")

// connectionPool implements interfaces.ConnectionPool. It maintains a set of backend gRPC connections for a
// dynamic cluster: a background refresh loop calls Discoverer.GetInstances and updates the instance
// list; connections for instances that disappeared are closed and sticky bindings removed;
// GetConnRoundRobin returns the next connection in round-robin order; GetConnForKey binds a key
// (e.g. session-id) to an instance and reuses that connection; OnBackendFailure unbinds the key,
// closes the connection to that instance, and calls Discoverer.UnregisterInstance. Fields: discoverer,
// factory, refreshInterval, logger; under mu: instances, keyToID (sticky key → instanceID), instanceConn (instanceID → conn), rr (round-robin index), closed.
type connectionPool struct {
	discoverer      interfaces.Discoverer
	factory         func(ctx context.Context, instance domain.ServiceInstance) (*grpc.ClientConn, error)
	refreshInterval time.Duration
	logger          log.Logger

	mu           sync.RWMutex
	instances    []domain.ServiceInstance
	keyToID      map[string]string
	instanceConn map[string]*grpc.ClientConn
	rr           int
	closed       bool
}

// NewConnectionPool creates a connection pool for one dynamic cluster: starts a goroutine that refreshes the instance list every refreshInterval and runs the first refresh. Panics on nil discoverer, factory or logger.
//
// Parameters: discoverer — source of instance list (e.g. adapters.DiscovererHTTP); factory — (ctx, ServiceInstance) → (*grpc.ClientConn, error) for dialing; refreshInterval — refresh interval (e.g. 5s); logger — logger (GetInstances errors are logged).
//
// Returns: interfaces.ConnectionPool (*connectionPool).
//
// Called from cmd/main for each dynamic cluster.
func NewConnectionPool(
	discoverer interfaces.Discoverer,
	factory func(ctx context.Context, instance domain.ServiceInstance) (*grpc.ClientConn, error),
	refreshInterval time.Duration,
	logger log.Logger,
) interfaces.ConnectionPool {
	p := &connectionPool{
		discoverer:      helpers.NilPanic(discoverer, "service.connection_pool.go: discoverer is required"),
		factory:         helpers.NilPanic(factory, "service.connection_pool.go: factory is required"),
		refreshInterval: refreshInterval,
		logger:          log.With(helpers.NilPanic(logger, "service.connection_pool.go: logger is required"), "component", "connection_pool"),
		keyToID:         make(map[string]string),
		instanceConn:    make(map[string]*grpc.ClientConn),
	}
	p.refresh()
	go p.refreshLoop()
	return p
}

// refreshLoop runs refresh every refreshInterval in a ticker loop. Exits when the pool is closed (ticker.Stop on loop exit not required — goroutine lives until process exit).
//
// Called only from NewConnectionPool in a separate goroutine.
func (p *connectionPool) refreshLoop() {
	ticker := time.NewTicker(p.refreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		p.refresh()
	}
}

// refresh fetches the current instance list from the discoverer; on error logs and returns. On success under lock closes connections for instances not in the new list, unbinds their sticky keys, replaces the instance list and resets rr if needed.
//
// Parameters and return: none. GetInstances error is not returned, only logged.
//
// Called from refreshLoop on timer and once from NewConnectionPool at startup.
func (p *connectionPool) refresh() {
	instances, err := p.discoverer.GetInstances()
	if err != nil {
		_ = log.With(p.logger, "err", err).Log("msg", "discoverer GetInstances failed")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	seen := make(map[string]bool, len(instances))
	for _, inst := range instances {
		seen[inst.InstanceID] = true
	}
	for id, conn := range p.instanceConn {
		if !seen[id] {
			_ = conn.Close()
			delete(p.instanceConn, id)
			for key, mapped := range p.keyToID {
				if mapped == id {
					delete(p.keyToID, key)
				}
			}
		}
	}
	p.instances = instances
	if p.rr >= len(p.instances) {
		p.rr = 0
	}
}

// GetConnectionRoundRobin returns a connection to the next instance in round-robin order, creating it via factory if needed. Caller should respect ctx cancellation (timeout/cancel lead to factory error).
//
// Parameter ctx — context for dial when creating a new connection; cancel or timeout lead to factory error and move to next instance (or ErrNoAvailableConnInstance if all attempts fail).
//
// Returns: (conn, instanceID, nil) on success; (nil, "", ErrConnPoolClosed) if pool is closed; (nil, "", ErrNoAvailableConnInstance) when instance list is empty or dial to all instances fails.
//
// Called from connectionResolverGeneric.GetConnection when route.Balancer.Type != sticky_sessions.
func (p *connectionPool) GetConnectionRoundRobin(ctx context.Context) (*grpc.ClientConn, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, "", ErrConnPoolClosed
	}
	if len(p.instances) == 0 {
		return nil, "", ErrNoAvailableConnInstance
	}
	for i := 0; i < len(p.instances); i++ {
		idx := (p.rr + i) % len(p.instances)
		inst := p.instances[idx]
		conn, err := p.getOrCreateConnLocked(ctx, inst)
		if err != nil {
			continue
		}
		p.rr = (idx + 1) % len(p.instances)
		return conn, inst.InstanceID, nil
	}
	return nil, "", ErrNoAvailableConnInstance
}

// GetConnectionForKey returns a connection for the sticky key: if key is already bound to an instance with a live connection returns it; otherwise picks a free instance or one already bound to this key, creates the connection if needed, binds key→instanceID and returns.
//
// Parameters: ctx — for dial when creating connection; key — sticky header value (e.g. session-id). Empty key yields (nil, "", ErrNoAvailableConnInstance).
//
// Returns: (conn, instanceID, nil) on success; (nil, "", ErrConnPoolClosed) if pool is closed; (nil, "", ErrNoAvailableConnInstance) on empty key or no suitable instance (all occupied by other keys or dial error).
//
// Called from connectionResolverGeneric.GetConnection when route.Balancer.Type == sticky_sessions.
func (p *connectionPool) GetConnectionForKey(ctx context.Context, key string) (*grpc.ClientConn, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, "", ErrConnPoolClosed
	}
	if key == "" {
		return nil, "", ErrNoAvailableConnInstance
	}
	if id := p.keyToID[key]; id != "" {
		if conn := p.instanceConn[id]; conn != nil {
			return conn, id, nil
		}
		delete(p.keyToID, key)
	}
	for _, inst := range p.instances {
		// Skip instance if it's already assigned to a different session (from our keyToID map).
		// Discoverer does not provide AssignedClientSessionID; we track assignments locally.
		if p.isInstanceAssignedToOtherKey(inst.InstanceID, key) {
			continue
		}
		conn, err := p.getOrCreateConnLocked(ctx, inst)
		if err != nil {
			continue
		}
		p.keyToID[key] = inst.InstanceID
		return conn, inst.InstanceID, nil
	}
	return nil, "", ErrNoAvailableConnInstance
}

// isInstanceAssignedToOtherKey returns true if instanceID is bound in keyToID to any key other than excludeKey. Used when choosing an instance for GetConnectionForKey (do not assign an instance already occupied by another session).
//
// Parameters: instanceID — instance ID; excludeKey — current session key (if instance is bound only to it, returns false). Caller must hold p.mu.
//
// Returns: true if the instance is occupied by another session, false otherwise.
//
// Called only from GetConnectionForKey under lock.
func (p *connectionPool) isInstanceAssignedToOtherKey(instanceID, excludeKey string) bool {
	for k, id := range p.keyToID {
		if k != excludeKey && id == instanceID {
			return true
		}
	}
	return false
}

// getOrCreateConnLocked returns the cached connection for the instance or creates it via factory, caches and returns. Caller must hold p.mu.
//
// Parameters: ctx — for dial; inst — instance (InstanceID, Ipv4, Port). On factory error the connection is not cached.
//
// Returns: (conn, nil) on success; (nil, error) on factory error.
//
// Called only from GetConnectionRoundRobin and GetConnectionForKey under lock.
func (p *connectionPool) getOrCreateConnLocked(ctx context.Context, inst domain.ServiceInstance) (*grpc.ClientConn, error) {
	if conn := p.instanceConn[inst.InstanceID]; conn != nil {
		return conn, nil
	}
	conn, err := p.factory(ctx, inst)
	if err != nil {
		return nil, err
	}
	p.instanceConn[inst.InstanceID] = conn
	return conn, nil
}

// OnBackendFailure unbinds the sticky key (if non-empty), closes and removes the connection for instanceID, removes the instance from the instances list (so retries don't hit the dead instance) and calls discoverer.UnregisterInstance(instanceID).
//
// Parameters: key — sticky key of the failed request (empty string allowed — only close and UnregisterInstance will run); instanceID — identifier of the instance that failed.
//
// Called from connectionResolverGeneric.OnBackendFailure on stream or dial failure to the backend.
func (p *connectionPool) OnBackendFailure(key string, instanceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if key != "" {
		delete(p.keyToID, key)
	}
	if conn := p.instanceConn[instanceID]; conn != nil {
		_ = conn.Close()
		delete(p.instanceConn, instanceID)
	}
	// Remove from instances so openBackendStream retries don't keep dialing the same dead instance.
	for i := 0; i < len(p.instances); i++ {
		if p.instances[i].InstanceID == instanceID {
			p.instances = append(p.instances[:i], p.instances[i+1:]...)
			if p.rr >= len(p.instances) {
				p.rr = 0
			}
			break
		}
	}
	_ = p.discoverer.UnregisterInstance(instanceID)
}

// Close marks the pool closed, closes all cached connections and clears the maps. Idempotent: repeated call returns nil with no side effects.
//
// Returns: nil (connection close errors are not returned).
//
// Called from connectionResolverGeneric.Close on shutdown (defer in cmd/main).
func (p *connectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	for _, conn := range p.instanceConn {
		_ = conn.Close()
	}
	p.instanceConn = map[string]*grpc.ClientConn{}
	p.keyToID = map[string]string{}
	return nil
}
