package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/netutil"
	"github.com/paqetpremium/paqetpremium/internal/tunnelpool"
	"github.com/xtaci/smux"
)

type Manager struct {
	cfg    *config.Config
	log    *slog.Logger
	health config.HealthSettings
	strategy string

	mu      sync.RWMutex
	entries map[string]*entry
	order   []string
	active  string
	rrIdx   uint32

	ctx    context.Context
	cancel context.CancelFunc
}

type entry struct {
	ep       config.UpstreamEndpoint
	pool     *tunnelpool.Pool
	healthy  bool
	failures int
	recovers int
	lastRTT  time.Duration
}

func NewManager(ctx context.Context, cfg *config.Config, remoteFlags []netutil.TCPFlagSet, log *slog.Logger) (*Manager, error) {
	endpoints, err := cfg.UpstreamEndpoints()
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	m := &Manager{
		cfg:      cfg,
		log:      log,
		health:   defaultHealth(cfg),
		strategy: defaultStrategy(cfg),
		entries:  make(map[string]*entry),
		ctx:      runCtx,
		cancel:   cancel,
	}

	for _, ep := range endpoints {
		transport := cfg.TransportForKey(ep.Key)
		pool, err := tunnelpool.Dial(runCtx, cfg, ep.Addr, transport, remoteFlags)
		if err != nil {
			m.Close()
			return nil, fmt.Errorf("upstream %q: %w", ep.Name, err)
		}
		m.entries[ep.Name] = &entry{
			ep:      ep,
			pool:    pool,
			healthy: true,
		}
		m.order = append(m.order, ep.Name)
	}

	if len(m.order) > 0 {
		m.active = m.order[0]
	}

	if err := m.pingAll(); err != nil {
		m.log.Warn("initial upstream ping had failures", "err", err)
	}

	go m.healthLoop()
	return m, nil
}

func defaultHealth(cfg *config.Config) config.HealthSettings {
	if cfg.Upstream != nil {
		return cfg.Upstream.HealthSettings()
	}
	return config.HealthSettings{
		Interval:         10 * time.Second,
		Timeout:          3 * time.Second,
		FailThreshold:    3,
		RecoverThreshold: 2,
	}
}

func defaultStrategy(cfg *config.Config) string {
	if cfg.Upstream != nil {
		return cfg.Upstream.NormalizedStrategy()
	}
	return config.StrategyFailover
}

func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.pool != nil {
			e.pool.Close()
		}
	}
	m.entries = nil
	m.order = nil
}

func (m *Manager) ActiveName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *Manager) ForServer(name string) tunnelpool.Opener {
	if name == "" {
		return m
	}
	return &boundOpener{mgr: m, name: name}
}

// Opener implementation — default route.

func (m *Manager) OpenTCP(target string) (*smux.Stream, error) {
	return m.poolFor("").OpenTCP(target)
}

func (m *Manager) OpenUDP(localAddr, target string) (*smux.Stream, bool, uint64, error) {
	return m.poolFor("").OpenUDP(localAddr, target)
}

func (m *Manager) CloseUDP(key uint64) {
	m.poolFor("").CloseUDP(key)
}

func (m *Manager) poolFor(bind string) *tunnelpool.Pool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if bind != "" {
		if e, ok := m.entries[bind]; ok && e.healthy {
			return e.pool
		}
	}

	name := m.pickLocked()
	if e, ok := m.entries[name]; ok {
		return e.pool
	}
	return nil
}

func (m *Manager) pickLocked() string {
	healthy := m.healthyNamesLocked()
	if len(healthy) == 0 {
		return m.active
	}

	switch m.strategy {
	case config.StrategyRoundRobin:
		i := atomic.AddUint32(&m.rrIdx, 1) - 1
		return healthy[int(i)%len(healthy)]
	case config.StrategyWeighted:
		return m.weightedPickLocked(healthy)
	case config.StrategyLeastLatency:
		return m.latencyPickLocked(healthy)
	default:
		if contains(healthy, m.active) {
			return m.active
		}
		m.active = healthy[0]
		return m.active
	}
}

func (m *Manager) healthyNamesLocked() []string {
	var names []string
	for _, name := range m.order {
		if e, ok := m.entries[name]; ok && e.healthy {
			names = append(names, name)
		}
	}
	return names
}

func (m *Manager) weightedPickLocked(healthy []string) string {
	total := 0
	for _, name := range healthy {
		total += m.entries[name].ep.Weight
	}
	if total == 0 {
		return healthy[0]
	}
	i := int(atomic.AddUint32(&m.rrIdx, 1)-1) % total
	for _, name := range healthy {
		w := m.entries[name].ep.Weight
		if i < w {
			return name
		}
		i -= w
	}
	return healthy[0]
}

func (m *Manager) latencyPickLocked(healthy []string) string {
	best := healthy[0]
	bestRTT := time.Duration(1<<63 - 1)
	for _, name := range healthy {
		rtt := m.entries[name].lastRTT
		if rtt == 0 {
			rtt = time.Millisecond
		}
		if rtt < bestRTT {
			bestRTT = rtt
			best = name
		}
	}
	return best
}

func (m *Manager) healthLoop() {
	ticker := time.NewTicker(m.health.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.runHealthCheck()
		}
	}
}

func (m *Manager) runHealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, e := range m.entries {
		start := time.Now()
		err := e.pool.PingWithTimeout(m.health.Timeout)
		rtt := time.Since(start)

		if err != nil {
			e.failures++
			e.recovers = 0
			if e.healthy && e.failures >= m.health.FailThreshold {
				e.healthy = false
				m.log.Warn("upstream unhealthy", "name", name, "addr", e.ep.Addr.String(), "failures", e.failures)
				if m.active == name {
					m.failoverLocked()
				}
			}
			continue
		}

		e.lastRTT = rtt
		e.failures = 0
		if !e.healthy {
			e.recovers++
			if e.recovers >= m.health.RecoverThreshold {
				e.healthy = true
				e.recovers = 0
				m.log.Info("upstream recovered", "name", name, "rtt", rtt)
			}
		}
	}
}

func (m *Manager) failoverLocked() {
	for _, name := range m.order {
		if e, ok := m.entries[name]; ok && e.healthy {
			if m.active != name {
				m.log.Info("upstream failover", "from", m.active, "to", name)
				m.active = name
			}
			return
		}
	}
	m.log.Error("no healthy upstream available")
}

// PingAll pings every configured upstream and returns the first error encountered,
// or nil if all upstreams responded.
func (m *Manager) PingAll() error {
	return m.pingAll()
}

func (m *Manager) pingAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var first error
	for name, e := range m.entries {
		if err := e.pool.PingWithTimeout(m.health.Timeout); err != nil {
			m.log.Warn("upstream ping failed", "name", name, "err", err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

type boundOpener struct {
	mgr  *Manager
	name string
}

func (b *boundOpener) OpenTCP(target string) (*smux.Stream, error) {
	p := b.mgr.poolFor(b.name)
	if p == nil {
		return nil, fmt.Errorf("upstream %q unavailable", b.name)
	}
	return p.OpenTCP(target)
}

func (b *boundOpener) OpenUDP(localAddr, target string) (*smux.Stream, bool, uint64, error) {
	p := b.mgr.poolFor(b.name)
	if p == nil {
		return nil, false, 0, fmt.Errorf("upstream %q unavailable", b.name)
	}
	return p.OpenUDP(localAddr, target)
}

func (b *boundOpener) CloseUDP(key uint64) {
	if p := b.mgr.poolFor(b.name); p != nil {
		p.CloseUDP(key)
	}
}

// Reload replaces upstream pools from a new config (hot reload).
func (m *Manager) Reload(ctx context.Context, cfg *config.Config, remoteFlags []netutil.TCPFlagSet) error {
	endpoints, err := cfg.UpstreamEndpoints()
	if err != nil {
		return err
	}

	newEntries := make(map[string]*entry, len(endpoints))
	newOrder := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		transport := cfg.TransportForKey(ep.Key)
		pool, err := tunnelpool.Dial(ctx, cfg, ep.Addr, transport, remoteFlags)
		if err != nil {
			for _, e := range newEntries {
				e.pool.Close()
			}
			return fmt.Errorf("upstream %q: %w", ep.Name, err)
		}
		newEntries[ep.Name] = &entry{ep: ep, pool: pool, healthy: true}
		newOrder = append(newOrder, ep.Name)
	}

	m.mu.Lock()
	old := m.entries
	m.cfg = cfg
	m.health = defaultHealth(cfg)
	m.strategy = defaultStrategy(cfg)
	m.entries = newEntries
	m.order = newOrder
	if len(newOrder) > 0 {
		m.active = newOrder[0]
	}
	m.mu.Unlock()

	for _, e := range old {
		if e.pool != nil {
			e.pool.Close()
		}
	}

	m.log.Info("upstream configuration reloaded",
		"servers", len(newOrder),
		"strategy", m.strategy,
		"active", m.ActiveName(),
	)
	return nil
}
