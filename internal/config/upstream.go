package config

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/netutil"
)

const (
	StrategyFailover     = "failover"
	StrategyRoundRobin   = "round_robin"
	StrategyWeighted     = "weighted"
	StrategyLeastLatency = "least_latency"
)

type HealthSettings struct {
	Interval         time.Duration
	Timeout          time.Duration
	FailThreshold    int
	RecoverThreshold int
}

func (u *UpstreamConfig) HealthSettings() HealthSettings {
	hc := u.HealthCheck
	interval := parseDuration(hc.Interval, 10*time.Second)
	timeout := parseDuration(hc.Timeout, 3*time.Second)
	fail := hc.FailThreshold
	if fail < 1 {
		fail = 3
	}
	recover := hc.RecoverThreshold
	if recover < 1 {
		recover = 2
	}
	return HealthSettings{
		Interval:         interval,
		Timeout:          timeout,
		FailThreshold:    fail,
		RecoverThreshold: recover,
	}
}

func (u *UpstreamConfig) NormalizedStrategy() string {
	switch strings.ToLower(strings.TrimSpace(u.Strategy)) {
	case StrategyRoundRobin, "round-robin":
		return StrategyRoundRobin
	case StrategyWeighted, "weight":
		return StrategyWeighted
	case StrategyLeastLatency, "least-latency", "latency":
		return StrategyLeastLatency
	default:
		return StrategyFailover
	}
}

// UpstreamEndpoint is a resolved upstream server entry.
type UpstreamEndpoint struct {
	Name      string
	Addr      *net.UDPAddr
	Key       string
	Weight    int
	Priority  int
	Transport TransportConfig
}

func (c *Config) UpstreamEndpoints() ([]UpstreamEndpoint, error) {
	if c.Upstream != nil && len(c.Upstream.Servers) > 0 {
		out := make([]UpstreamEndpoint, 0, len(c.Upstream.Servers))
		for i, s := range c.Upstream.Servers {
			addr, err := netutil.ParseUDPAddr(s.Addr, false)
			if err != nil {
				return nil, fmt.Errorf("upstream.servers[%d].addr: %w", i, err)
			}
			name := strings.TrimSpace(s.Name)
			if name == "" {
				name = fmt.Sprintf("upstream-%d", i+1)
			}
			key := strings.TrimSpace(s.Key)
			if key == "" {
				key = c.Transport.KCP.Key
			}
			weight := s.Weight
			if weight < 1 {
				weight = 1
			}
			priority := s.Priority
			if priority < 1 {
				priority = i + 1
			}
			tr := mergeTransport(c.Transport, s.Transport)
			tr.KCP.Key = key
			out = append(out, UpstreamEndpoint{
				Name:      name,
				Addr:      addr,
				Key:       key,
				Weight:    weight,
				Priority:  priority,
				Transport: tr,
			})
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Priority == out[j].Priority {
				return out[i].Name < out[j].Name
			}
			return out[i].Priority < out[j].Priority
		})
		return out, nil
	}

	if c.Server != nil && strings.TrimSpace(c.Server.Addr) != "" {
		addr, err := netutil.ParseUDPAddr(c.Server.Addr, false)
		if err != nil {
			return nil, err
		}
		return []UpstreamEndpoint{{
			Name:      "default",
			Addr:      addr,
			Key:       c.Transport.KCP.Key,
			Weight:    1,
			Priority:  1,
			Transport: c.Transport,
		}}, nil
	}

	return nil, fmt.Errorf("no upstream configured")
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func (c *Config) TransportForKey(key string) TransportConfig {
	t := c.Transport
	if key != "" {
		t.KCP.Key = key
	}
	return t
}

// mergeTransport overlays the non-zero fields of override onto base.
func mergeTransport(base TransportConfig, override *TransportConfig) TransportConfig {
	t := base
	if override == nil {
		return t
	}
	if override.Protocol != "" {
		t.Protocol = override.Protocol
	}
	if override.Conn > 0 {
		t.Conn = override.Conn
	}
	if override.KCP.Mode != "" {
		t.KCP.Mode = override.KCP.Mode
	}
	if override.KCP.Block != "" {
		t.KCP.Block = override.KCP.Block
	}
	if override.KCP.Key != "" {
		t.KCP.Key = override.KCP.Key
	}
	if override.KCP.MTU > 0 {
		t.KCP.MTU = override.KCP.MTU
	}
	if override.KCP.DataShard > 0 {
		t.KCP.DataShard = override.KCP.DataShard
	}
	if override.KCP.ParityShard > 0 {
		t.KCP.ParityShard = override.KCP.ParityShard
	}
	if override.KCP.SndWnd > 0 {
		t.KCP.SndWnd = override.KCP.SndWnd
	}
	if override.KCP.RcvWnd > 0 {
		t.KCP.RcvWnd = override.KCP.RcvWnd
	}
	if override.QUIC.ALPN != "" {
		t.QUIC.ALPN = override.QUIC.ALPN
	}
	if override.QUIC.IdleTimeout != "" {
		t.QUIC.IdleTimeout = override.QUIC.IdleTimeout
	}
	if override.QUIC.MaxIdleTimeout != "" {
		t.QUIC.MaxIdleTimeout = override.QUIC.MaxIdleTimeout
	}
	return t
}

func (c *Config) UpstreamStrategy() string {
	if c.Upstream != nil {
		return c.Upstream.NormalizedStrategy()
	}
	return StrategyFailover
}

// upstreamNameSet returns the set of valid upstream names, derived exactly as
// UpstreamEndpoints() derives them: each upstream.servers[i].name (falling back
// to "upstream-<i+1>" when empty), or "default" for the legacy single
// server.addr form.
func (c *Config) upstreamNameSet() map[string]bool {
	names := make(map[string]bool)
	if c.Upstream != nil && len(c.Upstream.Servers) > 0 {
		for i, s := range c.Upstream.Servers {
			name := strings.TrimSpace(s.Name)
			if name == "" {
				name = fmt.Sprintf("upstream-%d", i+1)
			}
			names[name] = true
		}
		return names
	}
	if c.Server != nil && strings.TrimSpace(c.Server.Addr) != "" {
		names["default"] = true
	}
	return names
}

func (c *Config) UpstreamNames() []string {
	eps, err := c.UpstreamEndpoints()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(eps))
	for _, ep := range eps {
		names = append(names, ep.Name)
	}
	return names
}

func (c *Config) UpstreamHealthTimeout() time.Duration {
	if c.Upstream != nil {
		return c.Upstream.HealthSettings().Timeout
	}
	return 3 * time.Second
}
